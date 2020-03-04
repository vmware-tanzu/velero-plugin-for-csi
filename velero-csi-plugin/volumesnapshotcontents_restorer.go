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
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	snapshotv1beta1api "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
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

	p.log.Infof("unstructured content: ")
	for k, v := range input.Item.UnstructuredContent() {
		p.log.Infof("%s: %s", k, v)
	}
	vscSnapshotHandle := *vsc.Status.SnapshotHandle
	vscSnapshotClass := *vsc.Spec.VolumeSnapshotClassName
	vsc.Spec = snapshotv1beta1api.VolumeSnapshotContentSpec{
		DeletionPolicy: snapshotv1beta1api.VolumeSnapshotContentRetain,
		Driver:         vsc.Spec.Driver,
		Source: snapshotv1beta1api.VolumeSnapshotContentSource{
			SnapshotHandle: &vscSnapshotHandle,
		},
		VolumeSnapshotClassName: &vscSnapshotClass,
		VolumeSnapshotRef: core_v1.ObjectReference{
			APIVersion: vsc.Spec.VolumeSnapshotRef.APIVersion,
			Kind:       vsc.Spec.VolumeSnapshotRef.Kind,
			Name:       vsc.Spec.VolumeSnapshotRef.Name,
			Namespace:  vsc.Spec.VolumeSnapshotRef.Namespace,
		},
	}

	vsc.Status = nil

	/*toRestore := &snapshotv1beta1api.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: vsc.Name,
		},
		Spec: snapshotv1beta1api.VolumeSnapshotContentSpec{
			DeletionPolicy: snapshotv1beta1api.VolumeSnapshotContentRetain,
			Driver:         vsc.Spec.Driver,
			Source: snapshotv1beta1api.VolumeSnapshotContentSource{
				SnapshotHandle: &vscSnapshotHandle,
			},
			VolumeSnapshotClassName: &vscSnapshotClass,
			VolumeSnapshotRef: core_v1.ObjectReference{
				APIVersion: vsc.Spec.VolumeSnapshotRef.APIVersion,
				Kind:       vsc.Spec.VolumeSnapshotRef.Kind,
				Name:       vsc.Spec.VolumeSnapshotRef.Name,
				Namespace:  vsc.Spec.VolumeSnapshotRef.Namespace,
			},
		},
	}*/

	vscMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&vsc)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	p.log.Info("vsc Spec")
	p.log.Infof("name: %s", vsc.Name)
	p.log.Infof("DeletionPolicy: %s", vsc.Spec.DeletionPolicy)
	p.log.Infof("Driver: %s", vsc.Spec.Driver)
	if vsc.Spec.Source.VolumeHandle != nil {
		p.log.Infof("toRestore Source volume handle: %s", *vsc.Spec.Source.VolumeHandle)
	}
	if vsc.Spec.Source.SnapshotHandle != nil {
		p.log.Infof("vsc Source snapshot handle: %s", *vsc.Spec.Source.SnapshotHandle)
	}
	p.log.Infof("VolumeSnapshotClassName: %s", *vsc.Spec.VolumeSnapshotClassName)
	p.log.Infof("APIVersion: %s, Kind: %s, Name: %s, Namespace: %s",
		vsc.Spec.VolumeSnapshotRef.APIVersion, vsc.Spec.VolumeSnapshotRef.Kind, vsc.Spec.VolumeSnapshotRef.Name,
		vsc.Spec.VolumeSnapshotRef.Namespace)
	if vsc.Status == nil {
		p.log.Info("vsc.Status is nil")
	}

	p.log.Infof("vscMap")
	for k, v := range vscMap {
		p.log.Infof("%s: %v", k, v)
	}

	p.log.Info("Returning from VSCRestorer")

	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem: &unstructured.Unstructured{Object: vscMap},
	}, nil
}
