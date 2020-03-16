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
	snapshotFake "github.com/kubernetes-csi/external-snapshotter/v2/pkg/client/clientset/versioned/fake"
	"github.com/sirupsen/logrus"
	corev1api "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

var (
	csiStorageClass = "csi-hostpath-sc"
	volumeMode      = "Filesystem"
)

func TestGetPVForPVC(t *testing.T) {
	boundPVC := &corev1api.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-csi-pvc",
			Namespace: "default",
		},
		Spec: corev1api.PersistentVolumeClaimSpec{
			AccessModes: []corev1api.PersistentVolumeAccessMode{corev1api.ReadWriteOnce},
			Resources: corev1api.ResourceRequirements{
				Requests: corev1api.ResourceList{},
			},
			StorageClassName: &csiStorageClass,
			VolumeName:       "test-csi-7d28e566-ade7-4ed6-9e15-2e44d2fbcc08",
		},
		Status: corev1api.PersistentVolumeClaimStatus{
			Phase:       corev1api.ClaimBound,
			AccessModes: []corev1api.PersistentVolumeAccessMode{corev1api.ReadWriteOnce},
			Capacity:    corev1api.ResourceList{},
		},
	}
	matchingPV := &corev1api.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-csi-7d28e566-ade7-4ed6-9e15-2e44d2fbcc08",
		},
		Spec: corev1api.PersistentVolumeSpec{
			AccessModes: []corev1api.PersistentVolumeAccessMode{corev1api.ReadWriteOnce},
			Capacity:    corev1api.ResourceList{},
			ClaimRef: &v1.ObjectReference{
				Kind:            "PersistentVolumeClaim",
				Name:            "test-csi-pvc",
				Namespace:       "default",
				ResourceVersion: "1027",
				UID:             "7d28e566-ade7-4ed6-9e15-2e44d2fbcc08",
			},
			PersistentVolumeSource: corev1api.PersistentVolumeSource{
				CSI: &corev1api.CSIPersistentVolumeSource{
					Driver: "hostpath.csi.k8s.io",
					FSType: "ext4",
					VolumeAttributes: map[string]string{
						"storage.kubernetes.io/csiProvisionerIdentity": "1582049697841-8081-hostpath.csi.k8s.io",
					},
					VolumeHandle: "e61f2b48-527a-11ea-b54f-cab6317018f1",
				},
			},
			PersistentVolumeReclaimPolicy: corev1api.PersistentVolumeReclaimDelete,
			StorageClassName:              csiStorageClass,
		},
		Status: corev1api.PersistentVolumeStatus{
			Phase: corev1api.VolumeBound,
		},
	}

	pvcWithNoVolumeName := &corev1api.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-vol-pvc",
			Namespace: "default",
		},
		Spec: corev1api.PersistentVolumeClaimSpec{
			AccessModes: []corev1api.PersistentVolumeAccessMode{corev1api.ReadWriteOnce},
			Resources: corev1api.ResourceRequirements{
				Requests: corev1api.ResourceList{},
			},
			StorageClassName: &csiStorageClass,
		},
		Status: corev1api.PersistentVolumeClaimStatus{},
	}

	unboundPVC := &corev1api.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unbound-pvc",
			Namespace: "default",
		},
		Spec: corev1api.PersistentVolumeClaimSpec{
			AccessModes: []corev1api.PersistentVolumeAccessMode{corev1api.ReadWriteOnce},
			Resources: corev1api.ResourceRequirements{
				Requests: corev1api.ResourceList{},
			},
			StorageClassName: &csiStorageClass,
			VolumeName:       "test-csi-7d28e566-ade7-4ed6-9e15-2e44d2fbcc08",
		},
		Status: corev1api.PersistentVolumeClaimStatus{
			Phase:       corev1api.ClaimPending,
			AccessModes: []corev1api.PersistentVolumeAccessMode{corev1api.ReadWriteOnce},
			Capacity:    corev1api.ResourceList{},
		},
	}

	testCases := []struct {
		name        string
		inPVC       *corev1api.PersistentVolumeClaim
		expectError bool
		expectedPV  *corev1api.PersistentVolume
	}{
		{
			name:        "should find PV matching the PVC",
			inPVC:       boundPVC,
			expectError: false,
			expectedPV:  matchingPV,
		},
		{
			name:        "should fail to find PV for PVC with no volumeName",
			inPVC:       pvcWithNoVolumeName,
			expectError: true,
			expectedPV:  nil,
		},
		{
			name:        "should fail to find PV for PVC not in bound phase",
			inPVC:       unboundPVC,
			expectError: true,
			expectedPV:  nil,
		},
	}

	objs := []runtime.Object{boundPVC, matchingPV, pvcWithNoVolumeName, unboundPVC}
	fakeClient := fake.NewSimpleClientset(objs...)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualPV, actualError := getPVForPVC(tc.inPVC, fakeClient.CoreV1())

			if tc.expectError && actualError == nil {
				t.Fatalf("getPVForPVC failed for [%s], Want error; Got nil error", tc.name)
			}
			if tc.expectError && actualPV != nil {
				t.Fatalf("getPVForPVC failed for [%s], Want PV: nil; Got PV: %q", tc.name, actualPV)
			}

			if !tc.expectError && actualError != nil {
				t.Fatalf("getPVForPVC failed for [%s], Want: nil error; Got: %v", tc.name, actualError)
			}
			if !tc.expectError && actualPV.Name != tc.expectedPV.Name {
				t.Fatalf("getPVForPVC failed for [%s], Want PV with name %q; Got PV with name %q", tc.name, tc.expectedPV.Name, actualPV.Name)
			}
		})
	}
}

