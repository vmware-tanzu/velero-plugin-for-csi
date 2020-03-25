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

	snapshotv1beta1api "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// VSRestorer is a restore item action for VolumeSnapshots
type VSRestorer struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that VSRestorer action should be invoked while restoring
// volumesnapshots.snapshot.storage.k8s.io resrouces.
func (p *VSRestorer) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshots.snapshot.storage.k8s.io"},
	}, nil
}

func resetVolumeSnapshotSpecForRestore(vs *snapshotv1beta1api.VolumeSnapshot, vscName *string) {
	// Spec of the backed-up object used the PVC as the source of the volumeSnapshot.
	// Restore operation will however, restore the volumesnapshot from the volumesnapshotcontent
	vs.Spec.Source.PersistentVolumeClaimName = nil
	vs.Spec.Source.VolumeSnapshotContentName = vscName
}

// Execute modifies the VolumeSnapshot's spec to set its data source to be the relevant VolumeSnapshotContent object,
// rather than the original PVC the snapshot was created from, so it can be statically re-bound to the
// volumesnapshotcontent on restore
func (p *VSRestorer) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.Log.Info("Starting VSRestorerAction")
	var vs snapshotv1beta1api.VolumeSnapshot

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &vs); err != nil {
		return &velero.RestoreItemActionExecuteOutput{}, errors.Wrapf(err, "failed to convert input.Item from unstructured")
	}

	labels := vs.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	snapHandle, exists := labels[util.VolumeSnapshotHandleLabel]
	if !exists {
		return nil, errors.Errorf("Volumesnapshot %s/%s does not have a %s label", vs.Namespace, vs.Name, util.VolumeSnapshotHandleLabel)
	}
	// TODO: refactor to reduce copy-pasta
	csiDriverName, exists := labels[util.CSIDriverNameLabel]
	if !exists {
		return nil, errors.Errorf("Volumesnapshot %s/%s does not have a %s label", vs.Namespace, vs.Name, util.CSIDriverNameLabel)
	}

	// TODO: generated name will be like velero-velero-something. Fix that.
	vsc := snapshotv1beta1api.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "velero-" + vs.Name + "-",
			Annotations: map[string]string{
				velerov1api.RestoreNameLabel: input.Restore.Name,
			},
		},
		Spec: snapshotv1beta1api.VolumeSnapshotContentSpec{
			DeletionPolicy: snapshotv1beta1api.VolumeSnapshotContentDelete,
			Driver:         csiDriverName,
			VolumeSnapshotRef: core_v1.ObjectReference{
				Kind:      "VolumeSnapshot",
				Namespace: vs.Namespace,
				Name:      vs.Name,
			},
			Source: snapshotv1beta1api.VolumeSnapshotContentSource{
				SnapshotHandle: &snapHandle,
			},
		},
	}

	// Set Delete snapshot secret annotations if it was present during backup of the dynamic volumesnapshot
	if util.IsVolumeSnapshotHasVSCDeleteSecret(&vs) {
		vsc.Annotations[util.PrefixedSnapshotterSecretNameKey] = vs.Annotations[util.CSIDeleteSnapshotSecretName]
		vsc.Annotations[util.PrefixedSnapshotterSecretNamespaceKey] = vs.Annotations[util.CSIDeleteSnapshotSecretNamespace]
	}

	_, snapClient, err := util.GetClients()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	vscupd, err := snapClient.SnapshotV1beta1().VolumeSnapshotContents().Create(&vsc)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create volumesnapshotcontents %s", vsc.GenerateName)
	}
	p.Log.Infof("Created VolumesnapshotContents %s with static binding to volumesnapshot %s/%s", vscupd, vs.Namespace, vs.Name)

	// Reset Spec to convert the dynamic snapshot to a static one.
	resetVolumeSnapshotSpecForRestore(&vs, &vscupd.Name)
	p.Log.Infof("VS Info: Source: snapHandle: %s, pvc==nil? %t", *vs.Spec.Source.VolumeSnapshotContentName, vs.Spec.Source.PersistentVolumeClaimName == nil)

	vsMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&vs)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	p.Log.Info("Returning from VSRestorerAction")

	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem: &unstructured.Unstructured{Object: vsMap},
	}, nil
}
