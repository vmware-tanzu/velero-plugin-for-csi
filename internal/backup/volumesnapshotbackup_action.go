package backup

import (
	"fmt"

	datamoverv1alpha1 "github.com/konveyor/volume-snapshot-mover/api/v1alpha1"
	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// VolumeSnapshotBackupBackupItemAction is a backup item action plugin to backup
// VolumeSnapshotBackup objects using Velero
type VolumeSnapshotBackupBackupItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the VolumeSnapshotBackupItemAction should be invoked to backup VolumeSnapshotBackups.
func (p *VolumeSnapshotBackupBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("VolumeSnapshotBackupBackupItemAction AppliesTo")

	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshotbackups.datamover.oadp.openshift.io"},
	}, nil
}

// Execute backs up a VolumeSnapshotBackup object with a completely filled status
func (p *VolumeSnapshotBackupBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.Log.Infof("Executing VolumeSnapshotBackupBackupItemAction")
	vsb := datamoverv1alpha1.VolumeSnapshotBackup{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &vsb); err != nil {
		return nil, nil, errors.WithStack(err)
	}
	p.Log.Infof("Converted Item to VSB: %v", vsb)
	vsbNew, err := util.GetVolumeSnapshotbackupWithStatusData(vsb.Namespace, vsb.Name, p.Log)

	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	vsb.Status = *vsbNew.Status.DeepCopy()

	vals := map[string]string{
		util.VolumeSnapshotMoverResticRepository:      vsb.Status.ResticRepository,
		util.VolumeSnapshotMoverSourcePVCName:         vsb.Status.SourcePVCData.Name,
		util.VolumeSnapshotMoverSourcePVCSize:         vsb.Status.SourcePVCData.Size,
		util.VolumeSnapshotMoverSourcePVCStorageClass: vsb.Status.SourcePVCData.StorageClassName,
		util.VolumeSnapshotMoverVolumeSnapshotClass:   vsb.Status.VolumeSnapshotClassName,
	}

	//Add all the relevant status info as annotations because velero strips status subresource for CRDs
	util.AddAnnotations(&vsb.ObjectMeta, vals)

	vsbMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&vsb)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// TODO ?: check if fake VSClass already exists

	// Add dummy VSClass as additional item
	dummyVSC := snapshotv1api.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-vsclass",
			Labels: map[string]string{
				util.WaitVolumeSnapshotBackup: "true",
			},
		},
		Driver:         "ebs.csi.aws.com",
		DeletionPolicy: "Retain",
	}

	// TODO ?: need create call for fake vsclass

	additionalItems := []velero.ResourceIdentifier{}

	// adding dummy VSClass instance as an additional item for blocking VSR
	additionalItems = append(additionalItems, velero.ResourceIdentifier{
		GroupResource: schema.GroupResource{Group: "snapshot.storage.k8s.io", Resource: "volumesnapshotclasses"},
		Name:          dummyVSC.Name,
	})

	p.Log.Infof("Add VSClass %s as an additional item", fmt.Sprintf("%s", dummyVSC.Name))

	return &unstructured.Unstructured{Object: vsbMap}, additionalItems, nil
}
