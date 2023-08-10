/*
Copyright 2019, 2020 the Velero contributors.

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
	"context"
	"fmt"
	"testing"
	"time"

	snapshotv1api "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	v1 "github.com/kubernetes-csi/external-snapshotter/client/v4/apis/volumesnapshot/v1"
	snapshotfake "github.com/kubernetes-csi/external-snapshotter/client/v4/clientset/versioned/fake"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
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

func TestExecute(t *testing.T) {
	boolTrue := true
	tests := []struct {
		name               string
		backup             *velerov1api.Backup
		pvc                *corev1.PersistentVolumeClaim
		pv                 *corev1.PersistentVolume
		sc                 *storagev1.StorageClass
		vsClass            *snapshotv1api.VolumeSnapshotClass
		operationID        string
		expectedErr        error
		expectedBackup     *velerov1api.Backup
		expectedDataUpload *velerov2alpha1.DataUpload
		expectedPVC        *corev1.PersistentVolumeClaim
	}{
		{
			name:        "Skip PVC handling if SnapshotVolume set to false",
			backup:      builder.ForBackup("velero", "test").SnapshotVolumes(false).Result(),
			expectedErr: nil,
		},
		{
			name:        "Skip PVC BIA when backup is in finalizing phase",
			backup:      builder.ForBackup("velero", "test").Phase(velerov1api.BackupPhaseFinalizing).Result(),
			expectedErr: nil,
		},
		{
			name:        "Test SnapshotMoveData",
			backup:      builder.ForBackup("velero", "test").SnapshotMoveData(true).Result(),
			pvc:         builder.ForPersistentVolumeClaim("velero", "testPVC").VolumeName("testPV").StorageClass("testSC").Phase(corev1.ClaimBound).Result(),
			pv:          builder.ForPersistentVolume("testPV").CSI("hostpath", "testVolume").Result(),
			sc:          builder.ForStorageClass("testSC").Provisioner("hostpath").Result(),
			vsClass:     builder.ForVolumeSnapshotClass("tescVSClass").Driver("hostpath").ObjectMeta(builder.WithLabels(util.VolumeSnapshotClassSelectorLabel, "")).Result(),
			operationID: ".",
			expectedErr: nil,
			expectedDataUpload: &velerov2alpha1.DataUpload{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DataUpload",
					APIVersion: velerov2alpha1.SchemeGroupVersion.String(),
				},
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
					Namespace:    "velero",
					Labels: map[string]string{
						velerov1api.BackupNameLabel:       "test",
						velerov1api.BackupUIDLabel:        "",
						velerov1api.PVCUIDLabel:           "",
						velerov1api.AsyncOperationIDLabel: "du-.",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "velero.io/v1",
							Kind:       "Backup",
							Name:       "test",
							UID:        "",
							Controller: &boolTrue,
						},
					},
				},
				Spec: velerov2alpha1.DataUploadSpec{
					SnapshotType: velerov2alpha1.SnapshotTypeCSI,
					CSISnapshot: &velerov2alpha1.CSISnapshotSpec{
						VolumeSnapshot: "",
						StorageClass:   "testSC",
						SnapshotClass:  "",
					},
					SourcePVC:       "testPVC",
					SourceNamespace: "velero",
				},
			},
		},
		{
			name:        "Verify PVC is modified as expected",
			backup:      builder.ForBackup("velero", "test").SnapshotMoveData(true).Result(),
			pvc:         builder.ForPersistentVolumeClaim("velero", "testPVC").VolumeName("testPV").StorageClass("testSC").Phase(corev1.ClaimBound).Result(),
			pv:          builder.ForPersistentVolume("testPV").CSI("hostpath", "testVolume").Result(),
			sc:          builder.ForStorageClass("testSC").Provisioner("hostpath").Result(),
			vsClass:     builder.ForVolumeSnapshotClass("tescVSClass").Driver("hostpath").ObjectMeta(builder.WithLabels(util.VolumeSnapshotClassSelectorLabel, "")).Result(),
			operationID: ".",
			expectedErr: nil,
			expectedPVC: builder.ForPersistentVolumeClaim("velero", "testPVC").
				ObjectMeta(builder.WithAnnotations(util.MustIncludeAdditionalItemAnnotation, "true", util.DataUploadNameAnnotation, "velero/", util.VolumeSnapshotLabel, ""),
					builder.WithLabels(velerov1api.BackupNameLabel, "test", util.VolumeSnapshotLabel, "")).
				VolumeName("testPV").StorageClass("testSC").Phase(corev1.ClaimBound).Result(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(*testing.T) {
			client := fake.NewSimpleClientset()
			snapshotClient := snapshotfake.NewSimpleClientset()
			veleroClient := velerofake.NewSimpleClientset()
			logger := logrus.New()
			logger.Level = logrus.DebugLevel

			if tc.pvc != nil {
				_, err := client.CoreV1().PersistentVolumeClaims(tc.pvc.Namespace).Create(context.Background(), tc.pvc, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			if tc.pv != nil {
				_, err := client.CoreV1().PersistentVolumes().Create(context.Background(), tc.pv, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			if tc.sc != nil {
				_, err := client.StorageV1().StorageClasses().Create(context.Background(), tc.sc, metav1.CreateOptions{})
				require.NoError(t, err)
			}
			if tc.vsClass != nil {
				_, err := snapshotClient.SnapshotV1().VolumeSnapshotClasses().Create(context.Background(), tc.vsClass, metav1.CreateOptions{})
				require.NoError(t, err)
			}

			pvcBIA := PVCBackupItemAction{
				Log:            logger,
				Client:         client,
				SnapshotClient: snapshotClient,
				VeleroClient:   veleroClient,
			}

			pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&tc.pvc)
			require.NoError(t, err)

			if boolptr.IsSetToTrue(tc.backup.Spec.SnapshotMoveData) == true {
				go func() {
					var vsList *v1.VolumeSnapshotList
					err := wait.PollImmediate(1*time.Second, 10*time.Second, func() (bool, error) {
						vsList, err = pvcBIA.SnapshotClient.SnapshotV1().VolumeSnapshots(tc.pvc.Namespace).List(context.Background(), metav1.ListOptions{})
						require.NoError(t, err)
						if err != nil || len(vsList.Items) == 0 {
							return false, nil
						}
						return true, nil
					})

					require.NoError(t, err)
					vscName := "testVSC"
					vsList.Items[0].Status = &v1.VolumeSnapshotStatus{BoundVolumeSnapshotContentName: &vscName}
					_, err = pvcBIA.SnapshotClient.SnapshotV1().VolumeSnapshots(vsList.Items[0].Namespace).UpdateStatus(context.Background(), &vsList.Items[0], metav1.UpdateOptions{})
					require.NoError(t, err)

					handleName := "testHandle"
					vsc := builder.ForVolumeSnapshotContent("testVSC").Status(&snapshotv1api.VolumeSnapshotContentStatus{SnapshotHandle: &handleName}).Result()
					_, err = pvcBIA.SnapshotClient.SnapshotV1().VolumeSnapshotContents().Create(context.Background(), vsc, metav1.CreateOptions{})
					require.NoError(t, err)
				}()
			}

			resultUnstructed, _, _, _, err := pvcBIA.Execute(&unstructured.Unstructured{Object: pvcMap}, tc.backup)
			if tc.expectedErr != nil {
				require.Equal(t, err, tc.expectedErr)
			}

			if tc.expectedDataUpload != nil {
				dataUploadList, err := veleroClient.VeleroV2alpha1().DataUploads(tc.backup.Namespace).List(context.Background(), metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", velerov1api.BackupNameLabel, tc.backup.Name)})
				require.NoError(t, err)
				require.Equal(t, 1, len(dataUploadList.Items))
				require.Equal(t, *tc.expectedDataUpload, dataUploadList.Items[0])
			}

			if tc.expectedPVC != nil {
				resultPVC := new(corev1.PersistentVolumeClaim)
				runtime.DefaultUnstructuredConverter.FromUnstructured(resultUnstructed.UnstructuredContent(), resultPVC)
				require.Equal(t, *tc.expectedPVC, *resultPVC)
			}
		})
	}
}

func TestProgress(t *testing.T) {
	currentTime := time.Now()
	tests := []struct {
		name             string
		backup           *velerov1api.Backup
		dataUpload       velerov2alpha1.DataUpload
		operationID      string
		expectedErr      string
		expectedProgress velero.OperationProgress
	}{
		{
			name:        "DataUpload cannot be found",
			backup:      builder.ForBackup("velero", "test").Result(),
			operationID: "testing",
			expectedErr: "not found DataUpload for operationID testing",
		},
		{
			name:   "DataUpload is found",
			backup: builder.ForBackup("velero", "test").Result(),
			dataUpload: velerov2alpha1.DataUpload{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DataUpload",
					APIVersion: "v2alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      "testing",
					Labels: map[string]string{
						velerov1api.AsyncOperationIDLabel: "testing",
					},
				},
				Status: velerov2alpha1.DataUploadStatus{
					Phase: velerov2alpha1.DataUploadPhaseFailed,
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
			client := fake.NewSimpleClientset()
			snapshotClient := snapshotfake.NewSimpleClientset()
			veleroClient := velerofake.NewSimpleClientset()
			logger := logrus.New()

			pvcBIA := PVCBackupItemAction{
				Log:            logger,
				Client:         client,
				SnapshotClient: snapshotClient,
				VeleroClient:   veleroClient,
			}

			_, err := veleroClient.VeleroV2alpha1().DataUploads(tc.dataUpload.Namespace).Create(context.Background(), &tc.dataUpload, metav1.CreateOptions{})
			require.NoError(t, err)

			progress, err := pvcBIA.Progress(tc.operationID, tc.backup)
			if tc.expectedErr != "" {
				require.Equal(t, tc.expectedErr, err.Error())
			}
			require.Equal(t, tc.expectedProgress, progress)
		})
	}
}

func TestCancel(t *testing.T) {
	tests := []struct {
		name               string
		backup             *velerov1api.Backup
		dataUpload         velerov2alpha1.DataUpload
		operationID        string
		expectedErr        error
		expectedDataUpload velerov2alpha1.DataUpload
	}{
		{
			name:   "Cancel DataUpload",
			backup: builder.ForBackup("velero", "test").Result(),
			dataUpload: velerov2alpha1.DataUpload{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DataUpload",
					APIVersion: "v2alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      "testing",
					Labels: map[string]string{
						velerov1api.AsyncOperationIDLabel: "testing",
					},
				},
			},
			operationID: "testing",
			expectedErr: nil,
			expectedDataUpload: velerov2alpha1.DataUpload{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DataUpload",
					APIVersion: "v2alpha1",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      "testing",
					Labels: map[string]string{
						velerov1api.AsyncOperationIDLabel: "testing",
					},
				},
				Spec: velerov2alpha1.DataUploadSpec{
					Cancel: true,
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(*testing.T) {
			client := fake.NewSimpleClientset()
			snapshotClient := snapshotfake.NewSimpleClientset()
			veleroClient := velerofake.NewSimpleClientset()
			logger := logrus.New()

			pvcBIA := PVCBackupItemAction{
				Log:            logger,
				Client:         client,
				SnapshotClient: snapshotClient,
				VeleroClient:   veleroClient,
			}

			_, err := veleroClient.VeleroV2alpha1().DataUploads(tc.dataUpload.Namespace).Create(context.Background(), &tc.dataUpload, metav1.CreateOptions{})
			require.NoError(t, err)

			err = pvcBIA.Cancel(tc.operationID, tc.backup)
			if tc.expectedErr != nil {
				require.Equal(t, err, tc.expectedErr)
			}

			du, err := veleroClient.VeleroV2alpha1().DataUploads(tc.dataUpload.Namespace).Get(context.Background(), tc.dataUpload.Name, metav1.GetOptions{})
			require.NoError(t, err)

			require.Equal(t, *du, tc.expectedDataUpload)
		})
	}
}
