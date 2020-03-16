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

			if tc.vsc.Name != before.Name {
				t.Errorf("%s failed, unexpected change to Object.Name, Want: %s; Got %s", tc.name, before.Name, tc.vsc.Name)
			}

			if tc.vsc.Spec.DeletionPolicy != snapshotv1beta1api.VolumeSnapshotContentRetain {
				t.Errorf("%s failed, unexpected DeletionPolicy, Want: %s; Got: %s",
					tc.name, snapshotv1beta1api.VolumeSnapshotContentRetain, tc.vsc.Spec.DeletionPolicy)
			}

			if tc.vsc.Spec.Source.VolumeHandle != nil {
				t.Errorf("%s failed, unexpected value for Source.VolumeHandle, Want: nil; Got %s",
					tc.name, *tc.vsc.Spec.Source.VolumeHandle)
			}

			if tc.vsc.Spec.Source.SnapshotHandle == nil || *tc.vsc.Spec.Source.SnapshotHandle != tc.snapshotHandle {
				got := "nil"
				if tc.vsc.Spec.Source.SnapshotHandle != nil {
					got = *tc.vsc.Spec.Source.SnapshotHandle
				}
				t.Errorf("%s failed, unexpected value for Source.SnapshotHandle, Want: %s; Got %s",
					tc.name, tc.snapshotHandle, got)
			}

			if tc.vsc.Spec.VolumeSnapshotRef.Name != before.Spec.VolumeSnapshotRef.Name {
				t.Errorf("%s failed, unexpected value for VolumeSnapshotRef.Name, Want: %s; Got: %s",
					tc.name, before.Spec.VolumeSnapshotRef.Name, tc.vsc.Spec.VolumeSnapshotRef.Name)
			}
			if tc.vsc.Spec.VolumeSnapshotRef.Namespace != before.Spec.VolumeSnapshotRef.Namespace {
				t.Errorf("%s failed, unexpected value for VolumeSnapshotRef.Namespace, Want: %s; Got: %s",
					tc.name, before.Spec.VolumeSnapshotRef.Namespace, tc.vsc.Spec.VolumeSnapshotRef.Namespace)
			}
			if tc.vsc.Spec.VolumeSnapshotRef.Kind != "VolumeSnapshot" {
				t.Errorf("%s failed, unexpected value for VolumeSnapshotRef.Kind, Want: VolumeSnapshot; Got: %s",
					tc.name, tc.vsc.Spec.VolumeSnapshotRef.Kind)
			}

			if tc.vsc.Spec.Driver != before.Spec.Driver {
				t.Errorf("%s failed, unexpected value for Spec.Driver, Want: %s; Got %s",
					tc.name, before.Spec.Driver, tc.vsc.Spec.Driver)
			}

			if *tc.vsc.Spec.VolumeSnapshotClassName != *before.Spec.VolumeSnapshotClassName {
				t.Errorf("%s failed, unexpected value for Spec.VolumeSnapshotClassName, Want: %s; Got %s",
					tc.name, *before.Spec.VolumeSnapshotClassName, *tc.vsc.Spec.VolumeSnapshotClassName)
			}

			if tc.vsc.Status != nil {
				t.Errorf("%s failed, unexpected value for Status, Want: nil; Got: %v", tc.name, tc.vsc.Status)
			}
		})

	}
}
