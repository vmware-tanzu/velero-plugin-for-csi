# Velero CSI Integration

Velero is seeking to support CSI volume snapshotting via our Backup and Restore Item Action plugins. The design is roughly:

* On backup, a PVC is examined for the StorageClass. If there is a SnapshotClass for the associated StorageClass, create a VolumeSnapshot for the PVC.
* On restore, look up the VolumeSnapshot and use it as a `dataSource` when restoring the PVC.

Some things to consider:

* A VolumeSnapshot is a namespaced resource; what happens if it doesn't exist?
* VolumeSnapshots need to be written to the backup tarball.
* When restoring into a new cluster in the same region/availability zone, the VolumeSnapshots should be restored early on. Likely need to add them to the `defaultRestorePriorities`.
