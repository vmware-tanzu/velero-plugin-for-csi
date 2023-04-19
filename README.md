[![Build Status][101]][102]

# Velero CSI plugins


This repository contains Velero plugins for snapshotting CSI backed PVCs using the [CSI _beta_ snapshot APIs][7].

CSI plugin reaches GA for AWS and Azure environments as of the [Velero 1.9 release][1]. It should also work for those environments in which CSI plugin provides durable snapshot (please check [WARNING](#WARNING) section for more details), but Velero team doesn't have enough resources to verify, so please do enough tests before using CSI plugin in some environments. For those environments that don't have durable snapshot capability, CSI plugin is still in beta state. After Velero's data mover function is ready (hopefully in Velero v1.10), CSI plugin will GA for those environments too.

For a list of prerequisites and installation instructions, please refer to our documentation [here][2].

# WARNING
CSI Snapshots are a standard Kubernetes mechanism for taking snapshots.  The actual implementation of snapshots varies by storage vendor.  For disaster recovery, snapshots must be stored in a durable store, such as an S3 bucket, tape library, etc. and not just on the primary storage.  If the snapshot is only stored on the primary storage and the storage is corrupted or destroyed the backup will be lost.

CSI snapshots on AWS EBS, Azure managed disks and Google Cloud Persistent Disk are durable and can be safely used for backup.

For all other storage systems, please check with your storage vendor.  If your storage vendor doesn't support 
durable snapshot storage you may want to consider 
[Velero's Restic Integration](https://velero.io/docs/latest/restic)

## Verified environment
Velero CSI plugin v0.4 is tested with Velero v1.10 on the following environments:
* EKS: ebs.csi.aws.com driver is tested with helm chart version helm-chart-aws-ebs-csi-driver-2.6.8 and external-snapshotter version is v5.0.1. v6.0.1 external-snapshotter is also tested with EBS CSI driver helm chart version helm-chart-aws-ebs-csi-driver-2.6.10.
* AKS: disk.csi.azure.com driver is tested. CSI related controllers are installed in control plane, so cannot find the exactly version. Tested AKS versions are 1.23.3 and v1.22.6.
* GKE: pd.csi.storage.gke.io driver is tested. CSI related controllers are installed in control plane, so cannot find the exactly version. Tested GKE versions are 1.23.8-gke.1900 and 1.25.1-gke.500.

## Compatibility

Below is a listing of plugin versions and respective Velero versions that are compatible.

| Plugin Version  | Velero Version |
|-----------------|----------------|
| v0.5.x          | v1.11.x        |
| v0.4.x          | v1.10.x        |
| v0.3.0          | v1.9.x         |
| v0.2.0          | v1.7.x, v1.8.x |

## Filing issues

If you would like to file a GitHub issue for the plugin, please open the issue on the [core Velero repo][103]

## Kinds of Plugins Included

### PVCBackupItemAction

A plugin of type BackupItemAction that backs up `PersistentVolumeClaims` which are backed by CSI volumes.

This plugin will create a [CSI VolumeSnapshot][3] which in turn triggers the CSI driver to perform the snapshot operation on the volume.

### VolumeSnapshotBackupItemAction

A plugin of type BackupItemAction that backs up [`volumesnapshots.snapshot.storage.k8s.io`][3].

When invoked, this plugin will capture information about the underlying [`volumesnapshotcontent.snapshot.storage.k8s.io`][4] in the annotations of the volumesnapshots being backed up. This plugin will also return the underlying [`volumesnapshotcontent.snapshot.storage.k8s.io`][4] and the associated [`snapshot.storage.k8s.io.volumesnapshotclasses`][5] as additional resources to be backed up.

### VolumeSnapshotContentBackupItemAction

A plugin of type BackupItemAction that backs up [`volumesnapshotcontent.snapshot.storage.k8s.io`][4]. 

This plugin will look for snapshot delete operation secrets from the [annotations][6] on the VolumeSnapshotContent object being backed up.

### VolumeSnapshotClassBackupItemAction

A plugin of type BackupItemAction that backs up [`snapshot.storage.k8s.io.volumesnapshotclasses`][5].

This plugin will look for snapshot list operation secret from the [annotations][6] on the VolumeSnapshotClass object being backed up.

### PVCRestoreItemAction

A plugin of type RestoreItemAction that restores `PersistentVolumeClaims` which were backed up by [PVCBackupItemAction](#PVCBackupItemAction).

This plugin will modify the spec of the `PersistentVolumeClaim` being restored to use the VolumeSnapshot, created during backup, as the data source ensuring that the newly provisioned volume, to satisfy this claim, may be pre-populated using the VolumeSnapshot.

### VolumeSnapshotRestoreItemAction

A plugin of type RestoreItemAction that restores [`volumesnapshots.snapshot.storage.k8s.io`][3]. 

This plugin will use the annotations, added during backup, to create a [`volumesnapshotcontent.snapshot.storage.k8s.io`][4] and statically bind it to the VolumeSnapshot object being restored. The plugin will also set the necessary [annotations][6] if the original VolumeSnapshotContent had snapshot deletion secrets associated with it. 

### VolumeSnapshotClassRestoreItemAction

A plugin of type RestoreItemAction that restores [`snapshot.storage.k8s.io.volumesnapshotclasses`][5]. 

This plugin will use the [annotations][6] on the object being restored to return, as additional items, any snapshot lister secret that is associated with the VolumeSnapshotClass.


## Building the plugins

Official images of the plugin are available on [Velero DockerHub](https://hub.docker.com/repository/docker/velero/velero-plugin-for-csi).

For development and testing, the plugin images may be built by running the below command

```bash
$ IMAGE=<YOUR_REGISTRY>/velero-plugin-for-csi:<YOUR_TAG> make container
```

## Known shortcomings

We are tracking known limitations with the plugins [here][2]

[1]: https://github.com/vmware-tanzu/velero/releases
[2]: https://velero.io/docs/csi
[3]: https://kubernetes.io/docs/concepts/storage/volume-snapshots/#volumesnapshots
[4]: https://kubernetes.io/docs/concepts/storage/volume-snapshots/#volume-snapshot-contents
[5]: https://kubernetes.io/docs/concepts/storage/volume-snapshot-classes/
[6]: https://github.com/kubernetes-csi/external-snapshotter/blob/master/pkg/utils/util.go#L59-L60
[7]: https://kubernetes.io/blog/2019/12/09/kubernetes-1-17-feature-cis-volume-snapshot-beta/

[101]: https://github.com/vmware-tanzu/velero-plugin-for-csi/workflows/Main%20CI/badge.svg
[102]: https://github.com/vmware-tanzu/velero-plugin-for-csi/actions?query=workflow%3A"Main+CI"
[103]: https://github.com/vmware-tanzu/velero/issues/new/choose
