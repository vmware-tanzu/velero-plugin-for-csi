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
package main

import (
	"testing"

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

		if tc.pvc.Annotations == nil {
			t.Error("Want: non-nil annotation; Got: nil annotation")
		}
		if tc.pvc.Labels == nil {
			t.Error("Want: non-nil labels; Got: nil labels")
		}

		actualSnapshotNameAnnotation, exists := tc.pvc.Annotations[volumeSnapshotLabel]
		if !exists {
			t.Errorf("missing expected value for annotation %s, Want: %s", volumeSnapshotLabel, tc.snapshotName)
		}
		if actualSnapshotNameAnnotation != tc.snapshotName {
			t.Errorf("unexpected value for annotation %s, Want: %s; Got: %s", volumeSnapshotLabel, tc.snapshotName, actualSnapshotNameAnnotation)
		}

		actualBackupName, exists := tc.pvc.Annotations[velerov1api.BackupNameLabel]
		if !exists {
			t.Errorf("missing expected value for annotation %s, Want: %s", velerov1api.BackupNameLabel, tc.backupName)
		}
		if actualBackupName != tc.backupName {
			t.Errorf("unexpected value for annotation %s, Want: %s; Got: %s", velerov1api.BackupNameLabel, tc.backupName, actualBackupName)
		}

		if before.Annotations != nil {
			for k, e := range before.Annotations {
				a, o := tc.pvc.Annotations[k]
				if !o {
					t.Errorf("missing annotation, Want: %s as value for annotation %s", e, k)
				}
				if a != e {
					t.Errorf("unexpected value for annotation %s, Want:%s; Got:%s", k, e, a)
				}
			}
		}

		actualSnapshotNameLabel, exists := tc.pvc.Labels[volumeSnapshotLabel]
		if !exists {
			t.Errorf("missing expected value for label %s, Want: %s", volumeSnapshotLabel, tc.snapshotName)
		}
		if actualSnapshotNameLabel != tc.snapshotName {
			t.Errorf("unexpected value for label %s, Want: %s; Got: %s", volumeSnapshotLabel, tc.snapshotName, actualSnapshotNameLabel)
		}

		if before.Labels != nil {
			for k, e := range before.Labels {
				a, o := tc.pvc.Annotations[k]
				if !o {
					t.Errorf("missing label, Want: %s as value for annotation %s", e, k)
				}
				if a != e {
					t.Errorf("unexpected value for label %s, Want:%s; Got:%s", k, e, a)
				}
			}
		}
	}
}
