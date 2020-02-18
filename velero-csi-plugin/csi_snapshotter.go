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
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/clientcmd"

	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
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
func (p *CSISnapshotter) Execute(item runtime.Unstructured, backup *v1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.log.Info("CSISnapshotter Execute")

	var pvc corev1api.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pvc); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// no storage class: we don't know how to map to a VolumeSnapshotClass
	if pvc.Spec.StorageClassName == nil {
		return item, nil, nil
	}

	client, snapshotClient, err := getClients()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	storageClass, err := client.StorageV1().StorageClasses().Get(*pvc.Spec.StorageClassName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "error getting storage class")
	}

	snapshotClasses, err := snapshotClient.SnapshotV1beta1().VolumeSnapshotClasses().List(metav1.ListOptions{})
	if err != nil {
		return nil, nil, errors.Wrap(err, "error listing snapshot classes")
	}

	var snapshotClass *snapshotv1beta1api.VolumeSnapshotClass
	for i := range snapshotClasses.Items {
		if snapshotClasses.Items[i].Driver == storageClass.Provisioner {
			snapshotClass = &snapshotClasses.Items[i]
		}
	}

	if snapshotClass == nil {
		return nil, nil, errors.Errorf("no volume snapshot class found for %s", storageClass.Provisioner)
	}

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
	annotations["velero.io/volume-snapshot-name"] = upd.Name
	accessor.SetAnnotations(annotations)

	return item, nil, nil
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
