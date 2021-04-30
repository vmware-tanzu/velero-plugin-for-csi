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

package main

import (
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/backup"
	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/delete"
	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/restore"
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

func main() {
	veleroplugin.NewServer().
		BindFlags(pflag.CommandLine).
		RegisterBackupItemAction("velero.io/csi-pvc-backupper", newPVCBackupItemAction).
		RegisterBackupItemAction("velero.io/csi-volumesnapshot-backupper", newVolumeSnapshotBackupItemAction).
		RegisterBackupItemAction("velero.io/csi-volumesnapshotclass-backupper", newVolumesnapshotClassBackupItemAction).
		RegisterBackupItemAction("velero.io/csi-volumesnapshotcontent-backupper", newVolumeSnapContentBackupItemAction).
		RegisterRestoreItemAction("velero.io/csi-pvc-restorer", newPVCRestoreItemAction).
		RegisterRestoreItemAction("velero.io/csi-volumesnapshot-restorer", newVolumeSnapshotRestoreItemAction).
		RegisterRestoreItemAction("velero.io/csi-volumesnapshotclass-restorer", newVolumeSnapshotClassRestoreItemAction).
		RegisterRestoreItemAction("velero.io/csi-volumesnapshotcontent-restorer", newVolumeSnapshotContentRestoreItemAction).
		RegisterDeleteItemAction("velero.io/csi-volumesnapshot-delete", newVolumeSnapshotDeleteItemAction).
		RegisterDeleteItemAction("velero.io/csi-volumesnapshotcontent-delete", newVolumeSnapshotContentDeleteItemAction).
		Serve()
}

func newPVCBackupItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &backup.PVCBackupItemAction{Log: logger}, nil
}

func newVolumeSnapshotBackupItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &backup.VolumeSnapshotBackupItemAction{Log: logger}, nil
}

func newVolumesnapshotClassBackupItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &backup.VolumeSnapshotClassBackupItemAction{Log: logger}, nil
}

func newVolumeSnapContentBackupItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &backup.VolumeSnapshotContentBackupItemAction{Log: logger}, nil
}

func newPVCRestoreItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &restore.PVCRestoreItemAction{Log: logger}, nil
}

func newVolumeSnapshotContentRestoreItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &restore.VolumeSnapshotContentRestoreItemAction{Log: logger}, nil
}

func newVolumeSnapshotRestoreItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &restore.VolumeSnapshotRestoreItemAction{Log: logger}, nil
}

func newVolumeSnapshotClassRestoreItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &restore.VolumeSnapshotClassRestoreItemAction{Log: logger}, nil
}

func newVolumeSnapshotDeleteItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &delete.VolumeSnapshotDeleteItemAction{Log: logger}, nil
}

func newVolumeSnapshotContentDeleteItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &delete.VolumeSnapshotContentDeleteItemAction{Log: logger}, nil
}
