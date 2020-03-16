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

	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResetPVCAnnotations(t *testing.T) {
	testCases := []struct {
		name                      string
		pvc                       corev1api.PersistentVolumeClaim
		preserveAnnotations       []string
		expectedAnnotations       map[string]string
		expectNonEmptyAnnotations bool
	}{
		{
			name:                      "should create empty annotation map",
			expectNonEmptyAnnotations: true,
			pvc: corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			preserveAnnotations: []string{"foo"},
		},
		{
			name:                      "should preserve all existing annotations",
			expectNonEmptyAnnotations: false,
			pvc: corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"ann1": "ann1-val",
						"ann2": "ann2-val",
						"ann3": "ann3-val",
						"ann4": "ann4-val",
					},
				},
			},
			preserveAnnotations: []string{"ann1", "ann2", "ann3", "ann4"},
		},
		{
			name:                      "should remove all existing annotations",
			expectNonEmptyAnnotations: false,
			pvc: corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"ann1": "ann1-val",
						"ann2": "ann2-val",
						"ann3": "ann3-val",
						"ann4": "ann4-val",
					},
				},
			},
			preserveAnnotations: []string{},
		},
		{
			name:                      "should preserve some existing annotations",
			expectNonEmptyAnnotations: false,
			pvc: corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"ann1": "ann1-val",
						"ann2": "ann2-val",
						"ann3": "ann3-val",
						"ann4": "ann4-val",
						"ann5": "ann5-val",
						"ann6": "ann6-val",
						"ann7": "ann7-val",
						"ann8": "ann8-val",
					},
				},
			},
			preserveAnnotations: []string{"ann1", "ann2", "ann3", "ann4"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resetPVCAnnotations(&tc.pvc, tc.preserveAnnotations)
			if tc.pvc.Annotations == nil {
				t.Errorf("%s failed, unexpected PVC annotations returned, Want: non-nil; Got: nil", tc.name)
			}
			if tc.expectNonEmptyAnnotations && len(tc.pvc.Annotations) != 0 {
				t.Errorf("%s failed, unexpected count of PVC annotations returned, Want: 0; Got: %d", tc.name, len(tc.pvc.Annotations))
			}

			if tc.expectNonEmptyAnnotations {
				return
			}

			if len(tc.preserveAnnotations) != len(tc.pvc.Annotations) {
				t.Errorf("%s failed, unexpected count of pvc annotations returned, Want: %d, Got: %d",
					tc.name, len(tc.preserveAnnotations), len(tc.pvc.Annotations))
			}

			for k := range tc.pvc.Annotations {
				if !contains(tc.preserveAnnotations, k) {
					t.Errorf("%s failed, annotation %s not required but preserved", tc.name, k)
				}
			}
		})
	}
}

