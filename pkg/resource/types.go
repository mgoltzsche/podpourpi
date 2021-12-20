package resource

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/registry/rest"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"
)

// Object must be implemented by all resources served by the apiserver.
type Object interface {
	// Object allows the apiserver libraries to operate on the Object
	runtime.Object

	// GetObjectMeta returns the object meta reference.
	GetObjectMeta() *metav1.ObjectMeta

	// Scoper is used to qualify the resource as either namespace scoped or non-namespace scoped.
	rest.Scoper

	// New returns a new instance of the resource -- e.g. &v1.Deployment{}
	New() runtime.Object

	// NewList return a new list instance of the resource -- e.g. &v1.DeploymentList{}
	NewList() runtime.Object

	// GetGroupVersionResource returns the GroupVersionResource for this resource.  The resource should
	// be the all lowercase and pluralized kind.s
	GetGroupVersionResource() schema.GroupVersionResource

	// IsStorageVersion returns true if the object is also the internal version -- i.e. is the type defined
	// for the API group an alias to this object.
	// If false, the resource is expected to implement MultiVersionObject interface.
	IsStorageVersion() bool
}

// ObjectList must be implemented by all resources' list object.
type ObjectList interface {
	// Object allows the apiserver libraries to operate on the Object
	runtime.Object

	// GetListMeta returns the list meta reference.
	GetListMeta() *metav1.ListMeta
}
