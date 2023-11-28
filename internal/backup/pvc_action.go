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

package backup

import (
	"context"
	"fmt"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	snapshotterClientSet "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	"github.com/vmware-tanzu/velero/pkg/kuberesource"
	"github.com/vmware-tanzu/velero/pkg/label"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	biav2 "github.com/vmware-tanzu/velero/pkg/plugin/velero/backupitemaction/v2"
	"github.com/vmware-tanzu/velero/pkg/util/boolptr"
)

// PVCBackupItemAction is a backup item action plugin for Velero.
type PVCBackupItemAction struct {
	Log            logrus.FieldLogger
	Client         kubernetes.Interface
	SnapshotClient snapshotterClientSet.Interface
	CRClient       crclient.Client
}

// AppliesTo returns information indicating that the PVCBackupItemAction should be invoked to backup PVCs.
func (p *PVCBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Debug("PVCBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"persistentvolumeclaims"},
	}, nil
}

// Execute recognizes PVCs backed by volumes provisioned by CSI drivers with volumesnapshotting capability and creates snapshots of the
// underlying PVs by creating volumesnapshot CSI API objects that will trigger the CSI driver to perform the snapshot operation on the volume.
func (p *PVCBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, string, []velero.ResourceIdentifier, error) {
	p.Log.Info("Starting PVCBackupItemAction")

	// Do nothing if volume snapshots have not been requested in this backup
	if boolptr.IsSetToFalse(backup.Spec.SnapshotVolumes) {
		p.Log.Infof("Volume snapshotting not requested for backup %s/%s", backup.Namespace, backup.Name)
		return item, nil, "", nil, nil
	}

	if backup.Status.Phase == velerov1api.BackupPhaseFinalizing ||
		backup.Status.Phase == velerov1api.BackupPhaseFinalizingPartiallyFailed {
		p.Log.WithFields(
			logrus.Fields{
				"Backup": fmt.Sprintf("%s/%s", backup.Namespace, backup.Name),
				"Phase":  backup.Status.Phase,
			},
		).Debug("Backup is in finalizing phase. Skip this PVC.")
		return item, nil, "", nil, nil
	}

	var pvc corev1api.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pvc); err != nil {
		return nil, nil, "", nil, errors.WithStack(err)
	}

	p.Log.Debugf("Fetching underlying PV for PVC %s", fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name))
	// Do nothing if this is not a CSI provisioned volume
	pv, err := util.GetPVForPVC(&pvc, p.Client.CoreV1())
	if err != nil {
		return nil, nil, "", nil, errors.WithStack(err)
	}
	if pv.Spec.PersistentVolumeSource.CSI == nil {
		p.Log.Infof("Skipping PVC %s/%s, associated PV %s is not a CSI volume", pvc.Namespace, pvc.Name, pv.Name)

		util.AddAnnotations(&pvc.ObjectMeta, map[string]string{
			util.SkippedNoCSIPVAnnotation: "true",
		})
		data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
		return &unstructured.Unstructured{Object: data}, nil, "", nil, err
	}

	// Do nothing if FS uploader is used to backup this PV
	isFSUploaderUsed, err := util.IsPVCDefaultToFSBackup(pvc.Namespace, pvc.Name, p.Client.CoreV1(), boolptr.IsSetToTrue(backup.Spec.DefaultVolumesToFsBackup))
	if err != nil {
		return nil, nil, "", nil, errors.WithStack(err)
	}
	if isFSUploaderUsed {
		p.Log.Infof("Skipping  PVC %s/%s, PV %s will be backed up using FS uploader", pvc.Namespace, pvc.Name, pv.Name)
		return item, nil, "", nil, nil
	}

	// no storage class: we don't know how to map to a VolumeSnapshotClass
	if pvc.Spec.StorageClassName == nil {
		return item, nil, "", nil, errors.Errorf("Cannot snapshot PVC %s/%s, PVC has no storage class.", pvc.Namespace, pvc.Name)
	}

	p.Log.Infof("Fetching storage class for PV %s", *pvc.Spec.StorageClassName)
	storageClass, err := p.Client.StorageV1().StorageClasses().Get(context.TODO(), *pvc.Spec.StorageClassName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, "", nil, errors.Wrap(err, "error getting storage class")
	}
	p.Log.Debugf("Fetching volumesnapshot class for %s", storageClass.Provisioner)
	snapshotClass, err := util.GetVolumeSnapshotClass(storageClass.Provisioner, backup, &pvc, p.Log, p.SnapshotClient.SnapshotV1())
	if err != nil {
		return nil, nil, "", nil, errors.Wrapf(err, "failed to get volumesnapshotclass for storageclass %s", storageClass.Name)
	}
	p.Log.Infof("volumesnapshot class=%s", snapshotClass.Name)

	vsLabels := map[string]string{}
	for k, v := range pvc.ObjectMeta.Labels {
		vsLabels[k] = v
	}
	vsLabels[velerov1api.BackupNameLabel] = label.GetValidName(backup.Name)

	// Craft the snapshot object to be created
	snapshot := snapshotv1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "velero-" + pvc.Name + "-",
			Namespace:    pvc.Namespace,
			Labels:       vsLabels,
		},
		Spec: snapshotv1api.VolumeSnapshotSpec{
			Source: snapshotv1api.VolumeSnapshotSource{
				PersistentVolumeClaimName: &pvc.Name,
			},
			VolumeSnapshotClassName: &snapshotClass.Name,
		},
	}

	upd, err := p.SnapshotClient.SnapshotV1().VolumeSnapshots(pvc.Namespace).Create(context.TODO(), &snapshot, metav1.CreateOptions{})
	if err != nil {
		return nil, nil, "", nil, errors.Wrapf(err, "error creating volume snapshot")
	}
	p.Log.Infof("Created volumesnapshot %s", fmt.Sprintf("%s/%s", upd.Namespace, upd.Name))

	labels := map[string]string{
		util.VolumeSnapshotLabel:    upd.Name,
		velerov1api.BackupNameLabel: backup.Name,
	}

	annotations := map[string]string{
		util.VolumeSnapshotLabel:                 upd.Name,
		util.MustIncludeAdditionalItemAnnotation: "true",
	}

	var additionalItems []velero.ResourceIdentifier
	operationID := ""
	var itemToUpdate []velero.ResourceIdentifier

	if boolptr.IsSetToTrue(backup.Spec.SnapshotMoveData) {
		operationID = label.GetValidName(string(velerov1api.AsyncOperationIDPrefixDataUpload) + string(backup.UID) + "." + string(pvc.UID))
		dataUploadLog := p.Log.WithFields(logrus.Fields{
			"Source PVC":     fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name),
			"VolumeSnapshot": fmt.Sprintf("%s/%s", upd.Namespace, upd.Name),
			"Operation ID":   operationID,
			"Backup":         backup.Name,
		})

		// Wait until VS associated VSC snapshot handle created before returning with
		// the Async operation for data mover.
		_, err := util.GetVolumeSnapshotContentForVolumeSnapshot(upd, p.SnapshotClient.SnapshotV1(),
			dataUploadLog, true, backup.Spec.CSISnapshotTimeout.Duration)
		if err != nil {
			dataUploadLog.Errorf("Fail to wait VolumeSnapshot snapshot handle created: %s", err.Error())
			util.CleanupVolumeSnapshot(upd, p.SnapshotClient.SnapshotV1(), p.Log)
			return nil, nil, "", nil, errors.WithStack(err)
		}

		dataUploadLog.Info("Starting data upload of backup")

		dataUpload, err := createDataUpload(context.Background(), backup, p.CRClient, upd, &pvc, operationID, snapshotClass)
		if err != nil {
			dataUploadLog.WithError(err).Error("failed to submit DataUpload")
			util.DeleteVolumeSnapshotIfAny(context.Background(), p.SnapshotClient, *upd, dataUploadLog)

			return nil, nil, "", nil, errors.Wrapf(err, "error creating DataUpload")
		} else {
			itemToUpdate = []velero.ResourceIdentifier{
				{
					GroupResource: schema.GroupResource{
						Group:    "velero.io",
						Resource: "datauploads",
					},
					Namespace: dataUpload.Namespace,
					Name:      dataUpload.Name,
				},
			}
			// Set the DataUploadNameLabel, which is used for restore to let CSI plugin check whether
			// it should handle the volume. If volume is CSI migration, PVC doesn't have the annotation.
			annotations[util.DataUploadNameAnnotation] = dataUpload.Namespace + "/" + dataUpload.Name

			dataUploadLog.Info("DataUpload is submitted successfully.")
		}
	} else {
		additionalItems = []velero.ResourceIdentifier{
			{
				GroupResource: kuberesource.VolumeSnapshots,
				Namespace:     upd.Namespace,
				Name:          upd.Name,
			},
		}
	}

	util.AddAnnotations(&pvc.ObjectMeta, annotations)
	util.AddLabels(&pvc.ObjectMeta, labels)

	p.Log.Infof("Returning from PVCBackupItemAction with %d additionalItems to backup", len(additionalItems))
	for _, ai := range additionalItems {
		p.Log.Debugf("%s: %s", ai.GroupResource.String(), ai.Name)
	}

	pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
	if err != nil {
		return nil, nil, "", nil, errors.WithStack(err)
	}

	return &unstructured.Unstructured{Object: pvcMap}, additionalItems, operationID, itemToUpdate, nil
}

