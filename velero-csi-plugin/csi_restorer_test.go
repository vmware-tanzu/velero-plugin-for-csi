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

	"github.com/stretchr/testify/assert"

	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestResetPVCAnnotations(t *testing.T) {
	testCases := []struct {
		name                string
		pvc                 corev1api.PersistentVolumeClaim
		preserveAnnotations []string
		expectedAnnotations map[string]string
	}{
		{
			name: "should create empty annotation map",
			pvc: corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			preserveAnnotations: []string{"foo"},
			expectedAnnotations: map[string]string{},
		},
		{
			name: "should preserve all existing annotations",
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
			expectedAnnotations: map[string]string{
				"ann1": "ann1-val",
				"ann2": "ann2-val",
				"ann3": "ann3-val",
				"ann4": "ann4-val",
			},
		},
		{
			name: "should remove all existing annotations",
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
			expectedAnnotations: map[string]string{},
		},
		{
			name: "should preserve some existing annotations",
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
			expectedAnnotations: map[string]string{
				"ann1": "ann1-val",
				"ann2": "ann2-val",
				"ann3": "ann3-val",
				"ann4": "ann4-val",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resetPVCAnnotations(&tc.pvc, tc.preserveAnnotations)
			assert.Equal(t, tc.expectedAnnotations, tc.pvc.Annotations)
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

			assert.Equalf(t, tc.pvc.Name, before.Name, "unexpected change to Object.Name, Want: %s; Got %s", before.Name, tc.pvc.Name)
			assert.Equalf(t, tc.pvc.Namespace, before.Namespace, "unexpected change to Object.Namespace, Want: %s; Got %s", before.Namespace, tc.pvc.Namespace)
			assert.Equalf(t, tc.pvc.Spec.AccessModes, before.Spec.AccessModes, "unexpected Spec.AccessModes, Want: %v; Got: %v", before.Spec.AccessModes, tc.pvc.Spec.AccessModes)
			assert.Equalf(t, tc.pvc.Spec.Selector, before.Spec.Selector, "unexpected change to Spec.Selector, Want: %s; Got: %s", before.Spec.Selector.String(), tc.pvc.Spec.Selector.String())
			assert.Equalf(t, tc.pvc.Spec.Resources, before.Spec.Resources, "unexpected change to Spec.Resources, Want: %s; Got: %s", before.Spec.Resources.String(), tc.pvc.Spec.Resources.String())
			assert.Emptyf(t, tc.pvc.Spec.VolumeName, "expected change to Spec.VolumeName missing, Want: \"\"; Got: %s", tc.pvc.Spec.VolumeName)
			assert.Equalf(t, *tc.pvc.Spec.VolumeMode, *before.Spec.VolumeMode, "expected change to Spec.VolumeName missing, Want: \"\"; Got: %s", tc.pvc.Spec.VolumeName)
			assert.NotNil(t, tc.pvc.Spec.DataSource, "expected change to Spec.DataSource missing")
			assert.Equalf(t, tc.pvc.Spec.DataSource.Kind, "VolumeSnapshot", "expected change to Spec.DataSource.Kind missing, Want: VolumeSnapshot, Got: %s", tc.pvc.Spec.DataSource.Kind)
			assert.Equalf(t, tc.pvc.Spec.DataSource.Name, tc.vsName, "expected change to Spec.DataSource.Name missing, Want: %s, Got: %s", tc.vsName, tc.pvc.Spec.DataSource.Name)
		})
	}
}