func TestGetPodsUsingPVC(t *testing.T) {
	objs := []runtime.Object{
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "default",
			},
			Spec: corev1api.PodSpec{
				Volumes: []corev1api.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1api.VolumeSource{
							PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: "default",
			},
			Spec: corev1api.PodSpec{
				Volumes: []corev1api.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1api.VolumeSource{
							PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod3",
				Namespace: "default",
			},
			Spec: corev1api.PodSpec{
				Volumes: []corev1api.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1api.VolumeSource{
							EmptyDir: &corev1api.EmptyDirVolumeSource{},
						},
					},
				},
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "awesome-ns",
			},
			Spec: corev1api.PodSpec{
				Volumes: []corev1api.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1api.VolumeSource{
							PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
	}
	fakeClient := fake.NewSimpleClientset(objs...)

	testCases := []struct {
		name             string
		pvcNamespace     string
		pvcName          string
		expectedPodCount int
	}{
		{
			name:             "should find exactly 2 pods using the PVC",
			pvcNamespace:     "default",
			pvcName:          "csi-pvc1",
			expectedPodCount: 2,
		},
		{
			name:             "should find exactly 1 pod using the PVC",
			pvcNamespace:     "awesome-ns",
			pvcName:          "csi-pvc1",
			expectedPodCount: 1,
		},
		{
			name:             "should find 0 pods using the PVC",
			pvcNamespace:     "default",
			pvcName:          "unused-pvc",
			expectedPodCount: 0,
		},
		{
			name:             "should find 0 pods in non-existent namespace",
			pvcNamespace:     "does-not-exist",
			pvcName:          "csi-pvc1",
			expectedPodCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualPods, err := getPodsUsingPVC(tc.pvcNamespace, tc.pvcName, fakeClient.CoreV1())
			if err != nil {
				t.Fatalf("Want error=nil; Got error=%v", err)
			}

			if len(actualPods) != tc.expectedPodCount {
				t.Fatalf("unexpected number of pods in result; Want: %d; Got: %d", tc.expectedPodCount, len(actualPods))
			}
		})
	}
}

