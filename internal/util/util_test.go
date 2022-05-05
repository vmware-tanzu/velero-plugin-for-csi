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

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	snapshotFake "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned/fake"
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
			actualPV, actualError := GetPVForPVC(tc.inPVC, fakeClient.CoreV1())

			if tc.expectError {
				assert.NotNil(t, actualError, "Want error; Got nil error")
				assert.Nilf(t, actualPV, "Want PV: nil; Got PV: %q", actualPV)
				return
			}

			assert.Nilf(t, actualError, "Want: nil error; Got: %v", actualError)
			assert.Equalf(t, actualPV.Name, tc.expectedPV.Name, "Want PV with name %q; Got PV with name %q", tc.expectedPV.Name, actualPV.Name)
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
			actualPods, err := GetPodsUsingPVC(tc.pvcNamespace, tc.pvcName, fakeClient.CoreV1())
			assert.Nilf(t, err, "Want error=nil; Got error=%v", err)
			assert.Equalf(t, len(actualPods), tc.expectedPodCount, "unexpected number of pods in result; Want: %d; Got: %d", tc.expectedPodCount, len(actualPods))
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
			actualVolumeName, err := GetPodVolumeNameForPVC(tc.pod, tc.pvcName)
			if tc.expectError && err == nil {
				assert.NotNil(t, err, "Want error; Got nil error")
				return
			}
			assert.Equalf(t, tc.expectedVolumeName, actualVolumeName, "unexpected podVolumename returned. Want %s; Got %s", tc.expectedVolumeName, actualVolumeName)
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
			actualResult := Contains(tc.inSlice, tc.inKey)
			assert.Equal(t, tc.expectedResult, actualResult)
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
		name                        string
		inPVCNamespace              string
		inPVCName                   string
		expectedIsResticUsed        bool
		defaultVolumeBackupToRestic bool
	}{
		{
			name:                        "2 pods using PVC, 1 pod using restic",
			inPVCNamespace:              "default",
			inPVCName:                   "csi-pvc1",
			expectedIsResticUsed:        true,
			defaultVolumeBackupToRestic: false,
		},
		{
			name:                        "2 pods using PVC, 2 pods using restic",
			inPVCNamespace:              "restic-ns",
			inPVCName:                   "csi-pvc1",
			expectedIsResticUsed:        true,
			defaultVolumeBackupToRestic: false,
		},
		{
			name:                        "2 pods using PVC, 0 pods using restic",
			inPVCNamespace:              "awesome-ns",
			inPVCName:                   "awesome-csi-pvc1",
			expectedIsResticUsed:        false,
			defaultVolumeBackupToRestic: false,
		},
		{
			name:                        "0 pods using PVC",
			inPVCNamespace:              "default",
			inPVCName:                   "does-not-exist",
			expectedIsResticUsed:        false,
			defaultVolumeBackupToRestic: false,
		},
		{
			name:                        "2 pods using PVC, using restic using restic by default",
			inPVCNamespace:              "awesome-ns",
			inPVCName:                   "awesome-csi-pvc1",
			expectedIsResticUsed:        true,
			defaultVolumeBackupToRestic: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualIsResticUsed, _ := IsPVCBackedUpByRestic(tc.inPVCNamespace, tc.inPVCName, fakeClient.CoreV1(), tc.defaultVolumeBackupToRestic)
			assert.Equal(t, tc.expectedIsResticUsed, actualIsResticUsed)
		})
	}
}