func TestResetPVCSpec(t *testing.T) {
	fileMode := corev1api.PersistentVolumeFilesystem
	blockMode := corev1api.PersistentVolumeBlock

	testCases := []struct {
		name   string
		pvc    corev1api.PersistentVolumeClaim
		vsName string
	}{
		{
			name: "should reset expected fields in pvc using file mode volumes",
			pvc: corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
				},
				Spec: corev1api.PersistentVolumeClaimSpec{
					AccessModes: []corev1api.PersistentVolumeAccessMode{corev1api.ReadOnlyMany, corev1api.ReadWriteMany, corev1api.ReadWriteOnce},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
							"baz": "qux",
						},
					},
					Resources: corev1api.ResourceRequirements{
						Requests: corev1api.ResourceList{
							corev1api.ResourceCPU: resource.Quantity{
								Format: resource.DecimalExponent,
							},
						},
					},
					VolumeName: "should-be-removed",
					VolumeMode: &fileMode,
				},
			},
			vsName: "test-vs",
		},
		{
			name: "should reset expected fields in pvc using block mode volumes",
			pvc: corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
				},
				Spec: corev1api.PersistentVolumeClaimSpec{
					AccessModes: []corev1api.PersistentVolumeAccessMode{corev1api.ReadOnlyMany, corev1api.ReadWriteMany, corev1api.ReadWriteOnce},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
							"baz": "qux",
						},
					},
					Resources: corev1api.ResourceRequirements{
						Requests: corev1api.ResourceList{
							corev1api.ResourceCPU: resource.Quantity{
								Format: resource.DecimalExponent,
							},
						},
					},
					VolumeName: "should-be-removed",
					VolumeMode: &blockMode,
				},
			},
			vsName: "test-vs",
		},
		{
			name: "should overwrite existing DataSource per reset parameters",
			pvc: corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pvc",
					Namespace: "test-ns",
				},
				Spec: corev1api.PersistentVolumeClaimSpec{
					AccessModes: []corev1api.PersistentVolumeAccessMode{corev1api.ReadOnlyMany, corev1api.ReadWriteMany, corev1api.ReadWriteOnce},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"foo": "bar",
							"baz": "qux",
						},
					},
					Resources: corev1api.ResourceRequirements{
						Requests: corev1api.ResourceList{
							corev1api.ResourceCPU: resource.Quantity{
								Format: resource.DecimalExponent,
							},
						},
					},
					VolumeName: "should-be-removed",
					VolumeMode: &fileMode,
					DataSource: &corev1api.TypedLocalObjectReference{
						Kind: "something-that-does-not-exist",
						Name: "not-found",
					},
				},
			},
			vsName: "test-vs",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.pvc.DeepCopy()
			resetPVCSpec(&tc.pvc, tc.vsName)

			if tc.pvc.Name != before.Name {
				t.Errorf("unexpected change to Object.Name, Want: %s; Got %s", before.Name, tc.pvc.Name)
			}
			if tc.pvc.Namespace != before.Namespace {
				t.Errorf("unexpected change to Object.Namespace, Want: %s; Got %s", before.Namespace, tc.pvc.Namespace)
			}

			if len(tc.pvc.Spec.AccessModes) != len(before.Spec.AccessModes) {
				t.Errorf("unexpected count of Spec.AccessModes, Want: %d; Got: %d", len(before.Spec.AccessModes), len(tc.pvc.Spec.AccessModes))
			}

			if tc.pvc.Spec.Selector.String() != before.Spec.Selector.String() {
				t.Errorf("unexpected change to Spec.Selector, Want: %s; Got: %s", before.Spec.Selector.String(), tc.pvc.Spec.Selector.String())
			}

			if tc.pvc.Spec.Resources.String() != before.Spec.Resources.String() {
				t.Errorf("unexpected change to Spec.Resources, Want: %s; Got: %s", before.Spec.Resources.String(), tc.pvc.Spec.Resources.String())
			}

			if tc.pvc.Spec.VolumeName != "" {
				t.Errorf("expected change to Spec.VolumeName missing, Want: \"\"; Got: %s", tc.pvc.Spec.VolumeName)
			}

			if *tc.pvc.Spec.VolumeMode != *before.Spec.VolumeMode {
				t.Errorf("unexpected change to Spec.VolumeMode, Want: %s; Got: %s", *before.Spec.VolumeMode, *tc.pvc.Spec.VolumeMode)
			}

			if tc.pvc.Spec.DataSource == nil {
				t.Error("expected change to Spec.DataSource missing")
			}

			if tc.pvc.Spec.DataSource.Kind != "VolumeSnapshot" {
				t.Errorf("expected change to Spec.DataSource.Kind missing, Want: VolumeSnapshot, Got: %s", tc.pvc.Spec.DataSource.Kind)
			}
			if tc.pvc.Spec.DataSource.Name != tc.vsName {
				t.Errorf("expected change to Spec.DataSource.Name missing, Want: %s, Got: %s", tc.vsName, tc.pvc.Spec.DataSource.Name)
			}
		})
	}
}
