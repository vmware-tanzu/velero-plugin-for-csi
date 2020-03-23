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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	snapshotv1beta1api "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// VSCRestorer is a restore item action plugin for Velero
type VSCRestorer struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating VSCRestorer action should be invoked while restoring
// volumesnapshotcontent.snapshot.storage.k8s.io resources
func (p *VSCRestorer) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshotcontent.snapshot.storage.k8s.io"},
	}, nil
}

func resetVSCSpecForRestore(vsc *snapshotv1beta1api.VolumeSnapshotContent, snapshotHandle *string) {
	// Spec of the backed-up object used the VolumeHandle as the source of the volumeSnapshotcontent.
	// Restore operation will however, restore the volumesnapshotcontent using the snapshotHandle as the source.
	vsc.Spec.DeletionPolicy = snapshotv1beta1api.VolumeSnapshotContentRetain
	vsc.Spec.Source.VolumeHandle = nil
	vsc.Spec.Source.SnapshotHandle = snapshotHandle
	vsc.Spec.VolumeSnapshotRef.UID = ""
	vsc.Spec.VolumeSnapshotRef.ResourceVersion = ""
}

// Execute modifies the volumesnapshotcontent's spec to use the storage provider snapshot handle as its source
// instead of the storage provider volume handle, as in the original spec, allowing the newly provisioned volume to be
// pre-populated with data from the volume snapshot.
func (p *VSCRestorer) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.Log.Info("Starting VSCRestorerAction")
	var vsc snapshotv1beta1api.VolumeSnapshotContent

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &vsc); err != nil {
		return &velero.RestoreItemActionExecuteOutput{}, errors.Wrapf(err, "failed to convert input.Item from unstructured")
	}

	var vscFromBackup snapshotv1beta1api.VolumeSnapshotContent
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured((input.ItemFromBackup.UnstructuredContent()), &vscFromBackup); err != nil {
		return &velero.RestoreItemActionExecuteOutput{}, errors.Wrapf(err, "failed to convert input.ItemFromBackup from unstructured")
	}

	if vscFromBackup.Status == nil || vscFromBackup.Status.SnapshotHandle == nil {
		return &velero.RestoreItemActionExecuteOutput{}, errors.Errorf("unable to lookup snapshotHandle from status")
	}

	resetVSCSpecForRestore(&vsc, vscFromBackup.Status.SnapshotHandle)

	vscMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&vsc)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	p.Log.Info("Returning from VSCRestorerAction")

	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem: &unstructured.Unstructured{Object: vscMap},
	}, nil
}
