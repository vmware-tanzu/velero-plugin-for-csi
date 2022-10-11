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

package util

const (
	VolumeSnapshotLabel              = "velero.io/volume-snapshot-name"
	VolumeSnapshotHandleAnnotation   = "velero.io/csi-volumesnapshot-handle"
	VolumeSnapshotRestoreSize        = "velero.io/vsi-volumesnapshot-restore-size"
	CSIDriverNameAnnotation          = "velero.io/csi-driver-name"
	CSIDeleteSnapshotSecretName      = "velero.io/csi-deletesnapshotsecret-name"
	CSIDeleteSnapshotSecretNamespace = "velero.io/csi-deletesnapshotsecret-namespace"
	CSIVSCDeletionPolicy             = "velero.io/csi-vsc-deletion-policy"
	VolumeSnapshotClassSelectorLabel = "velero.io/csi-volumesnapshot-class"

	// There is no release w/ these constants exported. Using the strings for now.
	// CSI Labels volumesnapshotclass
	// https://github.com/kubernetes-csi/external-snapshotter/blob/master/pkg/utils/util.go#L59-L60
	PrefixedSnapshotterListSecretNameKey      = "csi.storage.k8s.io/snapshotter-list-secret-name"
	PrefixedSnapshotterListSecretNamespaceKey = "csi.storage.k8s.io/snapshotter-list-secret-namespace"

	// CSI Labels volumesnapshotcontents
	PrefixedSnapshotterSecretNameKey      = "csi.storage.k8s.io/snapshotter-secret-name"
	PrefixedSnapshotterSecretNamespaceKey = "csi.storage.k8s.io/snapshotter-secret-namespace"

	// Velero checks this annotation to determine whether to skip resource excluding check.
	MustIncludeAdditionalItemAnnotation = "backup.velero.io/must-include-additional-items"
)
