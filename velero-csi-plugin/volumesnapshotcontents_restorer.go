/*
Copyright 2018, 2019 the Velero contributors.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/heptio/velero/pkg/plugin/velero"
	snapshotv1beta1api "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1beta1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// VSCRestorer is a restore item action plugin for Velero
type VSCRestorer struct {
	log logrus.FieldLogger
}

func (p *VSCRestorer) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshotcontent.snapshot.storage.k8s.io"},
	}, nil
}

func (p *VSCRestorer) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.log.Info("Starting VSCRestorer")
	var vsc snapshotv1beta1api.VolumeSnapshotContent

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &vsc); err != nil {
		return &velero.RestoreItemActionExecuteOutput{}, err
	}

	_, snapshotClient, err := getClients()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	snapRef := vsc.Spec.VolumeSnapshotRef
	volumeSnapshot, err := snapshotClient.SnapshotV1beta1().VolumeSnapshots(snapRef.Namespace).Get(snapRef.Name, metav1.GetOptions{})
	//TODO: better error handling, account for a 404
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Make sure we're referencing the new UID for the restored VolumeSnapshot
	// This is likely unnecessary, but what _is_ necessary is that the old UID be cleared.
	vsc.Spec.VolumeSnapshotRef.UID = volumeSnapshot.UID

	vscMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&vsc)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	p.log.Info("Returning from VSCRestorer")

	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem: &unstructured.Unstructured{Object: vscMap},
	}, nil
}
