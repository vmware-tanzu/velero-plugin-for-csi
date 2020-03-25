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
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// VolumeSnapshotBackupItemAction is a backup item action plugin to backup
// CSI VolumeSnapshot objects using Velero
type VolumeSnapshotBackupItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the VolumeSnapshotBackupItemAction should be invoked to backup volumesnapshots.
func (p *VolumeSnapshotBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VolumeSnapshotBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshots.snapshot.storage.k8s.io"},
	}, nil
}

func setVolumeSnapshotAnnotationsAndLabels(vs *snapshotv1beta1api.VolumeSnapshot, vals map[string]string) {
	if vs.Annotations == nil {
		vs.Annotations = make(map[string]string)
	}
	if vs.Labels == nil {
		vs.Labels = make(map[string]string)
	}

	for k, v := range vals {
		vs.Annotations[k] = v
		vs.Labels[k] = v
	}
}

// Execute backs VolumeSnapshotContents associated with the volumesnapshot as additional resource.
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
	p.Log.Infof("Getting VolumesnapshotContent for Volumesnapshot %s/%s", vs.Namespace, vs.Name)
	vsc, err := util.GetVolumeSnapshotContentForVolumeSnapshot(&vs, snapshotClient.SnapshotV1beta1(), p.Log)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	vals := map[string]string{
		util.VolumeSnapshotHandleLabel: *vsc.Status.SnapshotHandle,
		util.CSIDriverNameLabel:        vsc.Spec.Driver,
	}

	additionalItems := []velero.ResourceIdentifier{}

	// Capture the vsc's deletesnapshot secret annotations into the backup of the volumesnapshot
	if util.IsVolumeSnapshotContentHasDeleteSecret(vsc) {
		deleteSnapshotSecretName := vsc.Annotations[util.PrefixedSnapshotterSecretNameKey]
		deleteSnapshotSecretNamespace := vsc.Annotations[util.PrefixedSnapshotterSecretNamespaceKey]
		// TODO: add GroupResource for secret into kuberesource
		additionalItems = append(additionalItems, velero.ResourceIdentifier{
			GroupResource: schema.GroupResource{Group: "", Resource: "secrets"},
			Name:          deleteSnapshotSecretName,
			Namespace:     deleteSnapshotSecretNamespace,
		})

		vals[util.CSIDeleteSnapshotSecretName] = deleteSnapshotSecretName
		vals[util.CSIDeleteSnapshotSecretNamespace] = deleteSnapshotSecretNamespace

		p.Log.Infof("Found DeleteSnapshotSecret %s/%s on volumesnapshotcontent %s from its annotation", deleteSnapshotSecretNamespace, deleteSnapshotSecretName, vsc.Name)
	} else {
		p.Log.Infof("Volumesnapshotcontent %s does not have a deletesnapshot secret annotation", vsc.Name)
	}

	// save newly applied annotations and labels into the backed-up volumesnapshot
	setVolumeSnapshotAnnotationsAndLabels(&vs, vals)

	vsMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&vs)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	p.Log.Infof("Returning %d additionalItems to backup", len(additionalItems))
	for _, ai := range additionalItems {
		p.Log.Debugf("%s: %s", ai.GroupResource.String(), ai.Name)
	}

	return &unstructured.Unstructured{Object: vsMap}, additionalItems, nil
}