func TestGetVolumeSnapshotCalssForStorageClass(t *testing.T) {
	hostpathClass := &snapshotv1api.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hostpath",
			Labels: map[string]string{
				VolumeSnapshotClassSelectorLabel: "foo",
			},
		},
		Driver: "hostpath.csi.k8s.io",
	}

	fooClass := &snapshotv1api.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Labels: map[string]string{
				VolumeSnapshotClassSelectorLabel: "foo",
			},
		},
		Driver: "foo.csi.k8s.io",
	}

	barClass := &snapshotv1api.VolumeSnapshotClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "bar",
			Labels: map[string]string{
				VolumeSnapshotClassSelectorLabel: "foo",
			},
		},
		Driver: "bar.csi.k8s.io",
	}

	bazClass := &snapshotv1api.VolumeSnapshotClass{
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
		expectedVSC *snapshotv1api.VolumeSnapshotClass
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
			name:        "should not find foo volumesnapshotclass without \"velero.io/csi-volumesnapshot-class\" label",
			driverName:  "baz.csi.k8s.io",
			expectedVSC: bazClass,
			expectError: true,
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
			actualVSC, actualError := GetVolumeSnapshotClassForStorageClass(tc.driverName, fakeClient.SnapshotV1())

			if tc.expectError {
				assert.NotNil(t, actualError)
				assert.Nil(t, actualVSC)
				return
			}

			assert.Equalf(t, tc.expectedVSC.Name, actualVSC.Name, "unexpected volumesnapshotclass name returned. Want: %s; Got:%s", tc.name, tc.expectedVSC.Name, actualVSC.Name)
			assert.Equalf(t, tc.expectedVSC.Driver, actualVSC.Driver, "unexpected driver name returned. Want: %s; Got:%s", tc.name, tc.expectedVSC.Driver, actualVSC.Driver)
		})
	}
}

