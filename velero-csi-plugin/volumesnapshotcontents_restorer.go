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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// VSCRestorer is a restore item action plugin for Velero
type VSCRestorer struct {
	log logrus.FieldLogger
}

// AppliesTo returns information about which resources the VSCRestorer action should be invoked for.
// VSCRestorer RestoreItemAction plugin's Execute function will only be invoked on items that match the returned
// selector. A zero-valued ResourceSelector matches all resources.
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

// Execute allows the RestorePlugin to perform arbitrary logic with the item being restored,
// in this case, logic to correctly restore a CSI volumesnapshotcontent custom resource that represents a snapshot of a CSI backed volume.
func (p *VSCRestorer) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.log.Info("Starting VSCRestorerAction")
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

	p.log.Info("Returning from VSCRestorerAction")

	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem: &unstructured.Unstructured{Object: vscMap},
	}, nil
}
