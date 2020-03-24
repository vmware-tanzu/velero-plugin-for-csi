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
	"time"

	snapshotv1beta1api "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/kuberesource"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	"k8s.io/apimachinery/pkg/runtime"
)

// VolumeSnapshotBackupItemAction is a backup item action plugin to backup
// CSI VolumeSnapshot objects using Velero
type VolumeSnapshotBackupItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the CSISnapshotter action should be invoked to backup volumesnapshots.
func (p *VolumeSnapshotBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VolumeSnapshotBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshots.snapshot.storage.k8s.io"},
	}, nil
}

// Execute backs VolumeSnapshotContents associated with the volumesnapshot as additional resource.
func (p *VolumeSnapshotBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.Log.Infof("Executing VolumeSnapshotBackupItemAction")

	var vs snapshotv1beta1api.VolumeSnapshot
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &vs); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	if vs.Status == nil || vs.Status.BoundVolumeSnapshotContentName == nil {
		p.Log.Infof("VolumeSnapshot %s/%s has not yet been reconciled", vs.Namespace, vs.Name)
		_, snapshotClient, err := util.GetClients()
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}
		util.WaitForVolumesnapshotReconcile(&vs, p.Log, snapshotClient.SnapshotV1beta1(), 5*time.Second)
	}

	additionalItems := []velero.ResourceIdentifier{
		{
			GroupResource: kuberesource.VolumeSnapshotContents,
			Name:          *vs.Status.BoundVolumeSnapshotContentName,
		},
	}

	return item, additionalItems, nil
}
