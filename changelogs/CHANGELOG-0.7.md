## v0.7.0
### All Changes
* Use backup.Spec.CSISnapshotTimeout for DataDownload OperationTimeout (#204, @blackpiglet)
* Bump golang and net version. (#209, @blackpiglet)
* Fix panic when csi timeout duration is short. (#215, @sbahar619)
* Remove velero generated client, because it's going to be deprecated. (#214, @blackpiglet)
* Add VolumeSnapshotClass info in the DataUpload. (#216, @blackpiglet)
* Add uploader config.  (#217, @qiuming-best)