func TestGetPodVolumeNameForPVC(t *testing.T) {
	testCases := []struct {
		name               string
		pod                corev1api.Pod
		pvcName            string
		expectError        bool
		expectedVolumeName string
	}{
		{
			name: "should get volume name for pod with multuple PVCs",
			pod: corev1api.Pod{
				Spec: corev1api.PodSpec{
					Volumes: []corev1api.Volume{
						{
							Name: "csi-vol1",
							VolumeSource: corev1api.VolumeSource{
								PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
									ClaimName: "csi-pvc1",
								},
							},
						},
						{
							Name: "csi-vol2",
							VolumeSource: corev1api.VolumeSource{
								PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
									ClaimName: "csi-pvc2",
								},
							},
						},
						{
							Name: "csi-vol3",
							VolumeSource: corev1api.VolumeSource{
								PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
									ClaimName: "csi-pvc3",
								},
							},
						},
					},
				},
			},
			pvcName:            "csi-pvc2",
			expectedVolumeName: "csi-vol2",
			expectError:        false,
		},
		{
			name: "should get volume name from pod using exactly one PVC",
			pod: corev1api.Pod{
				Spec: corev1api.PodSpec{
					Volumes: []corev1api.Volume{
						{
							Name: "csi-vol1",
							VolumeSource: corev1api.VolumeSource{
								PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
									ClaimName: "csi-pvc1",
								},
							},
						},
					},
				},
			},
			pvcName:            "csi-pvc1",
			expectedVolumeName: "csi-vol1",
			expectError:        false,
		},
		{
			name: "should return error for pod with no PVCs",
			pod: corev1api.Pod{
				Spec: corev1api.PodSpec{},
			},
			pvcName:     "csi-pvc2",
			expectError: true,
		},
		{
			name: "should return error for pod with no matching PVC",
			pod: corev1api.Pod{
				Spec: corev1api.PodSpec{
					Volumes: []corev1api.Volume{
						{
							Name: "csi-vol1",
							VolumeSource: corev1api.VolumeSource{
								PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
									ClaimName: "csi-pvc1",
								},
							},
						},
					},
				},
			},
			pvcName:     "mismatch-pvc",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualVolumeName, err := getPodVolumeNameForPVC(tc.pod, tc.pvcName)
			if tc.expectError && err == nil {
				t.Fatalf("getPodVolumeNameForPVC failed for [%s], Want error; Got nil error", tc.name)
			}
			if !tc.expectError && tc.expectedVolumeName != actualVolumeName {
				t.Fatalf("getPodVolumeNameForPVC failed for [%s], unexpected podVolumename returned. Want %s; Got %s", tc.name, tc.expectedVolumeName, actualVolumeName)
			}
		})
	}
}

func TestGetPodVolumesUsingRestic(t *testing.T) {
	testCases := []struct {
		name                  string
		pod                   corev1api.Pod
		expectedResticVolumes []string
	}{
		{
			name: "Pod using restic on 3 volumes",
			pod: corev1api.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"backup.velero.io/backup-volumes": "vol1,vol2,vol3",
					},
				},
			},
			expectedResticVolumes: []string{"vol1", "vol2", "vol3"},
		},
		{
			name: "Pod using restic on 1 volume",
			pod: corev1api.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"backup.velero.io/backup-volumes": "vol1",
					},
				},
			},
			expectedResticVolumes: []string{"vol1"},
		},
		{
			name: "Pod using restic on 0 volumes",
			pod: corev1api.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"backup.velero.io/backup-volumes": "",
					},
				},
			},
			expectedResticVolumes: []string{},
		},
		{
			name: "Pod with no annotation",
			pod: corev1api.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
				},
			},
			expectedResticVolumes: []string{},
		},
		{
			name: "Pod with annotation but no restic annotations",
			pod: corev1api.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"annotation1": "val1",
						"annotation2": "val2",
						"annotation3": "val3",
					},
				},
			},
			expectedResticVolumes: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualResticVolumes := getPodVolumesUsingRestic(tc.pod)
			if len(tc.expectedResticVolumes) != len(actualResticVolumes) {
				t.Fatalf("getPodVolumesUsingRestic failed for %q, unexpected number of volumes using resitc, Want: %d; Got: %d", tc.name, len(tc.expectedResticVolumes),
					len(actualResticVolumes))
			}
		})
	}
}

func TestContains(t *testing.T) {
	testCases := []struct {
		name           string
		inSlice        []string
		inKey          string
		expectedResult bool
	}{
		{
			name:           "should find the key",
			inSlice:        []string{"key1", "key2", "key3", "key4", "key5"},
			inKey:          "key3",
			expectedResult: true,
		},
		{
			name:           "should not find the key in non-empty slice",
			inSlice:        []string{"key1", "key2", "key3", "key4", "key5"},
			inKey:          "key300",
			expectedResult: false,
		},
		{
			name:           "should not find key in empty slice",
			inSlice:        []string{},
			inKey:          "key300",
			expectedResult: false,
		},
		{
			name:           "should not find key in nil slice",
			inSlice:        nil,
			inKey:          "key300",
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualResult := contains(tc.inSlice, tc.inKey)
			if actualResult != tc.expectedResult {
				t.Fatalf("contains failed for [%s], Want: %t; Got: %t", tc.name, tc.expectedResult, actualResult)
			}
		})
	}
}

