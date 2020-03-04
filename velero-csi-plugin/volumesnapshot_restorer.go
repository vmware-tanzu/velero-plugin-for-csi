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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
)

// VSRestorer is a restore item action for VolumeSnapshots
type VSRestorer struct {
	log logrus.FieldLogger
}

func (p *VSRestorer) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: []string{"volumesnapshots.snapshot.storage.k8s.io"},
	}, nil
}

func (p *VSRestorer) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.log.Info("Starting VSRestorer")
	var vs snapshotv1beta1api.VolumeSnapshot

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), &vs); err != nil {
		return &velero.RestoreItemActionExecuteOutput{}, err
	}

	p.log.Infof("VSRestorer for %s/%s", vs.Namespace, vs.Name)

	if vs.Status != nil {
		p.log.Infof("BoundVolumeSnapshotContentName is %s", vs.Status.BoundVolumeSnapshotContentName)
	} else {
		p.log.Infof("vs status is nil")
	}

	vscName := *vs.Status.BoundVolumeSnapshotContentName
	vs.Spec = snapshotv1beta1api.VolumeSnapshotSpec{
		Source: snapshotv1beta1api.VolumeSnapshotSource{
			VolumeSnapshotContentName: &vscName,
		},
	}

	vs.Status = nil
	p.log.Infof("vs: %v", vs)

	vsMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&vs)
	if err != nil {
		p.log.Errorf("failed to convert vs into a map, %v", errors.WithStack(err))
		return nil, errors.WithStack(err)
	}

	p.log.Info("Returning from VSRestorer")

	return &velero.RestoreItemActionExecuteOutput{
		UpdatedItem: &unstructured.Unstructured{Object: vsMap},
	}, nil
}
