# Velero Example Plugins

[![Build Status][1]][2]

This repository contains example plugins for Velero.

## Kinds of Plugins

Velero currently supports the following kinds of plugins:

- **Object Store** - persists and retrieves backups, backup log files, restore warning/error files, restore logs.
- **Block Store** - creates snapshots from volumes (during a backup) and volumes from snapshots (during a restore).
- **Backup Item Action** - performs arbitrary logic on individual items prior to storing them in the backup file.
- **Restore Item Action** - performs arbitrary logic on individual items prior to restoring them in the Kubernetes cluster.

## Building the plugins

To build the plugins, run

```bash
$ make
```

To build the image, run

```bash
$ make container
```

This builds an image tagged as `gcr.io/heptio-images/velero-plugin-example`. If you want to specify a
different name, run

```bash
$ make container IMAGE=your-repo/your-name:here
```

## Deploying the plugins

***Note***: Currently this plugin is intended to work with the currently unreleased 1.0.0 version of velero. If you're running a version of velero that isn't that please clone and build your repo from the following [commit](https://github.com/faiq/velero-plugin-example/tree/499abcc55b729ce5e64cc5ebc6e3376bb51a4136). Alternatively, you can update your velero deployment on kubernetes with the [master tag](https://github.com/heptio/velero/blob/master/docs/image-tagging.md) if you want to use the latest version of this example plugin.


To deploy your plugin image to an Velero server:

1. Make sure your image is pushed to a registry that is accessible to your cluster's nodes.
2. Run `velero plugin add <image>`, e.g. `velero plugin add gcr.io/heptio-images/velero-plugin-example`

## Using the plugins

***Note***: As of v0.10.0, the Custom Resource Definitions used to define backup and block storage providers have changed. See [the previous docs][3] for using plugins with versions v0.6-v0.9.x.

When the plugin is deployed, it is only made available to use. To make the plugin effective, you must modify your configuration:

Backup storage:

1. Run `kubectl edit backupstoragelocation <location-name> -n <velero-namespace>` e.g. `kubectl edit backupstoragelocation default -n velero` OR `velero backup-location create <location-name> --provider <provider-name>`
2. Change the value of `spec.provider` to enable an **Object Store** plugin
3. Save and quit. The plugin will be used for the next `backup/restore`

Volume snapshot storage:

1. Run `kubectl edit volumesnapshotlocation <location-name> -n <velero-namespace>` e.g. `kubectl edit volumesnapshotlocation default -n velero` OR `velero snapshot-location create <location-name> --provider <provider-name>`
2. Change the value of `spec.provider` to enable a **Block Store** plugin
3. Save and quit. The plugin will be used for the next `backup/restore`

## Examples

To run with the example plugins, do the following:

1. Run `velero backup-location create  default --provider file` Optional: `--config bucket:<your-bucket>,prefix:<your-prefix>` to configure a bucket and/or prefix directories.
2. Run `velero snapshot-location create example-default --provider example-blockstore`
3. Run `kubectl edit deployment/velero -n <velero-namespace>`
4. Change the value of `spec.template.spec.args` to look like the following:

```yaml
      - args:
        - server
        - --default-volume-snapshot-locations
        - example-blockstore:example-default
```

5. Run `kubectl create -f examples/with-pv.yaml` to apply a sample nginx application that uses the example block store plugin. ***Note***: This example works best on a virtual machine, as it uses the host's `/tmp` directory for data storage.
6. Save and quit. The plugins will be used for the next `backup/restore`

## Creating your own plugin project

1. Create a new directory in your `$GOPATH`, e.g. `$GOPATH/src/github.com/someuser/velero-plugins`
2. Copy everything from this project into your new project

```bash
$ cp -a $GOPATH/src/github.com/heptio/velero-plugin-example/* $GOPATH/src/github.com/someuser/velero-plugins/.
```

3. Remove the git history

```bash
$ cd $GOPATH/src/github.com/someuser/velero-plugins
$ rm -rf .git
```

4. Adjust the existing plugin directories and source code as needed.

The `Makefile` is configured to automatically build all directories starting with the prefix `velero-`.
You most likely won't need to edit this file, as long as you follow this convention.

If you need to pull in additional dependencies to your vendor directory, just run

```bash
$ dep ensure
```

## Combining multiple plugins in one file

As of v0.10.0, Velero can host multiple plugins inside of a single, resumable process. The plugins can be
of any supported type. See `velero-examples/main.go`


[1]: https://travis-ci.org/heptio/velero-plugin-example.svg?branch=master
[2]: https://travis-ci.org/heptio/velero-plugin-example
[3]: https://github.com/heptio/velero-plugin-example/blob/v0.9.x/README.md#using-the-plugins