func TestGetVolumeSnapshotContentForVolumeSnapshot(t *testing.T) {
	vscName := "snapcontent-7d1bdbd1-d10d-439c-8d8e-e1c2565ddc53"
	snapshotHandle := "snapshot-handle"
	vscObj := &snapshotv1api.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: vscName,
		},
		Spec: snapshotv1api.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: corev1api.ObjectReference{
				Name:       "vol-snap-1",
				APIVersion: snapshotv1api.SchemeGroupVersion.String(),
			},
		},
		Status: &snapshotv1api.VolumeSnapshotContentStatus{
			SnapshotHandle: &snapshotHandle,
		},
	}
	validVS := &snapshotv1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vs",
			Namespace: "default",
		},
		Status: &snapshotv1api.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: &vscName,
		},
	}

	notFound := "does-not-exist"
	vsWithVSCNotFound := &snapshotv1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      notFound,
			Namespace: "default",
		},
		Status: &snapshotv1api.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: &notFound,
		},
	}

	vsWithNilStatus := &snapshotv1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nil-status-vs",
			Namespace: "default",
		},
		Status: nil,
	}
	vsWithNilStatusField := &snapshotv1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nil-status-field-vs",
			Namespace: "default",
		},
		Status: &snapshotv1api.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: nil,
		},
	}

	nilStatusVsc := "nil-status-vsc"
	vscWithNilStatus := &snapshotv1api.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: nilStatusVsc,
		},
		Spec: snapshotv1api.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: corev1api.ObjectReference{
				Name:       "vol-snap-1",
				APIVersion: snapshotv1api.SchemeGroupVersion.String(),
			},
		},
		Status: nil,
	}
	vsForNilStatusVsc := &snapshotv1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vs-for-nil-status-vsc",
			Namespace: "default",
		},
		Status: &snapshotv1api.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: &nilStatusVsc,
		},
	}

	nilStatusFieldVsc := "nil-status-field-vsc"
	vscWithNilStatusField := &snapshotv1api.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: nilStatusFieldVsc,
		},
		Spec: snapshotv1api.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: corev1api.ObjectReference{
				Name:       "vol-snap-1",
				APIVersion: snapshotv1api.SchemeGroupVersion.String(),
			},
		},
		Status: &snapshotv1api.VolumeSnapshotContentStatus{
			SnapshotHandle: nil,
		},
	}
	vsForNilStatusFieldVsc := &snapshotv1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vs-for-nil-status-field",
			Namespace: "default",
		},
		Status: &snapshotv1api.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: &nilStatusFieldVsc,
		},
	}

	objs := []runtime.Object{vscObj, validVS, vsWithVSCNotFound, vsWithNilStatus, vsWithNilStatusField, vscWithNilStatus, vsForNilStatusVsc, vscWithNilStatusField, vsForNilStatusFieldVsc}
	fakeClient := snapshotFake.NewSimpleClientset(objs...)
	testCases := []struct {
		name        string
		volSnap     *snapshotv1api.VolumeSnapshot
		exepctedVSC *snapshotv1api.VolumeSnapshotContent
		wait        bool
		expectError bool
	}{
		{
			name:        "waitEnabled should find volumesnapshotcontent for volumesnapshot",
			volSnap:     validVS,
			exepctedVSC: vscObj,
			wait:        true,
			expectError: false,
		},
		{
			name:        "waitEnabled should not find volumesnapshotcontent for volumesnapshot with non-existing snapshotcontent name in status.BoundVolumeSnapshotContentName",
			volSnap:     vsWithVSCNotFound,
			exepctedVSC: nil,
			wait:        true,
			expectError: true,
		},
		{
			name:        "waitEnabled should not find volumesnapshotcontent for a non-existent volumesnapshot",
			wait:        true,
			exepctedVSC: nil,
			expectError: true,
			volSnap: &snapshotv1api.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "not-found",
					Namespace: "default",
				},
				Status: &snapshotv1api.VolumeSnapshotStatus{
					BoundVolumeSnapshotContentName: &nilStatusVsc,
				},
			},
		},
		{
			name:        "waitDisabled should not find volumesnapshotcontent volumesnapshot status is nil",
			wait:        false,
			expectError: false,
			exepctedVSC: nil,
			volSnap:     vsWithNilStatus,
		},
		{
			name:        "waitDisabled should not find volumesnapshotcontent volumesnapshot status.BoundVolumeSnapshotContentName is nil",
			wait:        false,
			expectError: false,
			exepctedVSC: nil,
			volSnap:     vsWithNilStatusField,
		},
		{
			name:        "waitDisabled should find volumesnapshotcontent volumesnapshotcontent status is nil",
			wait:        false,
			expectError: false,
			exepctedVSC: vscWithNilStatus,
			volSnap:     vsForNilStatusVsc,
		},
		{
			name:        "waitDisabled should find volumesnapshotcontent volumesnapshotcontent status.SnapshotHandle is nil",
			wait:        false,
			expectError: false,
			exepctedVSC: vscWithNilStatusField,
			volSnap:     vsForNilStatusFieldVsc,
		},
		{
			name:        "waitDisabled should not find a non-existent volumesnapshotcontent",
			wait:        false,
			exepctedVSC: nil,
			expectError: true,
			volSnap:     vsWithVSCNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualVSC, actualError := GetVolumeSnapshotContentForVolumeSnapshot(tc.volSnap, fakeClient.SnapshotV1(), logrus.New().WithField("fake", "test"), tc.wait)
			if tc.expectError && actualError == nil {
				assert.NotNil(t, actualError)
				assert.Nil(t, actualVSC)
				return
			}
			assert.Equal(t, tc.exepctedVSC, actualVSC)
		})
	}
}

