module github.com/vmware-tanzu/velero-plugin-for-csi

go 1.13

require (
	github.com/hashicorp/go-plugin v1.0.1 // indirect
	github.com/kubernetes-csi/external-snapshotter/v2 v2.0.1
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5 // indirect
	github.com/vmware-tanzu/velero v1.2.0
	k8s.io/api v0.17.2
	k8s.io/apimachinery v0.17.2
	k8s.io/client-go v0.17.0
)
