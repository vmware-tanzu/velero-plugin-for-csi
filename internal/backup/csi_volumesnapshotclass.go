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

package backup

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	snapshotv1beta1api "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// VolumeSnapshotClassBackupItemAction is a backup item action plugin to backup
// CSI VolumeSnapshotclass objects using Velero
type VolumeSnapshotClassBackupItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the VolumeSnapshotClassBackupItemAction action should be invoked to backup volumesnapshotclass.
func (p *VolumeSnapshotClassBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VolumeSnapshotClassBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshotclass.snapshot.storage.k8s.io"},
	}, nil
}

// Execute backsup a VolumeSnapshotClass object and returns as additional items any snapshot lister secret that may be referenced in its annotations.
func (p *VolumeSnapshotClassBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.Log.Infof("Executing VolumeSnapshotClassBackupItemAction")

	var vsc snapshotv1beta1api.VolumeSnapshotClass
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &vsc); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	additionalItems := []velero.ResourceIdentifier{}

	p.Log.Infof("Returning %d additionalItems to backup", len(additionalItems))
	for _, ai := range additionalItems {
		p.Log.Debugf("%s: %s", ai.GroupResource.String(), ai.Name)
	}

	return item, additionalItems, nil
}