func TestIsVolumeSnapshotClassHasListerSecret(t *testing.T) {
	testCases := []struct {
		name      string
		snapClass snapshotv1api.VolumeSnapshotClass
		expected  bool
	}{
		{
			name: "should find both annotations",
			snapClass: snapshotv1api.VolumeSnapshotClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "class-1",
					Annotations: map[string]string{
						PrefixedSnapshotterListSecretNameKey:      "snapListSecret",
						PrefixedSnapshotterListSecretNamespaceKey: "awesome-ns",
					},
				},
			},
			expected: true,
		},
		{
			name: "should not find both annotations name is missing",
			snapClass: snapshotv1api.VolumeSnapshotClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "class-1",
					Annotations: map[string]string{
						"foo": "snapListSecret",
						PrefixedSnapshotterListSecretNamespaceKey: "awesome-ns",
					},
				},
			},
			expected: false,
		},
		{
			name: "should not find both annotations namespace is missing",
			snapClass: snapshotv1api.VolumeSnapshotClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "class-1",
					Annotations: map[string]string{
						PrefixedSnapshotterListSecretNameKey: "snapListSecret",
						"foo":                                "awesome-ns",
					},
				},
			},
			expected: false,
		},
		{
			name: "should not find expected annotation non-empty annotation",
			snapClass: snapshotv1api.VolumeSnapshotClass{
				ObjectMeta: metav1.ObjectMeta{
					Name: "class-2",
					Annotations: map[string]string{
						"foo": "snapListSecret",
						"bar": "awesome-ns",
					},
				},
			},
			expected: false,
		},
		{
			name: "should not find expected annotation nil annotation",
			snapClass: snapshotv1api.VolumeSnapshotClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "class-3",
					Annotations: nil,
				},
			},
			expected: false,
		},
		{
			name: "should not find expected annotation empty annotation",
			snapClass: snapshotv1api.VolumeSnapshotClass{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "class-3",
					Annotations: map[string]string{},
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := IsVolumeSnapshotClassHasListerSecret(&tc.snapClass)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestIsVolumeSnapshotContentHasDeleteSecret(t *testing.T) {
	testCases := []struct {
		name     string
		vsc      snapshotv1api.VolumeSnapshotContent
		expected bool
	}{
		{
			name: "should find both annotations",
			vsc: snapshotv1api.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vsc-1",
					Annotations: map[string]string{
						PrefixedSnapshotterSecretNameKey:      "delSnapSecret",
						PrefixedSnapshotterSecretNamespaceKey: "awesome-ns",
					},
				},
			},
			expected: true,
		},
		{
			name: "should not find both annotations name is missing",
			vsc: snapshotv1api.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vsc-2",
					Annotations: map[string]string{
						"foo":                                 "delSnapSecret",
						PrefixedSnapshotterSecretNamespaceKey: "awesome-ns",
					},
				},
			},
			expected: false,
		},
		{
			name: "should not find both annotations namespace is missing",
			vsc: snapshotv1api.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vsc-3",
					Annotations: map[string]string{
						PrefixedSnapshotterSecretNameKey: "delSnapSecret",
						"foo":                            "awesome-ns",
					},
				},
			},
			expected: false,
		},
		{
			name: "should not find expected annotation non-empty annotation",
			vsc: snapshotv1api.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vsc-4",
					Annotations: map[string]string{
						"foo": "delSnapSecret",
						"bar": "awesome-ns",
					},
				},
			},
			expected: false,
		},
		{
			name: "should not find expected annotation empty annotation",
			vsc: snapshotv1api.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "vsc-5",
					Annotations: map[string]string{},
				},
			},
			expected: false,
		},
		{
			name: "should not find expected annotation nil annotation",
			vsc: snapshotv1api.VolumeSnapshotContent{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "vsc-6",
					Annotations: nil,
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := IsVolumeSnapshotContentHasDeleteSecret(&tc.vsc)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestIsVolumeSnapshotHasVSCDeleteSecret(t *testing.T) {
	testCases := []struct {
		name     string
		vs       snapshotv1api.VolumeSnapshot
		expected bool
	}{
		{
			name: "should find both annotations",
			vs: snapshotv1api.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vs-1",
					Annotations: map[string]string{
						"velero.io/csi-deletesnapshotsecret-name":      "snapDelSecret",
						"velero.io/csi-deletesnapshotsecret-namespace": "awesome-ns",
					},
				},
			},
			expected: true,
		},
		{
			name: "should not find both annotations name is missing",
			vs: snapshotv1api.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vs-1",
					Annotations: map[string]string{
						"foo": "snapDelSecret",
						"velero.io/csi-deletesnapshotsecret-namespace": "awesome-ns",
					},
				},
			},
			expected: false,
		},
		{
			name: "should not find both annotations namespace is missing",
			vs: snapshotv1api.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vs-1",
					Annotations: map[string]string{
						"velero.io/csi-deletesnapshotsecret-name": "snapDelSecret",
						"foo": "awesome-ns",
					},
				},
			},
			expected: false,
		},
		{
			name: "should not find annotation non-empty annotation",
			vs: snapshotv1api.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name: "vs-1",
					Annotations: map[string]string{
						"foo": "snapDelSecret",
						"bar": "awesome-ns",
					},
				},
			},
			expected: false,
		},
		{
			name: "should not find annotation empty annotation",
			vs: snapshotv1api.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "vs-1",
					Annotations: map[string]string{},
				},
			},
			expected: false,
		},
		{
			name: "should not find annotation nil annotation",
			vs: snapshotv1api.VolumeSnapshot{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "vs-1",
					Annotations: nil,
				},
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := IsVolumeSnapshotHasVSCDeleteSecret(&tc.vs)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestAddAnnotations(t *testing.T) {
	annotationValues := map[string]string{
		"k1": "v1",
		"k2": "v2",
		"k3": "v3",
		"k4": "v4",
		"k5": "v5",
	}
	testCases := []struct {
		name  string
		o     metav1.ObjectMeta
		toAdd map[string]string
	}{
		{
			name: "should create a new annotation map when annotation is nil",
			o: metav1.ObjectMeta{
				Annotations: nil,
			},
			toAdd: annotationValues,
		},
		{
			name: "should add all supplied annotations into empty annotation",
			o: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
			toAdd: annotationValues,
		},
		{
			name: "should add all supplied annotations to existing annotation",
			o: metav1.ObjectMeta{
				Annotations: map[string]string{
					"k100": "v100",
					"k200": "v200",
					"k300": "v300",
				},
			},
			toAdd: annotationValues,
		},
		{
			name: "should overwrite some existing annotations",
			o: metav1.ObjectMeta{
				Annotations: map[string]string{
					"k100": "v100",
					"k2":   "v200",
					"k300": "v300",
				},
			},
			toAdd: annotationValues,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			AddAnnotations(&tc.o, tc.toAdd)
			for k, v := range tc.toAdd {
				actual, exists := tc.o.Annotations[k]
				assert.True(t, exists)
				assert.Equal(t, v, actual)
			}
		})
	}
}

