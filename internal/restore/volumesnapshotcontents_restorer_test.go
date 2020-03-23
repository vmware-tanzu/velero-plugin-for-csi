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

package restore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	snapshotv1beta1api "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	testDriver     = "hostpath.csi.k8s.io"
	snapshotHandle = "snaphandle-foo-bar"
	vsName         = "test-vs"
	vsNamespace    = "test-ns"
	vsClass        = "snap-class"
	volumeHandle   = "volhandle-foo-bar"
)

func TestResetVSCSpecForRestore(t *testing.T) {
	testCases := []struct {
		name           string
		vsc            snapshotv1beta1api.VolumeSnapshotContent
		snapshotHandle string
	}{
		{
			name: "should reset VolumeSnapshotContent.Spec as expected",
			vsc: snapshotv1beta1api.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-vsc",
				},
				Spec: snapshotv1beta1api.VolumeSnapshotContentSpec{
					VolumeSnapshotRef: corev1api.ObjectReference{
						Name:            vsName,
						Namespace:       vsNamespace,
						UID:             "should-be deleted",
						ResourceVersion: "1234",
						Kind:            "VolumeSnapshot",
					},
					DeletionPolicy:          snapshotv1beta1api.VolumeSnapshotContentDelete,
					Driver:                  testDriver,
					VolumeSnapshotClassName: &vsClass,
					Source: snapshotv1beta1api.VolumeSnapshotContentSource{
						VolumeHandle: &volumeHandle,
					},
				},
				Status: nil,
			},
			snapshotHandle: snapshotHandle,
		},
		{
			name: "should reset VolumeSnapshotContent.Spec overwriting value for Source.SnapshotHandle",
			vsc: snapshotv1beta1api.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-vsc",
				},
				Spec: snapshotv1beta1api.VolumeSnapshotContentSpec{
					VolumeSnapshotRef: corev1api.ObjectReference{
						Name:            vsName,
						Namespace:       vsNamespace,
						UID:             "should-be deleted",
						ResourceVersion: "1234",
						Kind:            "VolumeSnapshot",
					},
					DeletionPolicy:          snapshotv1beta1api.VolumeSnapshotContentDelete,
					Driver:                  testDriver,
					VolumeSnapshotClassName: &vsClass,
					Source: snapshotv1beta1api.VolumeSnapshotContentSource{
						SnapshotHandle: &volumeHandle,
					},
				},
				Status: nil,
			},
			snapshotHandle: snapshotHandle,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.vsc.DeepCopy()
			resetVSCSpecForRestore(&tc.vsc, &tc.snapshotHandle)

			assert.Equalf(t, tc.vsc.Name, before.Name, "unexpected change to Object.Name, Want: %s; Got %s", tc.name, before.Name, tc.vsc.Name)
			assert.Equal(t, tc.vsc.Spec.DeletionPolicy, snapshotv1beta1api.VolumeSnapshotContentRetain)
			assert.Nil(t, tc.vsc.Spec.Source.VolumeHandle)
			assert.NotNil(t, tc.vsc.Spec.Source.SnapshotHandle)
			assert.Equal(t, tc.snapshotHandle, *tc.vsc.Spec.Source.SnapshotHandle)
			assert.Equal(t, tc.vsc.Spec.VolumeSnapshotRef.Name, before.Spec.VolumeSnapshotRef.Name, "unexpected value for VolumeSnapshotRef.Name, Want: %s; Got: %s",
				tc.name, before.Spec.VolumeSnapshotRef.Name, tc.vsc.Spec.VolumeSnapshotRef.Name)
			assert.Equal(t, tc.vsc.Spec.VolumeSnapshotRef.Namespace, before.Spec.VolumeSnapshotRef.Namespace, "unexpected value for VolumeSnapshotRef.Namespace, Want: %s; Got: %s",
				tc.name, before.Spec.VolumeSnapshotRef.Namespace, tc.vsc.Spec.VolumeSnapshotRef.Namespace)
			assert.Equal(t, tc.vsc.Spec.VolumeSnapshotRef.Kind, "VolumeSnapshot", "unexpected value for VolumeSnapshotRef.Kind, Want: VolumeSnapshot; Got: %s",
				tc.name, tc.vsc.Spec.VolumeSnapshotRef.Kind)
			assert.Equal(t, tc.vsc.Spec.Driver, before.Spec.Driver, "unexpected value for Spec.Driver, Want: %s; Got %s",
				tc.name, before.Spec.Driver, tc.vsc.Spec.Driver)
			assert.Equal(t, *tc.vsc.Spec.VolumeSnapshotClassName, *before.Spec.VolumeSnapshotClassName, "unexpected value for Spec.VolumeSnapshotClassName, Want: %s; Got %s",
				tc.name, *before.Spec.VolumeSnapshotClassName, *tc.vsc.Spec.VolumeSnapshotClassName)
			assert.Nil(t, tc.vsc.Status)
		})

	}
}
