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
	p.Log.Info("VolumeSnapshotBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshots.snapshot.storage.k8s.io"},
	}, nil
}

func setVolumeSnapshotAnnotations(vs *snapshotv1beta1api.VolumeSnapshot, vals map[string]string) {
	if vs.Annotations == nil {
		vs.Annotations = make(map[string]string)
	}

	for k, v := range vals {
		vs.Annotations[k] = v
	}
}

// Execute backsup a CSI volumesnapshot object and captures, as labels and annotations, information from its associated volumesnapshotcontents such as CSI driver name, storage snapshot handle
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

	p.Log.Infof("Getting VolumesnapshotContent for Volumesnapshot %s/%s", vs.Namespace, vs.Name)
	vsc, err := util.GetVolumeSnapshotContentForVolumeSnapshot(&vs, snapshotClient.SnapshotV1beta1(), p.Log)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// Capture storage provider snapshot handle and CSI driver name
	// to be used on restore to create a static volumesnapshotcontent that will be the source of the volumesnapshot.
	vals := map[string]string{
		util.VolumeSnapshotHandleLabel: *vsc.Status.SnapshotHandle,
		util.CSIDriverNameLabel:        vsc.Spec.Driver,
	}

	// save newly applied annotations into the backed-up volumesnapshot item
	setVolumeSnapshotAnnotations(&vs, vals)

	vsMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&vs)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	additionalItems := []velero.ResourceIdentifier{
		{
			GroupResource: kuberesource.VolumeSnapshotClasses,
			Name:          *vs.Spec.VolumeSnapshotClassName,
		},
		{
			GroupResource: kuberesource.VolumeSnapshotContents,
			Name:          vsc.Name,
		},
	}
	p.Log.Infof("Returning %d additionalItems to backup", len(additionalItems))
	for _, ai := range additionalItems {
		p.Log.Debugf("%s: %s", ai.GroupResource.String(), ai.Name)
	}

	return &unstructured.Unstructured{Object: vsMap}, additionalItems, nil
}
