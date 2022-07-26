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
	"context"
	"fmt"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	datamoverv1alpha1 "github.com/konveyor/volume-snapshot-mover/api/v1alpha1"
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
func (p *VolumeSnapshotContentBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.Log.Infof("Executing VolumeSnapshotContentBackupItemAction")

	var snapCont snapshotv1api.VolumeSnapshotContent
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &snapCont); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	additionalItems := []velero.ResourceIdentifier{}

	// Create VolumeSnapshotBackup CR per VolumeSnapshotContent and add it as an additional item if its a DataMover case
	if util.DataMoverCase() {
		_, snapshotClient, err := util.GetClients()
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}

		// Wait for VSC to be in ready state
		VSCReady, err := util.WaitForVolumeSnapshotContentToBeReady(snapCont, snapshotClient.SnapshotV1(), p.Log)

		if err != nil {
			return nil, nil, errors.WithStack(err)
		}

		if !VSCReady {
			p.Log.Infof("volumesnapshotcontent not in ready state, still continuing with the backup")
		}

		// get secret name created by data mover controller
		resticSecretName, err := util.GetDataMoverCredName(backup, backup.Namespace, p.Log)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}

		// craft a VolumeBackupSnapshot object to be created
		vsb := datamoverv1alpha1.VolumeSnapshotBackup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprint("vsb-" + snapCont.Spec.VolumeSnapshotRef.Name),
				Namespace: snapCont.Spec.VolumeSnapshotRef.Namespace,
				Labels: map[string]string{
					util.BackupNameLabel: backup.Name,
				},
			},
			Spec: datamoverv1alpha1.VolumeSnapshotBackupSpec{
				VolumeSnapshotContent: corev1api.ObjectReference{
					Name: snapCont.Name,
				},
				ProtectedNamespace: backup.Namespace,
				ResticSecretRef: corev1api.LocalObjectReference{
					Name: resticSecretName,
				},
			},
		}

		// check if VolumeBackupSnapshot CR exists for VSC
		VSBExists, err := util.DoesVolumeSnapshotBackupExistForVSC(&snapCont, p.Log)
		if err != nil {
			return nil, nil, errors.WithStack(err)
		}

		// Create VSB only if does not exist for the VSC
		if !VSBExists {
			vsbClient, err := util.GetVolumeSnapshotMoverClient()

			err = vsbClient.Create(context.Background(), &vsb)

			if err != nil {
				return nil, nil, errors.Wrapf(err, "error creating volumesnapshotbackup CR")
			}

			p.Log.Infof("Created volumesnapshotbackup %s", fmt.Sprintf("%s/%s", vsb.Namespace, vsb.Name))
		}

		// adding volumesnapshotbackup instance as an additional item, need to block the plugin execution till VSB CR is recon complete
		additionalItems = append(additionalItems, velero.ResourceIdentifier{
			GroupResource: schema.GroupResource{Group: "datamover.oadp.openshift.io", Resource: "volumesnapshotbackup"},
			Name:          vsb.Name,
			Namespace:     vsb.Namespace,
		})
	}

	// we should backup the snapshot deletion secrets that may be referenced in the volumesnapshotcontent's annotation
	if util.IsVolumeSnapshotContentHasDeleteSecret(&snapCont) {
		// TODO: add GroupResource for secret into kuberesource
		additionalItems = append(additionalItems, velero.ResourceIdentifier{
			GroupResource: schema.GroupResource{Group: "", Resource: "secrets"},
			Name:          snapCont.Annotations[util.PrefixedSnapshotterSecretNameKey],
			Namespace:     snapCont.Annotations[util.PrefixedSnapshotterSecretNamespaceKey],
		})
	}

	p.Log.Infof("Returning from VolumeSnapshotContentBackupItemAction with %d additionalItems to backup", len(additionalItems))
	return item, additionalItems, nil
}
