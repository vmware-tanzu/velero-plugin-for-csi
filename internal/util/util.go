/*
Copyright 2020 the Velero contributors.

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

package util

import (
	"context"
	"fmt"
	"strings"
	"time"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumesnapshot/v1"
	snapshotterClientSet "github.com/kubernetes-csi/external-snapshotter/client/v7/clientset/versioned"
	snapshotter "github.com/kubernetes-csi/external-snapshotter/client/v7/clientset/versioned/typed/volumesnapshot/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1api "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/clientcmd"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov2alpha1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	"github.com/vmware-tanzu/velero/pkg/label"
	"github.com/vmware-tanzu/velero/pkg/util/podvolume"
)

const (
	VolumeSnapshotKindName    = "VolumeSnapshot"
	defaultCSISnapshotTimeout = 10 * time.Minute
)

func GetPVForPVC(pvc *corev1api.PersistentVolumeClaim, corev1 corev1client.PersistentVolumesGetter) (*corev1api.PersistentVolume, error) {
	if pvc.Spec.VolumeName == "" {
		return nil, errors.Errorf("PVC %s/%s has no volume backing this claim", pvc.Namespace, pvc.Name)
	}
	if pvc.Status.Phase != corev1api.ClaimBound {
		// TODO: confirm if this PVC should be snapshotted if it has no PV bound
		return nil, errors.Errorf("PVC %s/%s is in phase %v and is not bound to a volume", pvc.Namespace, pvc.Name, pvc.Status.Phase)
	}
	pvName := pvc.Spec.VolumeName
	pv, err := corev1.PersistentVolumes().Get(context.TODO(), pvName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get PV %s for PVC %s/%s", pvName, pvc.Namespace, pvc.Name)
	}
	return pv, nil
}

func GetPodsUsingPVC(pvcNamespace, pvcName string, corev1 corev1client.PodsGetter) ([]corev1api.Pod, error) {
	podsUsingPVC := []corev1api.Pod{}
	podList, err := corev1.Pods(pvcNamespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, p := range podList.Items {
		for _, v := range p.Spec.Volumes {
			if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == pvcName {
				podsUsingPVC = append(podsUsingPVC, p)
			}
		}
	}

	return podsUsingPVC, nil
}

func GetPodVolumeNameForPVC(pod corev1api.Pod, pvcName string) (string, error) {
	for _, v := range pod.Spec.Volumes {
		if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == pvcName {
			return v.Name, nil
		}
	}
	return "", errors.Errorf("Pod %s/%s does not use PVC %s/%s", pod.Namespace, pod.Name, pod.Namespace, pvcName)
}

func Contains(slice []string, key string) bool {
	for _, i := range slice {
		if i == key {
			return true
		}
	}
	return false
}

func IsPVCDefaultToFSBackup(pvcNamespace, pvcName string, podClient corev1client.PodsGetter, defaultVolumesToFsBackup bool) (bool, error) {
	pods, err := GetPodsUsingPVC(pvcNamespace, pvcName, podClient)
	if err != nil {
		return false, errors.WithStack(err)
	}

	for _, p := range pods {
		vols, _ := podvolume.GetVolumesByPod(&p, defaultVolumesToFsBackup, false)
		if len(vols) > 0 {
			volName, err := GetPodVolumeNameForPVC(p, pvcName)
			if err != nil {
				return false, err
			}
			if Contains(vols, volName) {
				return true, nil
			}
		}
	}

	return false, nil
}
func GetVolumeSnapshotClass(provisioner string, backup *velerov1api.Backup, pvc *corev1api.PersistentVolumeClaim, log logrus.FieldLogger, snapshotClient snapshotter.SnapshotV1Interface) (*snapshotv1api.VolumeSnapshotClass, error) {
	snapshotClasses, err := snapshotClient.VolumeSnapshotClasses().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "error listing volumesnapshot classes")
	}
	// If a snapshot class is sent for provider in PVC annotations, use that
	snapshotClass, err := GetVolumeSnapshotClassFromPVCAnnotationsForDriver(pvc, provisioner, snapshotClasses)
	if err != nil {
		log.Debugf("Didn't find VolumeSnapshotClass from PVC annotations: %v", err)
	}
	if snapshotClass != nil {
		return snapshotClass, nil
	}

	// If there is no annotation in PVC, attempt to fetch it from backup annotations
	snapshotClass, err = GetVolumeSnapshotClassFromBackupAnnotationsForDriver(backup, provisioner, snapshotClasses)
	if err != nil {
		log.Debugf("Didn't find VolumeSnapshotClass from Backup annotations: %v", err)
	}
	if snapshotClass != nil {
		return snapshotClass, nil
	}

	// fallback to default behaviour of fetching snapshot class based on label
	snapshotClass, err = GetVolumeSnapshotClassForStorageClass(provisioner, snapshotClasses)
	if err != nil || snapshotClass == nil {
		return nil, errors.Wrap(err, "error getting volumesnapshotclass")
	}

	return snapshotClass, nil
}

func GetVolumeSnapshotClassFromPVCAnnotationsForDriver(pvc *corev1api.PersistentVolumeClaim, provisioner string, snapshotClasses *snapshotv1api.VolumeSnapshotClassList) (*snapshotv1api.VolumeSnapshotClass, error) {
	annotationKey := VolumeSnapshotClassDriverPVCAnnotation
	snapshotClassName, ok := pvc.ObjectMeta.Annotations[annotationKey]
	if !ok {
		return nil, nil
	}
	for _, sc := range snapshotClasses.Items {
		if strings.EqualFold(snapshotClassName, sc.ObjectMeta.Name) {
			if !strings.EqualFold(sc.Driver, provisioner) {
				return nil, errors.Errorf("Incorrect volumesnapshotclass, snapshot class %s is not for driver %s", sc.ObjectMeta.Name, provisioner)
			}
			return &sc, nil
		}
	}
	return nil, errors.Errorf("No CSI VolumeSnapshotClass found with name %s for provisioner %s for PVC %s", snapshotClassName, provisioner, pvc.Name)
}

// GetVolumeSnapshotClassFromAnnotationsForDriver returns a VolumeSnapshotClass for the supplied volume provisioner/ driver name from the annotation of the backup
func GetVolumeSnapshotClassFromBackupAnnotationsForDriver(backup *velerov1api.Backup, provisioner string, snapshotClasses *snapshotv1api.VolumeSnapshotClassList) (*snapshotv1api.VolumeSnapshotClass, error) {
	annotationKey := fmt.Sprintf("%s_%s", VolumeSnapshotClassDriverBackupAnnotationPrefix, strings.ToLower(provisioner))
	snapshotClassName, ok := backup.ObjectMeta.Annotations[annotationKey]
	if !ok {
		return nil, nil
	}
	for _, sc := range snapshotClasses.Items {
		if strings.EqualFold(snapshotClassName, sc.ObjectMeta.Name) {
			if !strings.EqualFold(sc.Driver, provisioner) {
				return nil, errors.Errorf("Incorrect volumesnapshotclass, snapshot class %s is not for driver %s for backup %s", sc.ObjectMeta.Name, provisioner, backup.Name)
			}
			return &sc, nil
		}
	}
	return nil, errors.Errorf("No CSI VolumeSnapshotClass found with name %s for driver %s for backup %s", snapshotClassName, provisioner, backup.Name)
}

// GetVolumeSnapshotClassForStorageClass returns a VolumeSnapshotClass for the supplied volume provisioner/ driver name.
func GetVolumeSnapshotClassForStorageClass(provisioner string, snapshotClasses *snapshotv1api.VolumeSnapshotClassList) (*snapshotv1api.VolumeSnapshotClass, error) {
	n := 0
	var vsclass snapshotv1api.VolumeSnapshotClass
	// We pick the volumesnapshotclass that matches the CSI driver name and has a 'velero.io/csi-volumesnapshot-class'
	// label. This allows multiple VolumesnapshotClasses for the same driver with different values for the
	// other fields in the spec.
	// https://github.com/kubernetes-csi/external-snapshotter/blob/release-4.2/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
	for _, sc := range snapshotClasses.Items {
		_, hasLabelSelector := sc.Labels[VolumeSnapshotClassSelectorLabel]
		if sc.Driver == provisioner {
			n += 1
			vsclass = sc
			if hasLabelSelector {
				return &sc, nil
			}
		}
	}
	// If there's only one volumesnapshotclass for the driver, return it.
	if n == 1 {
		return &vsclass, nil
	}
	return nil, errors.Errorf("failed to get volumesnapshotclass for provisioner %s, ensure that the desired volumesnapshot class has the %s label", provisioner, VolumeSnapshotClassSelectorLabel)
}

// GetVolumeSnapshotContentForVolumeSnapshot returns the volumesnapshotcontent object associated with the volumesnapshot
func GetVolumeSnapshotContentForVolumeSnapshot(volSnap *snapshotv1api.VolumeSnapshot, snapshotClient snapshotter.SnapshotV1Interface, log logrus.FieldLogger, shouldWait bool, csiSnapshotTimeout time.Duration) (*snapshotv1api.VolumeSnapshotContent, error) {
	if !shouldWait {
		if volSnap.Status == nil || volSnap.Status.BoundVolumeSnapshotContentName == nil {
			// volumesnapshot hasn't been reconciled and we're not waiting for it.
			return nil, nil
		}
		vsc, err := snapshotClient.VolumeSnapshotContents().Get(context.TODO(), *volSnap.Status.BoundVolumeSnapshotContentName, metav1.GetOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "error getting volume snapshot content from API")
		}
		return vsc, nil
	}

	// We'll wait 10m for the VSC to be reconciled polling every 5s unless csiSnapshotTimeout is set
	timeout := defaultCSISnapshotTimeout
	if csiSnapshotTimeout > 0 {
		timeout = csiSnapshotTimeout
	}
	interval := 5 * time.Second
	var snapshotContent *snapshotv1api.VolumeSnapshotContent

	err := wait.PollUntilContextTimeout(context.Background(), interval, timeout, true, func(ctx context.Context) (bool, error) {
		vs, err := snapshotClient.VolumeSnapshots(volSnap.Namespace).Get(ctx, volSnap.Name, metav1.GetOptions{})
		if err != nil {
			return false, errors.Wrapf(err, fmt.Sprintf("failed to get volumesnapshot %s/%s", volSnap.Namespace, volSnap.Name))
		}

		if vs.Status == nil || vs.Status.BoundVolumeSnapshotContentName == nil {
			log.Infof("Waiting for CSI driver to reconcile volumesnapshot %s/%s. Retrying in %ds", volSnap.Namespace, volSnap.Name, interval/time.Second)
			return false, nil
		}

		snapshotContent, err = snapshotClient.VolumeSnapshotContents().Get(ctx, *vs.Status.BoundVolumeSnapshotContentName, metav1.GetOptions{})
		if err != nil {
			return false, errors.Wrapf(err, fmt.Sprintf("failed to get volumesnapshotcontent %s for volumesnapshot %s/%s", *vs.Status.BoundVolumeSnapshotContentName, vs.Namespace, vs.Name))
		}

		// we need to wait for the VolumeSnaphotContent to have a snapshot handle because during restore,
		// we'll use that snapshot handle as the source for the VolumeSnapshotContent so it's statically
		// bound to the existing snapshot.
		if snapshotContent.Status == nil || snapshotContent.Status.SnapshotHandle == nil {
			log.Infof("Waiting for volumesnapshotcontents %s to have snapshot handle. Retrying in %ds", snapshotContent.Name, interval/time.Second)
			if snapshotContent.Status != nil && snapshotContent.Status.Error != nil {
				log.Warnf("Volumesnapshotcontent %s has error: %v", snapshotContent.Name, *snapshotContent.Status.Error.Message)
			}
			return false, nil
		}

		return true, nil
	})

	if err != nil {
		if err == wait.ErrorInterrupted(errors.New("timed out waiting for the condition")) {
			if snapshotContent != nil && snapshotContent.Status != nil && snapshotContent.Status.Error != nil {
				log.Errorf("Timed out awaiting reconciliation of volumesnapshot, Volumesnapshotcontent %s has error: %v", snapshotContent.Name, *snapshotContent.Status.Error.Message)
				return nil, errors.Errorf("CSI got timed out with error: %v", *snapshotContent.Status.Error.Message)
			} else {
				log.Errorf("Timed out awaiting reconciliation of volumesnapshot %s/%s", volSnap.Namespace, volSnap.Name)
			}
		}
		return nil, err
	}

	return snapshotContent, nil
}

func GetClients() (*kubernetes.Clientset, snapshotterClientSet.Interface, error) {
	client, snapshotterClient, _, err := GetFullClients()

	return client, snapshotterClient, err
}

func GetFullClients() (*kubernetes.Clientset, snapshotterClientSet.Interface, crclient.Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	clientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	client, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	snapshotterClient, err := snapshotterClientSet.NewForConfig(clientConfig)
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	scheme := runtime.NewScheme()
	if err := velerov1api.AddToScheme(scheme); err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}
	if err := velerov2alpha1api.AddToScheme(scheme); err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	crClient, err := crclient.New(clientConfig, crclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, nil, nil, errors.WithStack(err)
	}

	return client, snapshotterClient, crClient, nil
}

// IsVolumeSnapshotClassHasListerSecret returns whether a volumesnapshotclass has a snapshotlister secret
func IsVolumeSnapshotClassHasListerSecret(vc *snapshotv1api.VolumeSnapshotClass) bool {
	// https://github.com/kubernetes-csi/external-snapshotter/blob/master/pkg/utils/util.go#L59-L60
	// There is no release w/ these constants exported. Using the strings for now.
	_, nameExists := vc.Annotations[PrefixedSnapshotterListSecretNameKey]
	_, nsExists := vc.Annotations[PrefixedSnapshotterListSecretNamespaceKey]
	return nameExists && nsExists
}

// IsVolumeSnapshotContentHasDeleteSecret returns whether a volumesnapshotcontent has a deletesnapshot secret
func IsVolumeSnapshotContentHasDeleteSecret(vsc *snapshotv1api.VolumeSnapshotContent) bool {
	// https://github.com/kubernetes-csi/external-snapshotter/blob/master/pkg/utils/util.go#L56-L57
	// use exported constants in the next release
	_, nameExists := vsc.Annotations[PrefixedSnapshotterSecretNameKey]
	_, nsExists := vsc.Annotations[PrefixedSnapshotterSecretNamespaceKey]
	return nameExists && nsExists
}

// IsVolumeSnapshotHasVSCDeleteSecret returns whether a volumesnapshot should set the deletesnapshot secret
// for the static volumesnapshotcontent that is created on restore
func IsVolumeSnapshotHasVSCDeleteSecret(vs *snapshotv1api.VolumeSnapshot) bool {
	_, nameExists := vs.Annotations[CSIDeleteSnapshotSecretName]
	_, nsExists := vs.Annotations[CSIDeleteSnapshotSecretNamespace]
	return nameExists && nsExists
}

// AddAnnotations adds the supplied key-values to the annotations on the object
func AddAnnotations(o *metav1.ObjectMeta, vals map[string]string) {
	if o.Annotations == nil {
		o.Annotations = make(map[string]string)
	}
	for k, v := range vals {
		o.Annotations[k] = v
	}
}

// AddLabels adds the supplied key-values to the labels on the object
func AddLabels(o *metav1.ObjectMeta, vals map[string]string) {
	if o.Labels == nil {
		o.Labels = make(map[string]string)
	}
	for k, v := range vals {
		o.Labels[k] = label.GetValidName(v)
	}
}

// IsVolumeSnapshotExists returns whether a specific volumesnapshot object exists.
func IsVolumeSnapshotExists(ns, name string, snapshotClient snapshotter.SnapshotV1Interface) bool {
	vs, err := snapshotClient.VolumeSnapshots(ns).Get(context.TODO(), name, metav1.GetOptions{})
	if err == nil && vs != nil {
		return true
	}

	return false
}

func SetVolumeSnapshotContentDeletionPolicy(vscName string, csiClient snapshotter.SnapshotV1Interface) error {
	pb := []byte(`{"spec":{"deletionPolicy":"Delete"}}`)
	_, err := csiClient.VolumeSnapshotContents().Patch(context.TODO(), vscName, types.MergePatchType, pb, metav1.PatchOptions{})

	return err
}

func HasBackupLabel(o *metav1.ObjectMeta, backupName string) bool {
	if o.Labels == nil || len(strings.TrimSpace(backupName)) == 0 {
		return false
	}
	return o.Labels[velerov1api.BackupNameLabel] == label.GetValidName(backupName)
}

func CleanupVolumeSnapshot(volSnap *snapshotv1api.VolumeSnapshot, snapshotClient snapshotter.SnapshotV1Interface, log logrus.FieldLogger) {
	log.Infof("Deleting Volumesnapshot %s/%s", volSnap.Namespace, volSnap.Name)
	vs, err := snapshotClient.VolumeSnapshots(volSnap.Namespace).Get(context.TODO(), volSnap.Name, metav1.GetOptions{})
	if err != nil {
		log.Debugf("Failed to get volumesnapshot %s/%s", volSnap.Namespace, volSnap.Name)
		return
	}

	if vs.Status != nil && vs.Status.BoundVolumeSnapshotContentName != nil {
		// we patch the DeletionPolicy of the volumesnapshotcontent to set it to Delete.
		// This ensures that the volume snapshot in the storage provider is also deleted.
		err := SetVolumeSnapshotContentDeletionPolicy(*vs.Status.BoundVolumeSnapshotContentName, snapshotClient)
		if err != nil {
			log.Debugf("Failed to patch DeletionPolicy of volume snapshot %s/%s", vs.Namespace, vs.Name)
		}
	}
	err = snapshotClient.VolumeSnapshots(vs.Namespace).Delete(context.TODO(), vs.Name, metav1.DeleteOptions{})
	if err != nil {
		log.Debugf("Failed to delete volumesnapshot %s/%s: %v", vs.Namespace, vs.Name, err)
	} else {
		log.Infof("Deleted volumesnapshot with volumesnapshotContent %s/%s", vs.Namespace, vs.Name)
	}
}

// deleteVolumeSnapshot is called by deleteVolumeSnapshots and handles the single VolumeSnapshot
// instance.
func DeleteVolumeSnapshot(vs snapshotv1api.VolumeSnapshot, vsc snapshotv1api.VolumeSnapshotContent,
	backup *velerov1api.Backup, snapshotClient snapshotter.SnapshotV1Interface, logger logrus.FieldLogger) {
	modifyVSCFlag := false
	if vs.Status != nil && vs.Status.BoundVolumeSnapshotContentName != nil && len(*vs.Status.BoundVolumeSnapshotContentName) > 0 {
		if vsc.Spec.DeletionPolicy == snapshotv1api.VolumeSnapshotContentDelete {
			modifyVSCFlag = true
		}
	} else {
		logger.Errorf("VolumeSnapshot %s/%s is not ready. This is not expected.", vs.Namespace, vs.Name)
	}

	// Change VolumeSnapshotContent's DeletionPolicy to Retain before deleting VolumeSnapshot,
	// because VolumeSnapshotContent will be deleted by deleting VolumeSnapshot, when
	// DeletionPolicy is set to Delete, but Velero needs VSC for cleaning snapshot on cloud
	// in backup deletion.
	if modifyVSCFlag {
		logger.Debugf("Patching VolumeSnapshotContent %s", vsc.Name)
		patchData := []byte(fmt.Sprintf(`{"spec":{"deletionPolicy":"%s"}}`, snapshotv1api.VolumeSnapshotContentRetain))
		updatedVSC, err := snapshotClient.VolumeSnapshotContents().Patch(context.Background(), vsc.Name, types.MergePatchType, patchData, metav1.PatchOptions{})
		if err != nil {
			logger.Errorf("fail to modify VolumeSnapshotContent %s DeletionPolicy to Retain: %s", vsc.Name, err.Error())
			return
		}

		defer func() {
			logger.Debugf("Start to recreate VolumeSnapshotContent %s", updatedVSC.Name)
			err := recreateVolumeSnapshotContent(*updatedVSC, backup, snapshotClient, logger)
			if err != nil {
				logger.Errorf("fail to recreate VolumeSnapshotContent %s: %s", updatedVSC.Name, err.Error())
			}
		}()
	}

	// Delete VolumeSnapshot from cluster
	logger.Debugf("Deleting VolumeSnapshot %s/%s", vs.Namespace, vs.Name)
	err := snapshotClient.VolumeSnapshots(vs.Namespace).Delete(context.TODO(), vs.Name, metav1.DeleteOptions{})
	if err != nil {
		logger.Errorf("fail to delete VolumeSnapshot %s/%s: %s", vs.Namespace, vs.Name, err.Error())
	}
}

// recreateVolumeSnapshotContent will delete then re-create VolumeSnapshotContent,
// because some parameter in VolumeSnapshotContent Spec is immutable, e.g. VolumeSnapshotRef
// and Source. Source is updated to let csi-controller thinks the VSC is statically provsisioned with VS.
// Set VolumeSnapshotRef's UID to nil will let the csi-controller finds out the related VS is gone, then
// VSC can be deleted.
func recreateVolumeSnapshotContent(vsc snapshotv1api.VolumeSnapshotContent, backup *velerov1api.Backup,
	snapshotClient snapshotter.SnapshotV1Interface, log logrus.FieldLogger) error {
	// Read resource timeout from backup annotation, if not set, use default value.
	timeout, err := time.ParseDuration(backup.Annotations[ResourceTimeoutAnnotation])
	if err != nil {
		log.Warnf("fail to parse resource timeout annotation %s: %s", backup.Annotations[ResourceTimeoutAnnotation], err.Error())
		timeout = 10 * time.Minute
	}
	log.Debugf("resource timeout is set to %s", timeout.String())
	interval := 1 * time.Second

	err = snapshotClient.VolumeSnapshotContents().Delete(context.TODO(), vsc.Name, metav1.DeleteOptions{})
	if err != nil {
		return errors.Wrapf(err, "fail to delete VolumeSnapshotContent: %s", vsc.Name)
	}

	// Check VolumeSnapshotContents is already deleted, before re-creating it.
	err = wait.PollUntilContextTimeout(context.Background(), interval, timeout, true, func(ctx context.Context) (bool, error) {
		_, err := snapshotClient.VolumeSnapshotContents().Get(ctx, vsc.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, errors.Wrapf(err, fmt.Sprintf("failed to get VolumeSnapshotContent %s", vsc.Name))
		}
		return false, nil
	})
	if err != nil {
		return errors.Wrapf(err, "fail to retrieve VolumeSnapshotContent %s info", vsc.Name)
	}

	// Make the VolumeSnapshotContent static
	vsc.Spec.Source = snapshotv1api.VolumeSnapshotContentSource{
		SnapshotHandle: vsc.Status.SnapshotHandle,
	}
	// Set VolumeSnapshotRef to none exist one, because VolumeSnapshotContent
	// validation webhook will check whether name and namespace are nil.
	// external-snapshotter needs Source pointing to snapshot and VolumeSnapshot
	// reference's UID to nil to determine the VolumeSnapshotContent is deletable.
	vsc.Spec.VolumeSnapshotRef = corev1api.ObjectReference{
		APIVersion: snapshotv1api.SchemeGroupVersion.String(),
		Kind:       "VolumeSnapshot",
		Namespace:  "ns-" + string(vsc.UID),
		Name:       "name-" + string(vsc.UID),
	}
	// ResourceVersion shouldn't exist for new creation.
	vsc.ResourceVersion = ""
	_, err = snapshotClient.VolumeSnapshotContents().Create(context.TODO(), &vsc, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrapf(err, "fail to create VolumeSnapshotContent %s", vsc.Name)
	}

	return nil
}

func DeleteVolumeSnapshotIfAny(ctx context.Context, snapshotClient snapshotterClientSet.Interface,
	vs snapshotv1api.VolumeSnapshot, log logrus.FieldLogger) {
	if err := snapshotClient.SnapshotV1().VolumeSnapshots(vs.Namespace).Delete(ctx, vs.Name, metav1.DeleteOptions{}); err != nil {
		log.WithError(err).Warnf("fail to delete VolumeSnapshot %s/%s", vs.Namespace, vs.Name)
	}
}
