package delete

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	snapshotv1beta1api "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
)

// VolumeSnapshotContentDeleteItemAction is a restore item action plugin for Velero
type VolumeSnapshotContentDeleteItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating VolumeSnapshotContentRestoreItemAction action should be invoked while restoring
// volumesnapshotcontent.snapshot.storage.k8s.io resources
func (p *VolumeSnapshotContentDeleteItemAction) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshotcontent.snapshot.storage.k8s.io"},
	}, nil
}

func (p *VolumeSnapshotContentDeleteItemAction) Execute(input *velero.DeleteItemActionExecuteInput) error {
	p.Log.Info("Starting VolumeSnapshotContentDeleteItemAction")

	var snapCont snapshotv1beta1api.VolumeSnapshotContent
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &snapCont); err != nil {
		return errors.Wrapf(err, "failed to convert input.Item from unstructured")
	}

	// We don't want this DeleteItemAction plugin to delete VolumesnapshotContent taken outside of Velero.
	// So skip deleting VolumesnapshotContent objects that were not created in the process of creating
	// the Velero backup being deleted.
	if !util.HasBackupLabel(&snapCont.ObjectMeta, input.Backup.Name) {
		p.Log.Info("VolumeSnapshotContent %s was not taken by backup %s, skipping deletion", snapCont.Name, input.Backup.Name)
		return nil
	}

	p.Log.Infof("Deleting VolumeSnapshotContent %s", snapCont.Name)

	_, snapClient, err := util.GetClients()
	if err != nil {
		return errors.WithStack(err)
	}

	err = util.SetVolumeSnapshotContentDeletionPolicy(snapCont.Name, snapClient.SnapshotV1beta1())
	if err != nil {
		if apierrors.IsNotFound(err) {
			p.Log.Infof("VolumeSnapshotContent %s not found", snapCont.Name)
			return nil
		}
		return errors.Wrapf(err, fmt.Sprintf("failed to set DeletionPolicy on volumesnapshotcontent %s. Skipping deletion", snapCont.Name))
	}

	err = snapClient.SnapshotV1beta1().VolumeSnapshotContents().Delete(context.TODO(), snapCont.Name, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		p.Log.Infof("VolumeSnapshotContent %s not found", snapCont.Name)
		return err
	}

	return nil
}