func TestIsPVCBackedUpByRestic(t *testing.T) {
	objs := []runtime.Object{
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "default",
			},
			Spec: corev1api.PodSpec{
				Volumes: []corev1api.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1api.VolumeSource{
							PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: "default",
				Annotations: map[string]string{
					"backup.velero.io/backup-volumes": "csi-vol1",
				},
			},
			Spec: corev1api.PodSpec{
				Volumes: []corev1api.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1api.VolumeSource{
							PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod3",
				Namespace: "default",
			},
			Spec: corev1api.PodSpec{
				Volumes: []corev1api.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1api.VolumeSource{
							EmptyDir: &corev1api.EmptyDirVolumeSource{},
						},
					},
				},
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "awesome-pod-1",
				Namespace: "awesome-ns",
			},
			Spec: corev1api.PodSpec{
				Volumes: []corev1api.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1api.VolumeSource{
							PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
								ClaimName: "awesome-csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "awesome-pod-2",
				Namespace: "awesome-ns",
			},
			Spec: corev1api.PodSpec{
				Volumes: []corev1api.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1api.VolumeSource{
							PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
								ClaimName: "awesome-csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "restic-ns",
				Annotations: map[string]string{
					"backup.velero.io/backup-volumes": "csi-vol1",
				},
			},
			Spec: corev1api.PodSpec{
				Volumes: []corev1api.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1api.VolumeSource{
							PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1api.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: "restic-ns",
				Annotations: map[string]string{
					"backup.velero.io/backup-volumes": "csi-vol1",
				},
			},
			Spec: corev1api.PodSpec{
				Volumes: []corev1api.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1api.VolumeSource{
							PersistentVolumeClaim: &corev1api.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
	}
	fakeClient := fake.NewSimpleClientset(objs...)

	testCases := []struct {
		name                 string
		inPVCNamespace       string
		inPVCName            string
		expectedIsResticUsed bool
	}{
		{
			name:                 "2 pods using PVC, 1 pod using restic",
			inPVCNamespace:       "default",
			inPVCName:            "csi-pvc1",
			expectedIsResticUsed: true,
		},
		{
			name:                 "2 pods using PVC, 2 pods using restic",
			inPVCNamespace:       "restic-ns",
			inPVCName:            "csi-pvc1",
			expectedIsResticUsed: true,
		},
		{
			name:                 "2 pods using PVC, 0 pods using restic",
			inPVCNamespace:       "awesome-ns",
			inPVCName:            "csi-pvc1",
			expectedIsResticUsed: false,
		},
		{
			name:                 "0 pods using PVC",
			inPVCNamespace:       "default",
			inPVCName:            "does-not-exist",
			expectedIsResticUsed: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualIsResticUsed, _ := isPVCBackedUpByRestic(tc.inPVCNamespace, tc.inPVCName, fakeClient.CoreV1())
			if actualIsResticUsed != tc.expectedIsResticUsed {
				t.Fatalf("isPVCBackedUpByRestic failed for [%s], Want: %t; Got: %t", tc.name, tc.expectedIsResticUsed, actualIsResticUsed)
			}
		})
	}
}

func TestGetVolumeSnapshotCalssForStorageClass(t *testing.T) {
	hostpathClass := &snapshotv1beta1api.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hostpath",
		},
		Driver: "hostpath.csi.k8s.io",
	}

	fooClass := &snapshotv1beta1api.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
		Driver: "foo.csi.k8s.io",
	}

	barClass := &snapshotv1beta1api.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
		},
		Driver: "bar.csi.k8s.io",
	}

	bazClass := &snapshotv1beta1api.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "baz",
		},
		Driver: "baz.csi.k8s.io",
	}

	objs := []runtime.Object{hostpathClass, fooClass, barClass, bazClass}
	fakeClient := snapshotFake.NewSimpleClientset(objs...)

	testCases := []struct {
		name        string
		driverName  string
		expectedVSC *snapshotv1beta1api.VolumeSnapshotClass
		expectError bool
	}{
		{
			name:        "should find hostpath volumesnapshotclass",
			driverName:  "hostpath.csi.k8s.io",
			expectedVSC: hostpathClass,
			expectError: false,
		},
		{
			name:        "should find foo volumesnapshotclass",
			driverName:  "foo.csi.k8s.io",
			expectedVSC: fooClass,
			expectError: false,
		},
		{
			name:        "should find var volumesnapshotclass",
			driverName:  "bar.csi.k8s.io",
			expectedVSC: barClass,
			expectError: false,
		},
		{
			name:        "should find foo volumesnapshotclass",
			driverName:  "baz.csi.k8s.io",
			expectedVSC: bazClass,
			expectError: false,
		},
		{
			name:        "should not find does-not-exist volumesnapshotclass",
			driverName:  "not-found.csi.k8s.io",
			expectedVSC: nil,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualVSC, actualError := getVolumeSnapshotClassForStorageClass(tc.driverName, fakeClient.SnapshotV1beta1())

			if tc.expectError && actualError == nil {
				t.Fatalf("getVolumeSnapshotClassForStorageClass failed for [%s]. Want error; Got no error", tc.name)
			}
			if tc.expectError && actualVSC != nil {
				t.Fatalf("getVolumeSnapshotClassForStorageClass failed for [%s], Want: nil result; Got non-nil result", tc.name)
			}
			if tc.expectError {
				return
			}

			if tc.expectedVSC.Name != actualVSC.Name {
				t.Fatalf("getVolumeSnapshotClassForStorageClass failed for [%s], unexpected volumesnapshotclass name returned. Want: %s; Got:%s", tc.name, tc.expectedVSC.Name, actualVSC.Name)
			}

			if tc.expectedVSC.Driver != actualVSC.Driver {
				t.Fatalf("getVolumeSnapshotClassForStorageClass failed for [%s], unexpected driver name returned. Want: %s; Got:%s", tc.name, tc.expectedVSC.Driver, actualVSC.Driver)
			}
		})
	}
}