func (p *PVCBackupItemAction) Name() string {
	return "PVCBackupItemAction"
}

func (p *PVCBackupItemAction) Progress(operationID string, backup *velerov1api.Backup) (velero.OperationProgress, error) {
	progress := velero.OperationProgress{}
	if operationID == "" {
		return progress, biav2.InvalidOperationIDError(operationID)
	}

	dataUpload, err := getDataUpload(context.Background(), backup, p.CRClient, operationID)
	if err != nil {
		p.Log.Errorf("fail to get DataUpload for backup %s/%s: %s", backup.Namespace, backup.Name, err.Error())
		return progress, err
	}
	if dataUpload.Status.Phase == velerov2alpha1.DataUploadPhaseNew || dataUpload.Status.Phase == "" {
		p.Log.Debugf("DataUpload is still not processed yet. Skip progress update.")
		return progress, nil
	}

	progress.Description = string(dataUpload.Status.Phase)
	progress.OperationUnits = "Bytes"
	progress.NCompleted = dataUpload.Status.Progress.BytesDone
	progress.NTotal = dataUpload.Status.Progress.TotalBytes

	if dataUpload.Status.StartTimestamp != nil {
		progress.Started = dataUpload.Status.StartTimestamp.Time
	}

	if dataUpload.Status.CompletionTimestamp != nil {
		progress.Updated = dataUpload.Status.CompletionTimestamp.Time
	}

	if dataUpload.Status.Phase == velerov2alpha1.DataUploadPhaseCompleted {
		progress.Completed = true
	} else if dataUpload.Status.Phase == velerov2alpha1.DataUploadPhaseFailed {
		progress.Completed = true
		progress.Err = dataUpload.Status.Message
	} else if dataUpload.Status.Phase == velerov2alpha1.DataUploadPhaseCanceled {
		progress.Completed = true
		progress.Err = "DataUpload is canceled"
	}

	return progress, nil
}

