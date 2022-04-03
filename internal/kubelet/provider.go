package kubelet

import (
	"context"
	"net/http"

	"github.com/virtual-kubelet/virtual-kubelet/node"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
)

// TODO: implement using https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/cri/remote/remote_runtime.go

// Provider contains the methods required to implement a virtual-kubelet provider.
//
// Errors produced by these methods should implement an interface from
// github.com/virtual-kubelet/virtual-kubelet/errdefs package in order for the
// core logic to be able to understand the type of failure.
type Provider interface {
	node.PodLifecycleHandler

	ResourceUpdater

	// GetContainerLogsHandler handles a Pod's container log retrieval.
	GetContainerLogsHandler(w http.ResponseWriter, r *http.Request)

	// RunInContainer executes a command in a container in the pod, copying data
	// between in/out/err and the container's stdin/stdout/stderr.
	RunInContainer(ctx context.Context, namespace, podName, containerName string, cmd []string, attach api.AttachIO) error
}