func TestAddLabels(t *testing.T) {
	labelValues := map[string]string{
		"l1": "v1",
		"l2": "v2",
		"l3": "v3",
		"l4": "v4",
		"l5": "v5",
	}
	testCases := []struct {
		name  string
		o     metav1.ObjectMeta
		toAdd map[string]string
	}{
		{
			name: "should create a new labels map when labels is nil",
			o: metav1.ObjectMeta{
				Labels: nil,
			},
			toAdd: labelValues,
		},
		{
			name: "should add all supplied labels into empty labels",
			o: metav1.ObjectMeta{
				Labels: map[string]string{},
			},
			toAdd: labelValues,
		},
		{
			name: "should add all supplied labels to existing labels",
			o: metav1.ObjectMeta{
				Labels: map[string]string{
					"l100": "v100",
					"l200": "v200",
					"l300": "v300",
				},
			},
			toAdd: labelValues,
		},
		{
			name: "should overwrite some existing labels",
			o: metav1.ObjectMeta{
				Labels: map[string]string{
					"l100": "v100",
					"l2":   "v200",
					"l300": "v300",
				},
			},
			toAdd: labelValues,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			AddLabels(&tc.o, tc.toAdd)
			for k, v := range tc.toAdd {
				actual, exists := tc.o.Labels[k]
				assert.True(t, exists)
				assert.Equal(t, v, actual)
			}
		})
	}
}

