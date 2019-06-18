# Velero CSI plugins


This repository contains _alpha_ plugins for CSI snapshotting via Velero.

These plugins are considered experimental and should _not_ be relied upon for production use.

Kubernetes 1.14 or newer is required for the CSI snapshot feature.


## Kinds of Plugins Included

- **Backup Item Action**: Creates a VolumeSnapshot resource from a PersistentVolumeClaim
- **Restore Item Actions**:
 * Edits a PersistentVolumeClaim to add a `dataSource` pointing at a VolumeSnapshot
 * Edits a VolumeSnapshot to remove the `source`, so that it's used as an 'import'.
 * Edits a VolumeSnapshotContents to point to the updated UID of a VolumeSnapshot

## Building the plugins

To build the plugins, run

```bash
$ make
```

## Known shortcomings

* Deleting a backup doesn't clean up associated VolumeSnapshot(Contents) objects.
* VolumeSnapshots taken with a backup aren't listed in the `backup describe` command.
* VSLs must be deleted, otherwise you'll get PartiallyFailed status on your backup as the volume snapshot plugins will try to run and fail due to missing configuration for the storage class.
* There is no VolumeSnapshot to VolumeSnapshotContent logic now, so to back up a namespace, you must back up all cluster resources.
* Restic doesn't work for CSI-created volumes currently, as they have an additional directory that we don't expect.
* `restore describe --detail` output needs to account for VolumeSnapshots
* Deleting a VolumeSnapshotContent object _also deletes the underlying volume on the cloud provider_. This needs to be addressed in order for the full restoration process to work as it does with Velero.

## Using with AWS's EBS CSI driver

Deploy the driver (see [their docs](https://github.com/kubernetes-sigs/aws-ebs-csi-driver/) for more details)

```
kubectl apply -f https://raw.githubusercontent.com/kubernetes-sigs/aws-ebs-csi-driver/master/deploy/kubernetes/manifest.yaml
```

Apply the storage class

```
kubectl apply -f sc.yaml
```

Apply the snapshot class

```
kubectl apply -f snapshotclass.yaml
```

Create the demo application

```
kubectl apply -f demo.yaml
```

Either start Velero locally, or edit a deployment, and provide the following flag to update the order in which to restore resources:

```
--restore-resource-priorities namespaces,storageclasses,customresourcedefinitions,volumesnapshotclass.snapshot.storage.k8s.io,volumesnapshots.snapshot.storage.k8s.io,volumesnapshotcontents.snapshot.storage.k8s.io,persistentvolumes,persistentvolumeclaims,secrets,configmaps,serviceaccounts,limitranges,pods,replicaset
```

Create a backup, including cluster resources so that the VolumeSnapshotContents objects are properly backed up (walking from a VolumeSnapshot to the associated Contents will be addressed at a later time):

```
velero backup create --include-cluster-resources=true --include-namespaces demo demo-backup --wait
```

Simulate a disaster:

```
kubectl delete ns/demo
```

Recover

```
velero restore create --from-backup demo-backup
```