func TestGetVolumeSnapshotContentForVolumeSnapshot(t *testing.T) {
	vscName := "snapcontent-7d1bdbd1-d10d-439c-8d8e-e1c2565ddc53"
	vscObj := &snapshotv1beta1api.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: vscName,
		},
		Spec: snapshotv1beta1api.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: corev1api.ObjectReference{
				Name:       "vol-snap-1",
				APIVersion: snapshotv1beta1api.SchemeGroupVersion.String(),
			},
		},
	}

	validVS := &snapshotv1beta1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vs",
			Namespace: "default",
		},
		Status: &snapshotv1beta1api.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: &vscName,
		},
	}

	notFound := "does-not-exist"
	vsWithVSCNotFound := &snapshotv1beta1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      notFound,
			Namespace: "default",
		},
		Status: &snapshotv1beta1api.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: &notFound,
		},
	}

	objs := []runtime.Object{vscObj, validVS, vsWithVSCNotFound}
	fakeClient := snapshotFake.NewSimpleClientset(objs...)
	testCases := []struct {
		name        string
		volSnap     *snapshotv1beta1api.VolumeSnapshot
		exepctedVSC *snapshotv1beta1api.VolumeSnapshotContent
		expectError bool
	}{
		{
			name:        "should find volumesnapshotcontent for volumesnapshot",
			volSnap:     validVS,
			exepctedVSC: vscObj,
			expectError: false,
		},
		{
			name:        "should not find volumesnapshotcontent for volumesnapshot with non-existing snapshotcontent name in status.BoundVolumeSnapshotContentName",
			volSnap:     vsWithVSCNotFound,
			exepctedVSC: nil,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualVSC, actualError := getVolumeSnapshotContentForVolumeSnapshot(tc.volSnap, fakeClient.SnapshotV1beta1(), logrus.New().WithField("fake", "test"))
			if tc.expectError && actualError == nil {
				t.Fatalf("getVolumeSnapshotContentForVolumeSnapshot failed for [%s], Want non-nil error; Got nil error", tc.name)
			}

			if tc.exepctedVSC == nil && actualVSC != nil {
				t.Fatalf("getVolumeSnapshotContentForVolumeSnapshot failed for [%s], Want nil result; Got non-nil result", tc.name)
			}
			if tc.exepctedVSC == nil {
				return
			}

			if actualVSC == nil && tc.exepctedVSC != nil {
				t.Fatalf("getVolumeSnapshotContentForVolumeSnapshot failed for [%s], Want non-nil result; Got nil result", tc.name)
			}

			if actualVSC.Name != tc.exepctedVSC.Name {
				t.Fatalf("getVolumeSnapshotContentForVolumeSnapshot failed for [%s], unexpected volumesnapshotcontent name; Want  %s; Got %s", tc.name, tc.exepctedVSC.Name, actualVSC.Name)
			}
		})
	}
}
