/*
Copyright 2018, 2019, 2020 the Velero contributors.

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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// VSRestorer is a restore item action for VolumeSnapshots
type VSRestorer struct {
	log logrus.FieldLogger
}

// AppliesTo returns information indicating that VSRestorer action should be invoked while restoring
// volumesnapshots.snapshot.storage.k8s.io resrouces.
func (p *VSRestorer) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshots.snapshot.storage.k8s.io"},
	}, nil
}

func resetVolumeSnapshotSpecForRestore(vs *snapshotv1beta1api.VolumeSnapshot, vscName *string) {
	// Spec of the backed-up object used the PVC as the source of the volumeSnapshot.
	// Restore operation will however, restore the volumesnapshot from the volumesnapshotcontent
	vs.Spec.Source.PersistentVolumeClaimName = nil
	vs.Spec.Source.VolumeSnapshotContentName = vscName
}

// Execute modifies the VolumeSnapshot's spec to set its data source to be the relevant VolumeSnapshotContent object,
// rather than the original PVC the snapshot was created from, so it can be statically re-bound to the
// volumesnapshotcontent on restore
func (p *VSRestorer) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.log.Info("Starting VSRestorerAction")
	var vs snapshotv1beta1api.VolumeSnapshot

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &vs); err != nil {
		return &velero.RestoreItemActionExecuteOutput{}, errors.Wrapf(err, "failed to convert input.Item from unstructured")
	}

	var vsFromBackup snapshotv1beta1api.VolumeSnapshot
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured((input.ItemFromBackup.UnstructuredContent()), &vsFromBackup); err != nil {
		return &velero.RestoreItemActionExecuteOutput{}, errors.Wrapf(err, "failed to convert input.ItemFromBackup from unstructured")
	}

	if vsFromBackup.Status == nil || vsFromBackup.Status.BoundVolumeSnapshotContentName == nil {
		return &velero.RestoreItemActionExecuteOutput{}, errors.Errorf("unable to lookup BoundVolumeSnapshotContentName from status")
	}

	resetVolumeSnapshotSpecForRestore(&vs, vsFromBackup.Status.BoundVolumeSnapshotContentName)

	vsMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&vs)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	p.log.Info("Returning from VSRestorerAction")

	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem: &unstructured.Unstructured{Object: vsMap},
	}, nil
}
