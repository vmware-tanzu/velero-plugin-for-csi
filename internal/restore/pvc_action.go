/*
Copyright 2019, 2020 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package restore

import (
	"context"
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch"
	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	snapshotterClientSet "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/kubernetes"

	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	veleroClientSet "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	"github.com/vmware-tanzu/velero/pkg/label"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	riav2 "github.com/vmware-tanzu/velero/pkg/plugin/velero/restoreitemaction/v2"
	"github.com/vmware-tanzu/velero/pkg/util/boolptr"
)

const (
	AnnBindCompleted          = "pv.kubernetes.io/bind-completed"
	AnnBoundByController      = "pv.kubernetes.io/bound-by-controller"
	AnnStorageProvisioner     = "volume.kubernetes.io/storage-provisioner"
	AnnBetaStorageProvisioner = "volume.beta.kubernetes.io/storage-provisioner"
	AnnSelectedNode           = "volume.kubernetes.io/selected-node"
)

const (
	GenerateNameRandomLength = 5
)

// PVCRestoreItemAction is a restore item action plugin for Velero
type PVCRestoreItemAction struct {
	Log            logrus.FieldLogger
	Client         kubernetes.Interface
	SnapshotClient snapshotterClientSet.Interface
	VeleroClient   veleroClientSet.Interface
}

// AppliesTo returns information indicating that the PVCRestoreItemAction should be run while restoring PVCs.
func (p *PVCRestoreItemAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"persistentvolumeclaims"},
		//TODO: add label selector volumeSnapshotLabel
	}, nil
}

func removePVCAnnotations(pvc *corev1api.PersistentVolumeClaim, remove []string) {
	if pvc.Annotations == nil {
		pvc.Annotations = make(map[string]string)
		return
	}
	for k := range pvc.Annotations {
		if util.Contains(remove, k) {
			delete(pvc.Annotations, k)
		}
	}
}

func resetPVCSpec(pvc *corev1api.PersistentVolumeClaim, vsName string) {
	// Restore operation for the PVC will use the volumesnapshot as the data source.
	// So clear out the volume name, which is a ref to the PV
	pvc.Spec.VolumeName = ""
	dataSourceRef := &corev1api.TypedLocalObjectReference{
		APIGroup: &snapshotv1api.SchemeGroupVersion.Group,
		Kind:     util.VolumeSnapshotKindName,
		Name:     vsName,
	}
	pvc.Spec.DataSource = dataSourceRef
	pvc.Spec.DataSourceRef = dataSourceRef
}

func setPVCStorageResourceRequest(pvc *corev1api.PersistentVolumeClaim, restoreSize resource.Quantity, log logrus.FieldLogger) {
	{
		if pvc.Spec.Resources.Requests == nil {
			pvc.Spec.Resources.Requests = corev1api.ResourceList{}
		}

		storageReq, exists := pvc.Spec.Resources.Requests[corev1api.ResourceStorage]
		if !exists || storageReq.Cmp(restoreSize) < 0 {
			pvc.Spec.Resources.Requests[corev1api.ResourceStorage] = restoreSize
			rs := pvc.Spec.Resources.Requests[corev1api.ResourceStorage]
			log.Infof("Resetting storage requests for PVC %s/%s to %s", pvc.Namespace, pvc.Name, rs.String())
		}
	}
}

// Execute modifies the PVC's spec to use the volumesnapshot object as the data source ensuring that the newly provisioned volume
// can be pre-populated with data from the volumesnapshot.
func (p *PVCRestoreItemAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	var pvc, pvcFromBackup corev1api.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &pvc); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.ItemFromBackup.UnstructuredContent(), &pvcFromBackup); err != nil {
		return nil, errors.WithStack(err)
	}

	logger := p.Log.WithFields(logrus.Fields{
		"Action":  "PVCRestoreItemAction",
		"PVC":     pvc.Namespace + "/" + pvc.Name,
		"Restore": input.Restore.Namespace + "/" + input.Restore.Name,
	})
	logger.Info("Starting PVCRestoreItemAction for PVC")

	removePVCAnnotations(&pvc,
		[]string{AnnBindCompleted, AnnBoundByController, AnnStorageProvisioner, AnnBetaStorageProvisioner, AnnSelectedNode})

	// If cross-namespace restore is configured, change the namespace
	// for PVC object to be restored
	if val, ok := input.Restore.Spec.NamespaceMapping[pvc.GetNamespace()]; ok {
		pvc.SetNamespace(val)
	}

	operationID := ""

	// remove the volumesnapshot name annotation as well
	// clean the DataUploadNameLabel for snapshot data mover case.
	removePVCAnnotations(&pvc, []string{util.VolumeSnapshotLabel, util.DataUploadNameAnnotation})

	if boolptr.IsSetToFalse(input.Restore.Spec.RestorePVs) {
		logger.Info("Restore did not request for PVs to be restored from snapshot")
		pvc.Spec.VolumeName = ""
		pvc.Spec.DataSource = nil
		pvc.Spec.DataSourceRef = nil
	} else {
		backup, err := p.VeleroClient.VeleroV1().Backups(input.Restore.Namespace).Get(context.Background(),
			input.Restore.Spec.BackupName, metav1.GetOptions{})
		if err != nil {
			logger.Error("Fail to get backup for restore.")
			return nil, fmt.Errorf("fail to get backup for restore: %s", err.Error())
		}

		if boolptr.IsSetToTrue(backup.Spec.SnapshotMoveData) {
			logger.Info("Start DataMover restore.")

			// If PVC doesn't have a DataUploadNameLabel, which should be created
			// during backup, then CSI cannot handle the volume during to restore,
			// so return early to let Velero tries to fall back to Velero native snapshot.
			if _, ok := pvcFromBackup.Annotations[util.DataUploadNameAnnotation]; !ok {
				logger.Warnf("PVC doesn't have a DataUpload for data mover. Return.")
				return &velero.RestoreItemActionExecuteOutput{
					UpdatedItem: input.Item,
				}, nil
			}

			operationID = label.GetValidName(string(velerov1api.AsyncOperationIDPrefixDataDownload) + string(input.Restore.UID) + "." + string(pvcFromBackup.UID))
			dataDownload, err := restoreFromDataUploadResult(context.Background(), input.Restore, &pvc,
				operationID, pvcFromBackup.Namespace, p.Client, p.VeleroClient)
			if err != nil {
				logger.Errorf("Fail to restore from DataUploadResult: %s", err.Error())
				return nil, errors.WithStack(err)
			}
			logger.Infof("DataDownload %s/%s is created successfully.", dataDownload.Namespace, dataDownload.Name)
		} else {
			volumeSnapshotName, ok := pvcFromBackup.Annotations[util.VolumeSnapshotLabel]
			if !ok {
				logger.Info("Skipping PVCRestoreItemAction for PVC , PVC does not have a CSI volumesnapshot.")
				// Make no change in the input PVC.
				return &velero.RestoreItemActionExecuteOutput{
					UpdatedItem: input.Item,
				}, nil
			}
			if err := restoreFromVolumeSnapshot(&pvc, p.SnapshotClient, volumeSnapshotName, logger); err != nil {
				logger.Errorf("Failed to restore PVC from VolumeSnapshot.")
				return nil, errors.WithStack(err)
			}
		}
	}

	pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	logger.Info("Returning from PVCRestoreItemAction for PVC")

	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem: &unstructured.Unstructured{Object: pvcMap},
		OperationID: operationID,
	}, nil
}

func (p *PVCRestoreItemAction) Name() string {
	return "PVCRestoreItemAction"
}

func (p *PVCRestoreItemAction) Progress(operationID string, restore *velerov1api.Restore) (velero.OperationProgress, error) {
	progress := velero.OperationProgress{}

	if operationID == "" {
		return progress, riav2.InvalidOperationIDError(operationID)
	}
	logger := p.Log.WithFields(logrus.Fields{
		"Action":      "PVCRestoreItemAction",
		"OperationID": operationID,
		"Namespace":   restore.Namespace,
	})

	dataDownload, err := getDataDownload(context.Background(), restore.Namespace, operationID, p.VeleroClient)
	if err != nil {
		logger.Errorf("fail to get DataDownload: %s", err.Error())
		return progress, err
	}
	if dataDownload.Status.Phase == velerov2alpha1.DataDownloadPhaseNew ||
		dataDownload.Status.Phase == "" {
		logger.Debugf("DataDownload is still not processed yet. Skip progress update.")
		return progress, nil
	}

	progress.Description = string(dataDownload.Status.Phase)
	progress.OperationUnits = "Bytes"
	progress.NCompleted = dataDownload.Status.Progress.BytesDone
	progress.NTotal = dataDownload.Status.Progress.TotalBytes

	if dataDownload.Status.StartTimestamp != nil {
		progress.Started = dataDownload.Status.StartTimestamp.Time
	}

	if dataDownload.Status.CompletionTimestamp != nil {
		progress.Updated = dataDownload.Status.CompletionTimestamp.Time
	}

	if dataDownload.Status.Phase == velerov2alpha1.DataDownloadPhaseCompleted {
		progress.Completed = true
	} else if dataDownload.Status.Phase == velerov2alpha1.DataDownloadPhaseCanceled {
		progress.Completed = true
		progress.Err = fmt.Sprintf("DataDownload is canceled")
	} else if dataDownload.Status.Phase == velerov2alpha1.DataDownloadPhaseFailed {
		progress.Completed = true
		progress.Err = dataDownload.Status.Message
	}

	return progress, nil
}

func (p *PVCRestoreItemAction) Cancel(operationID string, restore *velerov1api.Restore) error {
	if operationID == "" {
		return riav2.InvalidOperationIDError(operationID)
	}
	logger := p.Log.WithFields(logrus.Fields{
		"Action":      "PVCRestoreItemAction",
		"OperationID": operationID,
		"Namespace":   restore.Namespace,
	})

	dataDownload, err := getDataDownload(context.Background(), restore.Namespace, operationID, p.VeleroClient)
	if err != nil {
		logger.Errorf("fail to get DataDownload: %s", err.Error())
		return err
	}

	err = cancelDataDownload(context.Background(), p.VeleroClient, dataDownload)
	if err != nil {
		logger.Errorf("fail to cancel DataDownload %s: %s", dataDownload.Name, err.Error())
	}
	return err
}

func (p *PVCRestoreItemAction) AreAdditionalItemsReady(additionalItems []velero.ResourceIdentifier, restore *velerov1api.Restore) (bool, error) {
	return true, nil
}

func getDataUploadResult(ctx context.Context, restore *velerov1api.Restore, pvc *corev1api.PersistentVolumeClaim,
	sourceNamespace string, kubeClient kubernetes.Interface) (*velerov2alpha1.DataUploadResult, error) {
	labelSelector := fmt.Sprintf("%s=%s,%s=%s,%s=%s", velerov1api.PVCNamespaceNameLabel, label.GetValidName(sourceNamespace+"."+pvc.Name),
		velerov1api.RestoreUIDLabel, label.GetValidName(string(restore.UID)),
		velerov1api.ResourceUsageLabel, label.GetValidName(string(velerov1api.VeleroResourceUsageDataUploadResult)),
	)
	cmList, err := kubeClient.CoreV1().ConfigMaps(restore.Namespace).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, errors.Wrapf(err, "error to get DataUpload result cm with labels %s", labelSelector)
	}

	if len(cmList.Items) == 0 {
		return nil, errors.Errorf("no DataUpload result cm found with labels %s", labelSelector)
	}

	if len(cmList.Items) > 1 {
		return nil, errors.Errorf("multiple DataUpload result cms found with labels %s", labelSelector)
	}

	jsonBytes, exist := cmList.Items[0].Data[string(restore.UID)]
	if !exist {
		return nil, errors.Errorf("no DataUpload result found with restore key %s, restore %s", string(restore.UID), restore.Name)
	}

	result := velerov2alpha1.DataUploadResult{}
	err = json.Unmarshal([]byte(jsonBytes), &result)
	if err != nil {
		return nil, errors.Errorf("error to unmarshal DataUploadResult, restore UID %s, restore name %s", string(restore.UID), restore.Name)
	}

	return &result, nil
}

func getDataDownload(ctx context.Context, namespace string, operationID string, veleroClient veleroClientSet.Interface) (*velerov2alpha1.DataDownload, error) {
	dataDownloadList, err := veleroClient.VeleroV2alpha1().DataDownloads(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", velerov1api.AsyncOperationIDLabel, operationID),
	})
	if err != nil {
		return nil, errors.Wrap(err, "fail to list DataDownload")
	}

	if len(dataDownloadList.Items) == 0 {
		return nil, errors.Errorf("didn't find DataDownload")
	}

	if len(dataDownloadList.Items) > 1 {
		return nil, errors.Errorf("find multiple DataDownloads")
	}

	return &dataDownloadList.Items[0], nil
}

func cancelDataDownload(ctx context.Context, veleroClient veleroClientSet.Interface,
	dataDownload *velerov2alpha1.DataDownload) error {
	oldData, err := json.Marshal(dataDownload)
	if err != nil {
		return errors.Wrap(err, "fail to marshal origin DataDownload")
	}

	updatedDataDownload := dataDownload.DeepCopy()
	updatedDataDownload.Spec.Cancel = true

	newData, err := json.Marshal(updatedDataDownload)
	if err != nil {
		return errors.Wrap(err, "fail to marshal updated DataDownload")
	}

	patchData, err := jsonpatch.CreateMergePatch(oldData, newData)
	if err != nil {
		return errors.Wrap(err, "fail to create merge patch for DataDownload")
	}

	_, err = veleroClient.VeleroV2alpha1().DataDownloads(dataDownload.Namespace).Patch(ctx, dataDownload.Name,
		types.MergePatchType, patchData, metav1.PatchOptions{})
	return err
}

func newDataDownload(restore *velerov1api.Restore, dataUploadResult *velerov2alpha1.DataUploadResult,
	pvc *corev1api.PersistentVolumeClaim, operationID string) *velerov2alpha1.DataDownload {
	dataDownload := &velerov2alpha1.DataDownload{
		TypeMeta: metav1.TypeMeta{
			APIVersion: velerov2alpha1.SchemeGroupVersion.String(),
			Kind:       "DataDownload",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    restore.Namespace,
			GenerateName: restore.Name + "-",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: velerov1api.SchemeGroupVersion.String(),
					Kind:       "Restore",
					Name:       restore.Name,
					UID:        restore.UID,
					Controller: boolptr.True(),
				},
			},
			Labels: map[string]string{
				velerov1api.RestoreNameLabel:      label.GetValidName(restore.Name),
				velerov1api.RestoreUIDLabel:       string(restore.UID),
				velerov1api.AsyncOperationIDLabel: operationID,
			},
		},
		Spec: velerov2alpha1.DataDownloadSpec{
			TargetVolume: velerov2alpha1.TargetVolumeSpec{
				PVC:       pvc.Name,
				Namespace: pvc.Namespace,
			},
			BackupStorageLocation: dataUploadResult.BackupStorageLocation,
			DataMover:             dataUploadResult.DataMover,
			SnapshotID:            dataUploadResult.SnapshotID,
			SourceNamespace:       dataUploadResult.SourceNamespace,
			OperationTimeout:      restore.Spec.ItemOperationTimeout,
		},
	}

	return dataDownload
}

func restoreFromVolumeSnapshot(pvc *corev1api.PersistentVolumeClaim, snapClient snapshotterClientSet.Interface,
	volumeSnapshotName string, logger logrus.FieldLogger) error {
	vs, err := snapClient.SnapshotV1().VolumeSnapshots(pvc.Namespace).Get(context.TODO(), volumeSnapshotName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, fmt.Sprintf("Failed to get Volumesnapshot %s/%s to restore PVC %s/%s", pvc.Namespace, volumeSnapshotName, pvc.Namespace, pvc.Name))
	}

	if _, exists := vs.Annotations[util.VolumeSnapshotRestoreSize]; exists {
		restoreSize, err := resource.ParseQuantity(vs.Annotations[util.VolumeSnapshotRestoreSize])
		if err != nil {
			return errors.Wrapf(err, fmt.Sprintf("Failed to parse %s from annotation on Volumesnapshot %s/%s into restore size",
				vs.Annotations[util.VolumeSnapshotRestoreSize], vs.Namespace, vs.Name))
		}
		// It is possible that the volume provider allocated a larger capacity volume than what was requested in the backed up PVC.
		// In this scenario the volumesnapshot of the PVC will end being larger than its requested storage size.
		// Such a PVC, on restore as-is, will be stuck attempting to use a Volumesnapshot as a data source for a PVC that
		// is not large enough.
		// To counter that, here we set the storage request on the PVC to the larger of the PVC's storage request and the size of the
		// VolumeSnapshot
		setPVCStorageResourceRequest(pvc, restoreSize, logger)
	}

	resetPVCSpec(pvc, volumeSnapshotName)

	return nil
}

func restoreFromDataUploadResult(ctx context.Context, restore *velerov1api.Restore, pvc *corev1api.PersistentVolumeClaim,
	operationID string, sourceNamespace string, kubeClient kubernetes.Interface, veleroClient veleroClientSet.Interface) (*velerov2alpha1.DataDownload, error) {
	dataUploadResult, err := getDataUploadResult(ctx, restore, pvc, sourceNamespace, kubeClient)
	if err != nil {
		return nil, errors.Wrapf(err, "fail get DataUploadResult for restore: %s", restore.Name)
	}
	pvc.Spec.VolumeName = ""
	if pvc.Spec.Selector == nil {
		pvc.Spec.Selector = &metav1.LabelSelector{}
	}
	if pvc.Spec.Selector.MatchLabels == nil {
		pvc.Spec.Selector.MatchLabels = make(map[string]string)
	}
	pvc.Spec.Selector.MatchLabels[util.DynamicPVRestoreLabel] = label.GetValidName(fmt.Sprintf("%s.%s.%s", pvc.Namespace, pvc.Name, utilrand.String(GenerateNameRandomLength)))

	dataDownload := newDataDownload(restore, dataUploadResult, pvc, operationID)
	_, err = veleroClient.VeleroV2alpha1().DataDownloads(restore.Namespace).Create(ctx, dataDownload, metav1.CreateOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "fail to create DataDownload")
	}

	return dataDownload, nil
}
