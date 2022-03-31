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

package restore

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// VolumeSnapshotClassRestoreItemAction is a Velero restore item action plugin for VolumeSnapshotClass
type VolumeSnapshotClassRestoreItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that VolumeSnapshotClassRestoreItemAction should be invoked while restoring
// volumesnapshotclass.snapshot.storage.k8s.io resrouces.
func (p *VolumeSnapshotClassRestoreItemAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshotclass.snapshot.storage.k8s.io"},
	}, nil
}

// Execute restores volumesnapshotclass objects returning any snapshotlister secret as additional items to restore
func (p *VolumeSnapshotClassRestoreItemAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.Log.Info("Starting VolumeSnapshotClassRestoreItemAction")

	var snapClass snapshotv1api.VolumeSnapshotClass

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &snapClass); err != nil {
		return &velero.RestoreItemActionExecuteOutput{}, errors.Wrapf(err, "failed to convert input.Item from unstructured")
	}

	additionalItems := []velero.ResourceIdentifier{}
	if util.IsVolumeSnapshotClassHasListerSecret(&snapClass) {
		additionalItems = append(additionalItems, velero.ResourceIdentifier{
			GroupResource: schema.GroupResource{Group: "", Resource: "secrets"},
			Name:          snapClass.Annotations[util.PrefixedSnapshotterListSecretNameKey],
			Namespace:     snapClass.Annotations[util.PrefixedSnapshotterListSecretNamespaceKey],
		})
	}

	p.Log.Infof("Returning from VolumeSnapshotClassRestoreItemAction with %d additionalItems", len(additionalItems))

	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem:     input.Item,
		AdditionalItems: additionalItems,
	}, nil
}
