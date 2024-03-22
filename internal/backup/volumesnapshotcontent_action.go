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
	"fmt"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v7/apis/volumesnapshot/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// VolumeSnapshotContentBackupItemAction is a backup item action plugin to backup
// CSI VolumeSnapshotcontent objects using Velero
type VolumeSnapshotContentBackupItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the VolumeSnapshotContentBackupItemAction action should be invoked to backup volumesnapshotcontents.
func (p *VolumeSnapshotContentBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Debug("VolumeSnapshotContentBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshotcontent.snapshot.storage.k8s.io"},
	}, nil
}

// Execute returns the unmodified volumesnapshotcontent object along with the snapshot deletion secret, if any, from its annotation
// as additional items to backup.
func (p *VolumeSnapshotContentBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, string, []velero.ResourceIdentifier, error) {
	p.Log.Infof("Executing VolumeSnapshotContentBackupItemAction")

	if backup.Status.Phase == velerov1api.BackupPhaseFinalizing || backup.Status.Phase == velerov1api.BackupPhaseFinalizingPartiallyFailed {
		p.Log.WithField("Backup", fmt.Sprintf("%s/%s", backup.Namespace, backup.Name)).
			WithField("BackupPhase", backup.Status.Phase).Debug("Skipping VolumeSnapshotContentBackupItemAction as backup is in finalizing phase.")
		return item, nil, "", nil, nil
	}

	var snapCont snapshotv1api.VolumeSnapshotContent
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &snapCont); err != nil {
		return nil, nil, "", nil, errors.WithStack(err)
	}

	additionalItems := []velero.ResourceIdentifier{}

	// we should backup the snapshot deletion secrets that may be referenced in the volumesnapshotcontent's annotation
	if util.IsVolumeSnapshotContentHasDeleteSecret(&snapCont) {
		// TODO: add GroupResource for secret into kuberesource
		additionalItems = append(additionalItems, velero.ResourceIdentifier{
			GroupResource: schema.GroupResource{Group: "", Resource: "secrets"},
			Name:          snapCont.Annotations[util.PrefixedSnapshotterSecretNameKey],
			Namespace:     snapCont.Annotations[util.PrefixedSnapshotterSecretNamespaceKey],
		})

		util.AddAnnotations(&snapCont.ObjectMeta, map[string]string{
			util.MustIncludeAdditionalItemAnnotation: "true",
		})
	}

	snapContMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&snapCont)
	if err != nil {
		return nil, nil, "", nil, errors.WithStack(err)
	}

	p.Log.Infof("Returning from VolumeSnapshotContentBackupItemAction with %d additionalItems to backup", len(additionalItems))
	return &unstructured.Unstructured{Object: snapContMap}, additionalItems, "", nil, nil
}

func (p *VolumeSnapshotContentBackupItemAction) Name() string {
	return "VolumeSnapshotContentBackupItemAction"
}

func (p *VolumeSnapshotContentBackupItemAction) Progress(operationID string, backup *velerov1api.Backup) (velero.OperationProgress, error) {
	return velero.OperationProgress{}, nil
}

func (p *VolumeSnapshotContentBackupItemAction) Cancel(operationID string, backup *velerov1api.Backup) error {
	// CSI Specification doesn't support canceling a snapshot creation.
	return nil
}
