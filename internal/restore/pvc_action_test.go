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
	"context"
	"testing"
	"time"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	snapshotfake "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned/fake"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/vmware-tanzu/velero-plugin-for-csi/internal/util"
	"github.com/vmware-tanzu/velero/pkg/apis/velero/shared"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	"github.com/vmware-tanzu/velero/pkg/builder"
	velerofake "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned/fake"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	"github.com/vmware-tanzu/velero/pkg/util/boolptr"
)

func TestRemovePVCAnnotations(t *testing.T) {
	testCases := []struct {
		name                string
		pvc                 corev1api.PersistentVolumeClaim
		removeAnnotations   []string
		expectedAnnotations map[string]string
	}{
		{
			name: "should create empty annotation map",
			pvc: corev1api.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: nil,
				},
			},
			removeAnnotations:   []string{"foo"},
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
			removeAnnotations: []string{},
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
			removeAnnotations:   []string{"ann1", "ann2", "ann3", "ann4"},
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
			removeAnnotations: []string{"ann1", "ann2", "ann3", "ann4"},
			expectedAnnotations: map[string]string{
				"ann5": "ann5-val",
				"ann6": "ann6-val",
				"ann7": "ann7-val",
				"ann8": "ann8-val",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			removePVCAnnotations(&tc.pvc, tc.removeAnnotations)
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
					DataSourceRef: &corev1api.TypedLocalObjectReference{
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
			assert.Equalf(t, tc.pvc.Spec.DataSource.Kind, util.VolumeSnapshotKindName, "expected change to Spec.DataSource.Kind missing, Want: VolumeSnapshot, Got: %s", tc.pvc.Spec.DataSource.Kind)
			assert.Equalf(t, tc.pvc.Spec.DataSource.Name, tc.vsName, "expected change to Spec.DataSource.Name missing, Want: %s, Got: %s", tc.vsName, tc.pvc.Spec.DataSource.Name)
			assert.Equalf(t, tc.pvc.Spec.DataSourceRef.Name, tc.vsName, "expected change to Spec.DataSourceRef.Name missing, Want: %s, Got: %s", tc.vsName, tc.pvc.Spec.DataSourceRef.Name)
		})
	}
}

func TestResetPVCResourceRequest(t *testing.T) {
	var storageReq50Mi, storageReq1Gi, cpuQty resource.Quantity

	storageReq50Mi, err := resource.ParseQuantity("50Mi")
	assert.NoError(t, err)
	storageReq1Gi, err = resource.ParseQuantity("1Gi")
	assert.NoError(t, err)
	cpuQty, err = resource.ParseQuantity("100m")
	assert.NoError(t, err)

	testCases := []struct {
		name                      string
		pvc                       corev1api.PersistentVolumeClaim
		restoreSize               resource.Quantity
		expectedStorageRequestQty string
	}{
		{
			name: "should set storage resource request from volumesnapshot, pvc has nil resource requests",
			pvc: corev1api.PersistentVolumeClaim{
				Spec: corev1api.PersistentVolumeClaimSpec{
					Resources: corev1api.ResourceRequirements{
						Requests: nil,
					},
				},
			},
			restoreSize:               storageReq50Mi,
			expectedStorageRequestQty: "50Mi",
		},
		{
			name: "should set storage resource request from volumesnapshot, pvc has empty resource requests",
			pvc: corev1api.PersistentVolumeClaim{
				Spec: corev1api.PersistentVolumeClaimSpec{
					Resources: corev1api.ResourceRequirements{
						Requests: corev1api.ResourceList{},
					},
				},
			},
			restoreSize:               storageReq50Mi,
			expectedStorageRequestQty: "50Mi",
		},
		{
			name: "should merge resource requests from volumesnapshot into pvc with no storage resource requests",
			pvc: corev1api.PersistentVolumeClaim{
				Spec: corev1api.PersistentVolumeClaimSpec{
					Resources: corev1api.ResourceRequirements{
						Requests: corev1api.ResourceList{
							corev1api.ResourceCPU: cpuQty,
						},
					},
				},
			},
			restoreSize:               storageReq50Mi,
			expectedStorageRequestQty: "50Mi",
		},
		{
			name: "should set storage resource request from volumesnapshot, pvc requests less storage",
			pvc: corev1api.PersistentVolumeClaim{
				Spec: corev1api.PersistentVolumeClaimSpec{
					Resources: corev1api.ResourceRequirements{
						Requests: corev1api.ResourceList{
							corev1api.ResourceStorage: storageReq50Mi,
						},
					},
				},
			},
			restoreSize:               storageReq1Gi,
			expectedStorageRequestQty: "1Gi",
		},
		{
			name: "should not set storage resource request from volumesnapshot, pvc requests more storage",
			pvc: corev1api.PersistentVolumeClaim{
				Spec: corev1api.PersistentVolumeClaimSpec{
					Resources: corev1api.ResourceRequirements{
						Requests: corev1api.ResourceList{
							corev1api.ResourceStorage: storageReq1Gi,
						},
					},
				},
			},
			restoreSize:               storageReq50Mi,
			expectedStorageRequestQty: "1Gi",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			log := logrus.New().WithField("unit-test", tc.name)
			setPVCStorageResourceRequest(&tc.pvc, tc.restoreSize, log)
			expected, err := resource.ParseQuantity(tc.expectedStorageRequestQty)
			assert.NoError(t, err)
			assert.Equal(t, expected, tc.pvc.Spec.Resources.Requests[corev1api.ResourceStorage])
		})
	}
}

