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
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetPVCAnnotationsAndLabels(t *testing.T) {
	testCases := []struct {
		name         string
		pvc          corev1api.PersistentVolumeClaim
		snapshotName string
		backupName   string
	}{
		{
			name: "should set annotations and labels when they are nil",
			pvc: corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
					Labels:      nil,
				},
			},
		},
		{
			name: "should set annotation and labels on existing annotatation and labels",
			pvc: corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"foo": "foo-val",
						"bar": "bar-val",
						"baz": "baz-val",
					},
					Labels: map[string]string{
						"foo": "foo-val",
						"bar": "bar-val",
						"baz": "baz-val",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		before := tc.pvc.DeepCopy()
		setPVCAnnotationsAndLabels(&tc.pvc, tc.snapshotName, tc.backupName)

		assert.NotNil(t, tc.pvc.Annotations, "Want: non-nil annotation; Got: nil annotation")
		assert.NotNil(t, tc.pvc.Labels, "Want: non-nil labels; Got: nil labels")

		actualSnapshotNameAnnotation, exists := tc.pvc.Annotations[util.VolumeSnapshotLabel]
		assert.Truef(t, exists, "missing expected value for annotation %s, Want: %s", util.VolumeSnapshotLabel, tc.snapshotName)
		assert.Equalf(t, actualSnapshotNameAnnotation, tc.snapshotName, "unexpected value for annotation %s, Want: %s; Got: %s", util.VolumeSnapshotLabel, tc.snapshotName, actualSnapshotNameAnnotation)

		actualBackupName, exists := tc.pvc.Annotations[velerov1api.BackupNameLabel]
		assert.Truef(t, exists, "missing expected value for annotation %s, Want: %s", velerov1api.BackupNameLabel, tc.backupName)
		assert.Equalf(t, actualBackupName, tc.backupName, "unexpected value for annotation %s, Want: %s; Got: %s", velerov1api.BackupNameLabel, tc.backupName, actualBackupName)

		if before.Annotations != nil {
			for k, e := range before.Annotations {
				a, o := tc.pvc.Annotations[k]
				assert.Truef(t, o, "missing annotation, Want: %s as value for annotation %s", e, k)
				assert.Equalf(t, a, e, "unexpected value for annotation %s, Want:%s; Got:%s", k, e, a)
			}
		}

		actualSnapshotNameLabel, exists := tc.pvc.Labels[util.VolumeSnapshotLabel]
		assert.Truef(t, exists, "missing expected value for label %s, Want: %s", util.VolumeSnapshotLabel, tc.snapshotName)
		assert.Equalf(t, actualSnapshotNameLabel, tc.snapshotName, "unexpected value for label %s, Want: %s; Got: %s", util.VolumeSnapshotLabel, tc.snapshotName, actualSnapshotNameLabel)

		if before.Labels != nil {
			for k, e := range before.Labels {
				a, o := tc.pvc.Annotations[k]
				assert.Truef(t, o, "missing label, Want: %s as value for label %s", e, k)
				assert.Equalf(t, a, e, "unexpected value for label %s, Want:%s; Got:%s", k, e, a)
			}
		}
	}
}
