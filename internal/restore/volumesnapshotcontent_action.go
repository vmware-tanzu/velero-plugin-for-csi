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

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// VolumeSnapshotContentRestoreItemAction is a restore item action plugin for Velero
type VolumeSnapshotContentRestoreItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating VolumeSnapshotContentRestoreItemAction action should be invoked while restoring
// volumesnapshotcontent.snapshot.storage.k8s.io resources
func (p *VolumeSnapshotContentRestoreItemAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshotcontent.snapshot.storage.k8s.io"},
	}, nil
}

// Execute restores a volumesnapshotcontent object without modification returning the snapshot lister secret, if any, as
// additional items to restore.
func (p *VolumeSnapshotContentRestoreItemAction) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.Log.Info("Starting VolumeSnapshotContentRestoreItemAction")
	var snapCont snapshotv1api.VolumeSnapshotContent

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &snapCont); err != nil {
		return &velero.RestoreItemActionExecuteOutput{}, errors.Wrapf(err, "failed to convert input.Item from unstructured")
	}

	additionalItems := []velero.ResourceIdentifier{}
	if util.IsVolumeSnapshotContentHasDeleteSecret(&snapCont) {
		additionalItems = append(additionalItems,
			velero.ResourceIdentifier{
				GroupResource: schema.GroupResource{Group: "", Resource: "secrets"},
				Name:          snapCont.Annotations[util.CSIDeleteSnapshotSecretName],
				Namespace:     snapCont.Annotations[util.CSIDeleteSnapshotSecretNamespace],
			},
		)
	}

	p.Log.Infof("Returning from VolumeSnapshotContentRestoreItemAction with %d additionalItems", len(additionalItems))
	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem:     input.Item,
		AdditionalItems: additionalItems,
	}, nil
}