func TestIsVolumeSnapshotExists(t *testing.T) {
	vsExists := &snapshotv1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vs-exists",
			Namespace: "default",
		},
	}
	vsNotExists := &snapshotv1api.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vs-does-not-exists",
			Namespace: "default",
		},
	}

	objs := []runtime.Object{vsExists}
	fakeClient := snapshotFake.NewSimpleClientset(objs...)
	testCases := []struct {
		name     string
		expected bool
		vs       *snapshotv1api.VolumeSnapshot
	}{
		{
			name:     "should find existing VolumeSnapshot object",
			expected: true,
			vs:       vsExists,
		},
		{
			name:     "should not find non-existing VolumeSnapshot object",
			expected: false,
			vs:       vsNotExists,
		},
		{
			name:     "should not find a nil VolumeSnapshot object",
			expected: false,
			vs:       nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := IsVolumeSnapshotExists(tc.vs, fakeClient.SnapshotV1())
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestSetVolumeSnapshotContentDeletionPolicy(t *testing.T) {
	testCases := []struct {
		name         string
		inputVSCName string
		objs         []runtime.Object
		expectError  bool
	}{
		{
			name:         "should update DeletionPolicy of a VSC from retain to delete",
			inputVSCName: "retainVSC",
			objs: []runtime.Object{
				&snapshotv1api.VolumeSnapshotContent{
					ObjectMeta: metav1.ObjectMeta{
						Name: "retainVSC",
					},
					Spec: snapshotv1api.VolumeSnapshotContentSpec{
						DeletionPolicy: snapshotv1api.VolumeSnapshotContentRetain,
					},
				},
			},
			expectError: false,
		},
		{
			name:         "should be a no-op updating if DeletionPolicy of a VSC is already Delete",
			inputVSCName: "deleteVSC",
			objs: []runtime.Object{
				&snapshotv1api.VolumeSnapshotContent{
					ObjectMeta: metav1.ObjectMeta{
						Name: "deleteVSC",
					},
					Spec: snapshotv1api.VolumeSnapshotContentSpec{
						DeletionPolicy: snapshotv1api.VolumeSnapshotContentDelete,
					},
				},
			},
			expectError: false,
		},
		{
			name:         "should update DeletionPolicy of a VSC with no DeletionPolicy",
			inputVSCName: "nothingVSC",
			objs: []runtime.Object{
				&snapshotv1api.VolumeSnapshotContent{
					ObjectMeta: metav1.ObjectMeta{
						Name: "nothingVSC",
					},
					Spec: snapshotv1api.VolumeSnapshotContentSpec{},
				},
			},
			expectError: false,
		},
		{
			name:         "should return not found error if supplied VSC does not exist",
			inputVSCName: "does-not-exist",
			objs:         []runtime.Object{},
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := snapshotFake.NewSimpleClientset(tc.objs...)
			err := SetVolumeSnapshotContentDeletionPolicy(tc.inputVSCName, fakeClient.SnapshotV1())
			if tc.expectError {
				assert.NotNil(t, err)
			} else {
				assert.Nil(t, err)
				actual, err := fakeClient.SnapshotV1().VolumeSnapshotContents().Get(context.TODO(), tc.inputVSCName, metav1.GetOptions{})
				assert.Nil(t, err)
				assert.Equal(t, snapshotv1api.VolumeSnapshotContentDelete, actual.Spec.DeletionPolicy)
			}
		})
	}
}

func TestIsByBackup(t *testing.T) {
	testCases := []struct {
		name       string
		o          metav1.ObjectMeta
		backupName string
		expected   bool
	}{
		{
			name:     "object has no labels",
			o:        metav1.ObjectMeta{},
			expected: false,
		},
		{
			name:       "object has no velero backup label",
			backupName: "csi-b1",
			o: metav1.ObjectMeta{
				Labels: map[string]string{
					"l100": "v100",
					"l2":   "v200",
					"l300": "v300",
				},
			},
			expected: false,
		},
		{
			name:       "object has velero backup label but value not equal to backup name",
			backupName: "csi-b1",
			o: metav1.ObjectMeta{
				Labels: map[string]string{
					"velero.io/backup-name": "does-not-match",
					"l100":                  "v100",
					"l2":                    "v200",
					"l300":                  "v300",
				},
			},
			expected: false,
		},
		{
			name:       "object has backup label with matchin backup name value",
			backupName: "does-match",
			o: metav1.ObjectMeta{
				Labels: map[string]string{
					"velero.io/backup-name": "does-match",
					"l100":                  "v100",
					"l2":                    "v200",
					"l300":                  "v300",
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		actual := HasBackupLabel(&tc.o, tc.backupName)
		assert.Equal(t, tc.expected, actual)
	}
}
