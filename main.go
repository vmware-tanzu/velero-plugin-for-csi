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
	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/restore"
	veleroplugin "github.com/vmware-tanzu/velero/pkg/plugin/framework"
)

func main() {
	veleroplugin.NewServer().
		BindFlags(pflag.CommandLine).
		RegisterBackupItemAction("velero.io/csi-snapshotter", newCSISnapshotter).
		RegisterBackupItemAction("velero.io/volumesnapshot-backupper", newVolumeSnapshotBackupItemAction).
		RegisterRestoreItemAction("velero.io/csi-restorer", newCSIRestorer).
		RegisterRestoreItemAction("velero.io/volumesnapshotcontents-restorer", newVSCRestorer).
		RegisterRestoreItemAction("velero.io/volumesnapshots-restorer", newVSRestorer).
		Serve()
}

func newCSISnapshotter(logger logrus.FieldLogger) (interface{}, error) {
	return &backup.CSISnapshotter{Log: logger}, nil
}

func newVolumeSnapshotBackupItemAction(logger logrus.FieldLogger) (interface{}, error) {
	return &backup.VolumeSnapshotBackupItemAction{Log: logger}, nil
}

func newCSIRestorer(logger logrus.FieldLogger) (interface{}, error) {
	return &restore.CSIRestorer{Log: logger}, nil
}

func newVSCRestorer(logger logrus.FieldLogger) (interface{}, error) {
	return &restore.VSCRestorer{Log: logger}, nil
}

func newVSRestorer(logger logrus.FieldLogger) (interface{}, error) {
	return &restore.VSRestorer{Log: logger}, nil
}
