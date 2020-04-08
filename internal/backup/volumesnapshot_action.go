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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/kuberesource"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// VolumeSnapshotBackupItemAction is a backup item action plugin to backup
// CSI VolumeSnapshot objects using Velero
type VolumeSnapshotBackupItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the VolumeSnapshotBackupItemAction should be invoked to backup volumesnapshots.
func (p *VolumeSnapshotBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Debug("VolumeSnapshotBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshots.snapshot.storage.k8s.io"},
	}, nil
}

// Execute backs up a CSI volumesnapshot object and captures, as labels and annotations, information from its associated volumesnapshotcontents such as CSI driver name, storage snapshot handle
// and namespace and name of the snapshot delete secret, if any. It returns the volumesnapshotclass and the volumesnapshotcontents as additional items to be backed up.
func (p *VolumeSnapshotBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.Log.Infof("Executing VolumeSnapshotBackupItemAction")

	var vs snapshotv1beta1api.VolumeSnapshot
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &vs); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	_, snapshotClient, err := util.GetClients()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	additionalItems := []velero.ResourceIdentifier{
		{
			GroupResource: kuberesource.VolumeSnapshotClasses,
			Name:          *vs.Spec.VolumeSnapshotClassName,
		},
	}

	// determine if we are backing up a volumesnapshot that was created by velero while performing backup of a
	// CSI backed PVC.
	// For volumesnapshots that were created during the backup of a CSI backed PVC, we will wait for the volumecontents to
	// be available.
	// For volumesnapshots created outside of velero, we expect the volumesnapshotcontent to be available prior to backing up
	// the volumesnapshot. In case of a failure, backup should be re-attempted after the CSI driver has reconciled the volumesnapshot.
	// existence of the velerov1api.BackupNameLabel indicates that the volumesnapshot was created while backing up a
	// CSI backed PVC.

	// TODO: use UID of the backup object instead of just looking for a match in the backup name. This will ensure we don't wait in cases where
	// backup names are reused.
	// We want to await reconciliation of only those volumesnapshots created during the ongoing backup.
	// For this we will wait only if the backup label exists on the volumesnapshot object and the
	// backup name is the same as that of the value of the backupLabel
	shouldWait := vs.Labels[velerov1api.BackupNameLabel] == backup.Name

	p.Log.Infof("Getting VolumesnapshotContent for Volumesnapshot %s/%s", vs.Namespace, vs.Name)

	vsc, err := util.GetVolumeSnapshotContentForVolumeSnapshot(&vs, snapshotClient.SnapshotV1beta1(), p.Log, shouldWait)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	if vsc != nil {
		// when we are backing up volumesnapshots created outside of velero, we will not
		// await volumesnapshot reconciliation and in this case GetVolumeSnapshotContentForVolumeSnapshot
		// may not find the associated volumesnapshotcontents to add to the backup.
		// This is not an error encountered in the backup process. So we add the volumesnapshotcontent
		// to the backup only if one is found.
		additionalItems = append(additionalItems, velero.ResourceIdentifier{
			GroupResource: kuberesource.VolumeSnapshotContents,
			Name:          vsc.Name,
		})

		if vsc.Status != nil && vsc.Status.SnapshotHandle != nil {
			// Capture storage provider snapshot handle and CSI driver name
			// to be used on restore to create a static volumesnapshotcontent that will be the source of the volumesnapshot.
			vals := map[string]string{
				util.VolumeSnapshotHandleAnnotation: *vsc.Status.SnapshotHandle,
				util.CSIDriverNameAnnotation:        vsc.Spec.Driver,
			}
			// save newly applied annotations into the backed-up volumesnapshot item
			util.AddAnnotations(&vs.ObjectMeta, vals)
		}
	}

	vsMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&vs)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	p.Log.Infof("Returning from VolumeSnapshotBackupItemAction with %d additionalItems to backup", len(additionalItems))
	for _, ai := range additionalItems {
		p.Log.Debugf("%s: %s", ai.GroupResource.String(), ai.Name)
	}

	return &unstructured.Unstructured{Object: vsMap}, additionalItems, nil
}
