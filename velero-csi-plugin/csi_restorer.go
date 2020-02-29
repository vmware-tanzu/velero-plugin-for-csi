/*
Copyright 2018, 2019 the Velero contributors.

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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	snapshotv1beta1api "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	core_v1 "k8s.io/api/core/v1"
	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// CSIRestorer is a restore item action plugin for Velero
type CSIRestorer struct {
	log logrus.FieldLogger
}

// AppliesTo returns information about which resources this action should be invoked for.
// A RestoreItemAction's Execute function will only be invoked on items that match the returned
// selector. A zero-valued ResourceSelector matches all resources.g
func (p *CSIRestorer) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"persistentvolumeclaims"},
	}, nil
}

// Execute allows the RestorePlugin to perform arbitrary logic with the item being restored,
// in this case, setting a custom annotation on the item being restored.
func (p *CSIRestorer) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	var pvc corev1api.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &pvc); err != nil {
		return nil, errors.WithStack(err)
	}
	metadata, err := meta.Accessor(input.Item)
	if err != nil {
		return &velero.RestoreItemActionExecuteOutput{}, err
	}

	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	volumeSnapshotName, ok := annotations["velero.io/volume-snapshot-name"]
	if !ok {
		return nil, errors.Errorf("Could not find volume snapshot name on PVC")
	}

	sc := *pvc.Spec.StorageClassName
	toRestore := corev1api.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
		},
		Spec: corev1api.PersistentVolumeClaimSpec{
			StorageClassName: &sc,
			DataSource: &core_v1.TypedLocalObjectReference{
				APIGroup: &snapshotv1beta1api.SchemeGroupVersion.Group,
				Kind:     "VolumeSnapshot",
				Name:     volumeSnapshotName,
			},
			AccessModes: pvc.Spec.DeepCopy().AccessModes,
			Resources:   pvc.DeepCopy().Spec.Resources,
		},
	}

	pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&toRestore)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem: &unstructured.Unstructured{Object: pvcMap},
	}, nil
}
