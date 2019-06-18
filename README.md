# Velero CSI plugins


This repository contains _alpha_ plugins for CSI snapshotting via Velero.

These plugins are considered experimental and should _not_ be relied upon for production use.

Kubernetes 1.14 or newer is required for the CSI snapshot feature.


## Kinds of Plugins

- **Backup Item Action** - Creates a VolumeSnapshot resource from a PersistentVolumeClaim
- **Restore Item Action** - Edits a PersistentVolumeClaim to add a `dataSource` pointing at a VolumeSnapshot

## Building the plugins

To build the plugins, run

```bash
$ make
```

To build the image, run

```bash
$ make container
```


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