func (p *PVCBackupItemAction) Cancel(operationID string, backup *velerov1api.Backup) error {
	if operationID == "" {
		return biav2.InvalidOperationIDError(operationID)
	}

	dataUpload, err := getDataUpload(context.Background(), backup, p.CRClient, operationID)
	if err != nil {
		p.Log.Errorf("fail to get DataUpload for backup %s/%s: %s", backup.Namespace, backup.Name, err.Error())
		return err
	}

	return cancelDataUpload(context.Background(), p.CRClient, dataUpload)
}

func newDataUpload(backup *velerov1api.Backup, vs *snapshotv1api.VolumeSnapshot,
	pvc *corev1api.PersistentVolumeClaim, operationID string, vsClass *snapshotv1api.VolumeSnapshotClass) *velerov2alpha1.DataUpload {
	dataUpload := &velerov2alpha1.DataUpload{
		TypeMeta: metav1.TypeMeta{
			APIVersion: velerov2alpha1.SchemeGroupVersion.String(),
			Kind:       "DataUpload",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:    backup.Namespace,
			GenerateName: backup.Name + "-",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: velerov1api.SchemeGroupVersion.String(),
					Kind:       "Backup",
					Name:       backup.Name,
					UID:        backup.UID,
					Controller: boolptr.True(),
				},
			},
			Labels: map[string]string{
				velerov1api.BackupNameLabel:       label.GetValidName(backup.Name),
				velerov1api.BackupUIDLabel:        string(backup.UID),
				velerov1api.PVCUIDLabel:           string(pvc.UID),
				velerov1api.AsyncOperationIDLabel: operationID,
			},
		},
		Spec: velerov2alpha1.DataUploadSpec{
			SnapshotType: velerov2alpha1.SnapshotTypeCSI,
			CSISnapshot: &velerov2alpha1.CSISnapshotSpec{
				VolumeSnapshot: vs.Name,
				StorageClass:   *pvc.Spec.StorageClassName,
				SnapshotClass:  vsClass.Name,
			},
			SourcePVC:             pvc.Name,
			DataMover:             backup.Spec.DataMover,
			BackupStorageLocation: backup.Spec.StorageLocation,
			SourceNamespace:       pvc.Namespace,
			OperationTimeout:      backup.Spec.CSISnapshotTimeout,
		},
	}

	return dataUpload
}

