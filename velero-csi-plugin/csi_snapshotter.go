/*
Copyright 2017, 2019 the Velero contributors.

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

package main

import (
	snapshotv1beta1api "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	snapshotter "github.com/kubernetes-csi/external-snapshotter/v2/pkg/client/clientset/versioned"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	"github.com/vmware-tanzu/velero/pkg/util/boolptr"
)

const (
	veleroSnapshotNameAnnotation = "velero.io/volume-snapshot-name"
)

// CSISnapshotter is a backup item action plugin for Velero.
type CSISnapshotter struct {
	log logrus.FieldLogger
}

// AppliesTo returns information about which resources this action should be invoked for.
// A BackupPlugin's Execute function will only be invoked on items that match the returned
// selector. A zero-valued ResourceSelector matches all resources.
func (p *CSISnapshotter) AppliesTo() (velero.ResourceSelector, error) {
	p.log.Info("CSISnapshotter AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"persistentvolumeclaims"},
	}, nil
}

// Execute allows the ItemAction to perform arbitrary logic with the item being backed up,
// in this case, setting a custom annotation on the item being backed up.
func (p *CSISnapshotter) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.log.Info("CSISnapshotter Execute")

	// Do nothing if volume snapshots have not been requested in this backup
	if boolptr.IsSetToFalse(backup.Spec.SnapshotVolumes) {
		p.log.Infof("Volume snapshotting not requested for backup %s/%s", backup.Namespace, backup.Name)
		return item, nil, nil
	}

	var pvc corev1api.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pvc); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	client, snapshotClient, err := getClients()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// Do nothing if restic is used to backup this PV
	isResticUsed, err := isPVCBackedUpByRestic(pvc.Namespace, pvc.Name, client.CoreV1())
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if isResticUsed {
		p.log.Infof("Nothing to do for PVC %s/%s is backed up using restic")
		return item, nil, nil
	}

	// Do nothing if this is not a CSI provisioned volume
	pv, err := getPVForPVC(&pvc, client.CoreV1())
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	if pv.Spec.PersistentVolumeSource.CSI == nil {
		p.log.Infof("Nothing to do for PVC %s/%s, PV %s is not a CSI volume", pvc.Namespace, pvc.Name, pv.Name)
		return item, nil, nil
	}

	// no storage class: we don't know how to map to a VolumeSnapshotClass
	if pvc.Spec.StorageClassName == nil {
		return item, nil, errors.Errorf("Cannot snpashot PVC %s/%s, PVC has no storage class.", pvc.Namespace, pvc.Name)
	}

	storageClass, err := client.StorageV1().StorageClasses().Get(*pvc.Spec.StorageClassName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting storage class")
	}

	snapshotClass, err := getVolumeSnapshotClassForStorageClass(storageClass.Provisioner, snapshotClient.SnapshotV1beta1())
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get volumesnapshotclass for storageclass %s", storageClass.Name)
	}

	// Craft the snapshot object to be created
	snapshot := snapshotv1beta1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "velero-" + pvc.Name + "-",
		},
		Spec: snapshotv1beta1api.VolumeSnapshotSpec{
			Source: snapshotv1beta1api.VolumeSnapshotSource{
				PersistentVolumeClaimName: &pvc.Name,
			},
			VolumeSnapshotClassName: &snapshotClass.Name,
		},
	}

	upd, err := snapshotClient.SnapshotV1beta1().VolumeSnapshots(pvc.Namespace).Create(&snapshot)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error creating volume snapshot")
	}

	accessor, err := meta.Accessor(item)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	annotations := accessor.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[veleroSnapshotNameAnnotation] = upd.Name
	annotations[velerov1api.BackupNameLabel] = backup.Name
	accessor.SetAnnotations(annotations)

	//TODO: add retry logic here. CSI driver may take time to reconcile the snapshotcontents object
	snapshotContents, err := getVolumeSnapshotContentForVolumeSnapshot(snapshotClient.SnapshotV1beta1(), upd.Name)

	// TODO: Create GroupResource obejects for volumesnapshot objects
	additionalItems := []velero.ResourceIdentifier{
		{
			GroupResource: schema.GroupResource{
				Group:    snapshotv1beta1api.GroupName,
				Resource: "volumesnapshotclasses",
			},
			Name: snapshotClass.Name,
		},
		{
			GroupResource: schema.GroupResource{
				Group:    snapshotv1beta1api.GroupName,
				Resource: "volumesnapshots",
			},
			Namespace: pvc.Namespace,
			Name:      upd.Name,
		},
		{
			GroupResource: schema.GroupResource{
				Group:    snapshotv1beta1api.GroupName,
				Resource: "volumesnapshotcontents",
			},
			Name: snapshotContents.Name,
		},
	}

	return item, additionalItems, nil
}

func getClients() (*kubernetes.Clientset, *snapshotter.Clientset, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	clientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	client, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	snapshotterClient, err := snapshotter.NewForConfig(clientConfig)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	return client, snapshotterClient, nil
}