func TestProgress(t *testing.T) {
	currentTime := time.Now()
	tests := []struct {
		name             string
		restore          *velerov1api.Restore
		dataDownload     *velerov2alpha1.DataDownload
		operationID      string
		expectedErr      string
		expectedProgress velero.OperationProgress
	}{
		{
			name:        "DataDownload cannot be found",
			restore:     builder.ForRestore("velero", "test").Result(),
			operationID: "testing",
			expectedErr: "didn't find DataDownload",
		},
		{
			name:    "DataUpload is found",
			restore: builder.ForRestore("velero", "test").Result(),
			dataDownload: &velerov2alpha1.DataDownload{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DataUpload",
					APIVersion: "v2alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      "testing",
					Labels: map[string]string{
						util.AsyncOperationIDLabel: "testing",
					},
				},
				Status: velerov2alpha1.DataDownloadStatus{
					Phase: velerov2alpha1.DataDownloadPhaseFailed,
					Progress: shared.DataMoveOperationProgress{
						BytesDone:  1000,
						TotalBytes: 1000,
					},
					StartTimestamp:      &metav1.Time{Time: currentTime},
					CompletionTimestamp: &metav1.Time{Time: currentTime},
					Message:             "Testing error",
				},
			},
			operationID: "testing",
			expectedProgress: velero.OperationProgress{
				Completed:      true,
				Err:            "Testing error",
				NCompleted:     1000,
				NTotal:         1000,
				OperationUnits: "Bytes",
				Description:    "Failed",
				Started:        currentTime,
				Updated:        currentTime,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(*testing.T) {
			pvcRIA := PVCRestoreItemAction{
				Log:          logrus.New(),
				VeleroClient: velerofake.NewSimpleClientset(),
			}
			if tc.dataDownload != nil {
				_, err := pvcRIA.VeleroClient.VeleroV2alpha1().DataDownloads(tc.dataDownload.Namespace).Create(context.Background(), tc.dataDownload, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			progress, err := pvcRIA.Progress(tc.operationID, tc.restore)
			if tc.expectedErr != "" {
				require.Equal(t, tc.expectedErr, err.Error())
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expectedProgress, progress)
		})
	}
}

func TestCancel(t *testing.T) {
	tests := []struct {
		name                 string
		restore              *velerov1api.Restore
		dataDownload         *velerov2alpha1.DataDownload
		operationID          string
		expectedErr          string
		expectedDataDownload velerov2alpha1.DataDownload
	}{
		{
			name:    "Cancel DataUpload",
			restore: builder.ForRestore("velero", "test").Result(),
			dataDownload: &velerov2alpha1.DataDownload{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DataDownload",
					APIVersion: "v2alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      "testing",
					Labels: map[string]string{
						util.AsyncOperationIDLabel: "testing",
					},
				},
			},
			operationID: "testing",
			expectedErr: "",
			expectedDataDownload: velerov2alpha1.DataDownload{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DataDownload",
					APIVersion: "v2alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      "testing",
					Labels: map[string]string{
						util.AsyncOperationIDLabel: "testing",
					},
				},
				Spec: velerov2alpha1.DataDownloadSpec{
					Cancel: true,
				},
			},
		},
		{
			name:         "Cannot find DataUpload",
			restore:      builder.ForRestore("velero", "test").Result(),
			dataDownload: nil,
			operationID:  "testing",
			expectedErr:  "didn't find DataDownload",
			expectedDataDownload: velerov2alpha1.DataDownload{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DataDownload",
					APIVersion: "v2alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      "testing",
					Labels: map[string]string{
						util.AsyncOperationIDLabel: "testing",
					},
				},
				Spec: velerov2alpha1.DataDownloadSpec{
					Cancel: true,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(*testing.T) {
			pvcRIA := PVCRestoreItemAction{
				Log:          logrus.New(),
				VeleroClient: velerofake.NewSimpleClientset(),
			}
			if tc.dataDownload != nil {
				_, err := pvcRIA.VeleroClient.VeleroV2alpha1().DataDownloads(tc.dataDownload.Namespace).Create(context.Background(), tc.dataDownload, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			err := pvcRIA.Cancel(tc.operationID, tc.restore)
			if tc.expectedErr != "" {
				require.Equal(t, tc.expectedErr, err.Error())
				return
			}
			require.NoError(t, err)

			resultDataDownload, err := pvcRIA.VeleroClient.VeleroV2alpha1().DataDownloads(tc.dataDownload.Namespace).Get(context.Background(), tc.dataDownload.Name, metav1.GetOptions{})
			require.NoError(t, err)

			require.Equal(t, tc.expectedDataDownload, *resultDataDownload)
		})
	}
}

func TestExecute(t *testing.T) {
	tests := []struct {
		name                 string
		backup               *velerov1api.Backup
		restore              *velerov1api.Restore
		pvc                  *corev1api.PersistentVolumeClaim
		vs                   *snapshotv1api.VolumeSnapshot
		dataUploadResult     *corev1api.ConfigMap
		expectedErr          string
		expectedDataDownload *velerov2alpha1.DataDownload
		expectedPVC          *corev1api.PersistentVolumeClaim
	}{
		{
			name:        "Don't restore PV",
			restore:     builder.ForRestore("velero", "testRestore").Backup("testBackup").RestorePVs(false).Result(),
			pvc:         builder.ForPersistentVolumeClaim("velero", "testPVC").Result(),
			expectedPVC: builder.ForPersistentVolumeClaim("velero", "testPVC").VolumeName("").Result(),
		},
		{
			name:        "restore's backup cannot be found",
			restore:     builder.ForRestore("velero", "testRestore").Backup("testBackup").Result(),
			pvc:         builder.ForPersistentVolumeClaim("velero", "testPVC").Result(),
			expectedErr: "fail to get backup for restore: backups.velero.io \"testBackup\" not found",
		},
		{
			name:        "VolumeSnapshot cannot be found",
			backup:      builder.ForBackup("velero", "testBackup").Result(),
			restore:     builder.ForRestore("velero", "testRestore").Backup("testBackup").Result(),
			pvc:         builder.ForPersistentVolumeClaim("velero", "testPVC").ObjectMeta(builder.WithAnnotations(util.VolumeSnapshotLabel, "testVS")).Result(),
			expectedErr: "Failed to get Volumesnapshot velero/testVS to restore PVC velero/testPVC: volumesnapshots.snapshot.storage.k8s.io \"testVS\" not found",
		},
		{
			name:    "Restore from VolumeSnapshot",
			backup:  builder.ForBackup("velero", "testBackup").Result(),
			restore: builder.ForRestore("velero", "testRestore").Backup("testBackup").Result(),
			pvc: builder.ForPersistentVolumeClaim("velero", "testPVC").ObjectMeta(builder.WithAnnotations(util.VolumeSnapshotLabel, "testVS")).
				RequestResource(map[corev1api.ResourceName]resource.Quantity{corev1api.ResourceStorage: resource.MustParse("10Gi")}).
				DataSource(&corev1api.TypedLocalObjectReference{APIGroup: &snapshotv1api.SchemeGroupVersion.Group, Kind: util.VolumeSnapshotKindName, Name: "testVS"}).
				DataSourceRef(&corev1api.TypedLocalObjectReference{APIGroup: &snapshotv1api.SchemeGroupVersion.Group, Kind: util.VolumeSnapshotKindName, Name: "testVS"}).
				Result(),
			vs:          builder.ForVolumeSnapshot("velero", "testVS").ObjectMeta(builder.WithAnnotations(util.VolumeSnapshotRestoreSize, "10Gi")).Result(),
			expectedPVC: builder.ForPersistentVolumeClaim("velero", "testPVC").ObjectMeta(builder.WithAnnotations("velero.io/volume-snapshot-name", "testVS")).Result(),
		},
		{
			name:        "DataUploadResult cannot be found",
			backup:      builder.ForBackup("velero", "testBackup").SnapshotMoveData(true).Result(),
			restore:     builder.ForRestore("velero", "testRestore").Backup("testBackup").Result(),
			pvc:         builder.ForPersistentVolumeClaim("velero", "testPVC").ObjectMeta(builder.WithAnnotations(util.VolumeSnapshotRestoreSize, "10Gi")).Result(),
			expectedPVC: builder.ForPersistentVolumeClaim("velero", "testPVC").Result(),
			expectedErr: "fail get DataUploadResult for restore: testRestore: no DataUpload result cm found with labels velero.io/pvc-namespace-name=velero.testPVC,velero.io/restore-uid=",
		},
		{
			name:             "Restore from DataUploadResult",
			backup:           builder.ForBackup("velero", "testBackup").SnapshotMoveData(true).Result(),
			restore:          builder.ForRestore("velero", "testRestore").Backup("testBackup").ObjectMeta(builder.WithUID("uid")).Result(),
			pvc:              builder.ForPersistentVolumeClaim("velero", "testPVC").ObjectMeta(builder.WithAnnotations(util.VolumeSnapshotRestoreSize, "10Gi")).Result(),
			dataUploadResult: builder.ForConfigMap("velero", "testCM").Data("uid", "{}").ObjectMeta(builder.WithLabels(velerov1api.RestoreUIDLabel, "uid", util.PVCNamespaceNameLabel, "velero.testPVC")).Result(),
			expectedPVC:      builder.ForPersistentVolumeClaim("velero", "testPVC").ObjectMeta(builder.WithAnnotations("velero.io/vsi-volumesnapshot-restore-size", "10Gi")).Result(),
			expectedDataDownload: builder.ForDataDownload("velero", "").TargetVolume(velerov2alpha1.TargetVolumeSpec{PVC: "testPVC", Namespace: "velero"}).
				ObjectMeta(builder.WithOwnerReference([]metav1.OwnerReference{{APIVersion: velerov1api.SchemeGroupVersion.String(), Kind: "Restore", Name: "testRestore", UID: "uid", Controller: boolptr.True()}}),
					builder.WithLabelsMap(map[string]string{util.AsyncOperationIDLabel: "dd-uid.", velerov1api.RestoreNameLabel: "testRestore", velerov1api.RestoreUIDLabel: "uid"}),
					builder.WithGenerateName("testRestore-")).Result(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(*testing.T) {
			pvcRIA := PVCRestoreItemAction{
				Log:            logrus.New(),
				Client:         fake.NewSimpleClientset(),
				SnapshotClient: snapshotfake.NewSimpleClientset(),
				VeleroClient:   velerofake.NewSimpleClientset(),
			}
			input := new(velero.RestoreItemActionExecuteInput)

			if tc.pvc != nil {
				pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(tc.pvc)
				require.NoError(t, err)

				input.Item = &unstructured.Unstructured{Object: pvcMap}
				input.ItemFromBackup = &unstructured.Unstructured{Object: pvcMap}
				input.Restore = tc.restore
			}

			if tc.backup != nil {
				_, err := pvcRIA.VeleroClient.VeleroV1().Backups(tc.backup.Namespace).Create(context.Background(), tc.backup, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			if tc.vs != nil {
				_, err := pvcRIA.SnapshotClient.SnapshotV1().VolumeSnapshots(tc.vs.Namespace).Create(context.Background(), tc.vs, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			if tc.dataUploadResult != nil {
				_, err := pvcRIA.Client.CoreV1().ConfigMaps(tc.dataUploadResult.Namespace).Create(context.Background(), tc.dataUploadResult, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			output, err := pvcRIA.Execute(input)
			if tc.expectedErr != "" {
				require.Equal(t, tc.expectedErr, err.Error())
				return
			}
			require.NoError(t, err)

			if tc.expectedPVC != nil {
				pvc := new(corev1api.PersistentVolumeClaim)
				err := runtime.DefaultUnstructuredConverter.FromUnstructured(output.UpdatedItem.UnstructuredContent(), pvc)
				require.NoError(t, err)
				require.Equal(t, tc.expectedPVC.GetObjectMeta(), pvc.GetObjectMeta())
				if pvc.Spec.Selector != nil && pvc.Spec.Selector.MatchLabels != nil {
					require.Contains(t, pvc.Spec.Selector.MatchLabels[util.DynamicPVRestoreLabel], tc.pvc.Namespace+"."+tc.pvc.Name)
				}
			}
			if tc.expectedDataDownload != nil {
				dataDownload, err := pvcRIA.VeleroClient.VeleroV2alpha1().DataDownloads(tc.expectedDataDownload.Namespace).Get(context.Background(), tc.expectedDataDownload.Name, metav1.GetOptions{})
				require.NoError(t, err)
				require.Equal(t, tc.expectedDataDownload, dataDownload)
			}
		})
	}
}
