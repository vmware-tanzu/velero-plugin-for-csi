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
	p.Log.Info("VolumeSnapshotBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshotcontent.snapshot.storage.k8s.io"},
	}, nil
}

// Execute backs up only those VolumeSnapshotContents that are not bound with any volumesnapshot object.
func (p *VolumeSnapshotContentBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.Log.Infof("Executing VolumeSnapshotContentBackupItemAction")

	var vsc snapshotv1beta1api.VolumeSnapshotContent
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &vsc); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	_, snapshotClient, err := util.GetClients()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	bound, err := util.IsVolumeSnapshotContentBound(&vsc, snapshotClient.SnapshotV1beta1())
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to verify volumesnapshot ref in volumesnapshot %s", vsc.Name)
	}

	additionalItems := []velero.ResourceIdentifier{}

	// we should backup the snapshot deletion secrets that may be referenced in the volumesnapshotcontent's annotation
	if util.IsVolumeSnapshotContentHasDeleteSecret(&vsc) {
		deleteSnapshotSecretName := vsc.Annotations[util.PrefixedSnapshotterSecretNameKey]
		deleteSnapshotSecretNamespace := vsc.Annotations[util.PrefixedSnapshotterSecretNamespaceKey]
		// TODO: add GroupResource for secret into kuberesource
		additionalItems = append(additionalItems, velero.ResourceIdentifier{
			GroupResource: schema.GroupResource{Group: "", Resource: "secrets"},
			Name:          deleteSnapshotSecretName,
			Namespace:     deleteSnapshotSecretNamespace,
		})
		p.Log.Infof("Found DeleteSnapshotSecret %s/%s on volumesnapshotcontent %s from its annotation", deleteSnapshotSecretNamespace, deleteSnapshotSecretName, vsc.Name)
	} else {
		p.Log.Infof("Volumesnapshotcontent %s does not have a deletesnapshot secret annotation", vsc.Name)
	}

	// volumesnapshotcontents that are not bound to a volumesnapshot exist in the cluster because their DeletionPolicy was set to Retain.
	// this means they still hold reference to a storage provider snapshot which will no longer be discoverable in the cluster if this volumesnapshotcontent
	// is not backed up.
	retItem := item

	if bound {
		// volumesnapshotcontents that are bound to a volumesnapshot object will not be explicitly backed up
		// they will be backed up in the process of backing up the volumesnapshot objects that they are bound to
		retItem = nil
	}

	p.Log.Infof("Returning %d additionalItems to backup", len(additionalItems))
	for _, ai := range additionalItems {
		p.Log.Debugf("%s: %s", ai.GroupResource.String(), ai.Name)
	}
	return retItem, additionalItems, nil
}
