package main

import (
	"testing"

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
		actualPV, actualError := getPVForPVC(tc.inPVC, fakeClient.CoreV1())

		if tc.expectError && actualError == nil {
			t.Fatalf("getPVForPVC failed for %s, Want error; Got nil error", tc.name)
		}
		if tc.expectError && actualPV != nil {
			t.Fatalf("getPVForPVC failed for %s, Want PV: nil; Got PV: %q", tc.name, actualPV)
		}

		if !tc.expectError && actualError != nil {
			t.Fatalf("getPVForPVC failed for %s, Want: nil error; Got: %v", tc.name, actualError)
		}
		if !tc.expectError && actualPV.Name != tc.expectedPV.Name {
			t.Fatalf("getPVForPVC failed for %s, Want PV with name %q; Got PV with name %q", tc.name, tc.expectedPV.Name, actualPV.Name)
		}
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
			pvcName:          "unused-pvc",
			expectedPodCount: 0,
		},
	}

	for _, tc := range testCases {
		actualPods, err := getPodsUsingPVC(tc.pvcNamespace, tc.pvcName, fakeClient.CoreV1())
		if err != nil {
			t.Fatalf("getPodsUsingPVC failed for %s, Want error=nil; Got error=%v", tc.name, err)
		}

		if len(actualPods) != tc.expectedPodCount {
			t.Fatalf("getPodsUsingPVC failed for %s, unexpected number of pods in result; Want: %d; Got: %d", tc.name, tc.expectedPodCount, len(actualPods))
		}
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
			pvcName:     "vsi-pvc2",
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
		actualVolumeName, err := getPodVolumeNameForPVC(tc.pod, tc.pvcName)
		if tc.expectError && err == nil {
			t.Fatalf("getPodVolumeNameForPVC failed for %s, Want error; Got nil error", tc.name)
		}
		if !tc.expectError && tc.expectedVolumeName != actualVolumeName {
			t.Fatalf("getPodVolumeNameForPVC failed for %s, unexpected podVolumename returned. Want %s; Got %s", tc.name, tc.expectedVolumeName, actualVolumeName)
		}
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
		actualResticVolumes := getPodVolumesUsingRestic(tc.pod)
		if len(tc.expectedResticVolumes) != len(actualResticVolumes) {
			t.Fatalf("getPodVolumesUsingRestic failed for %q, unexpected number of volumes using resitc, Want: %d; Got: %d", tc.name, len(tc.expectedResticVolumes),
				len(actualResticVolumes))
		}
	}
}