func createDataUpload(ctx context.Context, backup *velerov1api.Backup, crClient crclient.Client,
	vs *snapshotv1api.VolumeSnapshot, pvc *corev1api.PersistentVolumeClaim, operationID string,
	vsClass *snapshotv1api.VolumeSnapshotClass) (*velerov2alpha1.DataUpload, error) {
	dataUpload := newDataUpload(backup, vs, pvc, operationID, vsClass)

	err := crClient.Create(ctx, dataUpload)
	if err != nil {
		return nil, errors.Wrap(err, "fail to create DataUpload CR")
	}

	return dataUpload, err
}

func getDataUpload(ctx context.Context, backup *velerov1api.Backup,
	crClient crclient.Client, operationID string) (*velerov2alpha1.DataUpload, error) {
	dataUploadList := new(velerov2alpha1.DataUploadList)
	err := crClient.List(context.Background(), dataUploadList, &crclient.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{velerov1api.AsyncOperationIDLabel: operationID}),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "error to list DataUpload")
	}

	if len(dataUploadList.Items) == 0 {
		return nil, errors.Errorf("not found DataUpload for operationID %s", operationID)
	}

	if len(dataUploadList.Items) > 1 {
		return nil, errors.Errorf("more than one DataUpload found operationID %s", operationID)
	}

	return &dataUploadList.Items[0], nil
}

func cancelDataUpload(ctx context.Context, crClient crclient.Client,
	dataUpload *velerov2alpha1.DataUpload) error {

	updatedDataUpload := dataUpload.DeepCopy()
	updatedDataUpload.Spec.Cancel = true

	err := crClient.Patch(context.Background(), updatedDataUpload, crclient.MergeFrom(dataUpload))
	if err != nil {
		return errors.Wrap(err, "error patch DataUpload")
	}

	return nil
}
