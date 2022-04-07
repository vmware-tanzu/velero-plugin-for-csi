package delete

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// VolumeSnapshotDeleteItemAction is a backup item action plugin for Velero.
type VolumeSnapshotDeleteItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the VolumeSnapshotBackupItemAction should be invoked to backup volumesnapshots.
func (p *VolumeSnapshotDeleteItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Debug("VolumeSnapshotBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshots.snapshot.storage.k8s.io"},
	}, nil
}

func (p *VolumeSnapshotDeleteItemAction) Execute(input *velero.DeleteItemActionExecuteInput) error {
	p.Log.Info("Starting VolumeSnapshotDeleteItemAction for volumeSnapshot")

	var vs snapshotv1api.VolumeSnapshot

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &vs); err != nil {
		return errors.Wrapf(err, "failed to convert input.Item from unstructured")
	}

	// We don't want this DeleteItemAction plugin to delete Volumesnapshot taken outside of Velero.
	// So skip deleting Volumesnapshot objects that were not created in the process of creating
	// the Velero backup being deleted.
	if !util.HasBackupLabel(&vs.ObjectMeta, input.Backup.Name) {
		p.Log.Info("VolumeSnapshot %s/%s was not taken by backup %s, skipping deletion", vs.Namespace, vs.Name, input.Backup.Name)
		return nil
	}

	p.Log.Infof("Deleting Volumesnapshot %s/%s", vs.Namespace, vs.Name)
	_, snapClient, err := util.GetClients()
	if err != nil {
		return errors.WithStack(err)
	}
	if vs.Status != nil && vs.Status.BoundVolumeSnapshotContentName != nil {
		// we patch the DeletionPolicy of the volumesnapshotcontent to set it to Delete.
		// This ensures that the volume snapshot in the storage provider is also deleted.
		err := util.SetVolumeSnapshotContentDeletionPolicy(*vs.Status.BoundVolumeSnapshotContentName, snapClient.SnapshotV1())
		if err != nil && !apierrors.IsNotFound(err) {
			return errors.Wrapf(err, fmt.Sprintf("failed to patch DeletionPolicy of volume snapshot %s/%s", vs.Namespace, vs.Name))
		}

		if apierrors.IsNotFound(err) {
			return nil
		}
	}
	err = snapClient.SnapshotV1().VolumeSnapshots(vs.Namespace).Delete(context.TODO(), vs.Name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}
