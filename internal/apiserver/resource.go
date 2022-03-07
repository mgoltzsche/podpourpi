package apiserver

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
)

func NewResource(obj runtime.Object, list runtime.Object, namespaceScoped bool, gv schema.GroupVersionResource) resource.Object {
	return &Resource{Object: obj, objectMeta: &metav1.ObjectMeta{}, listObject: list, gv: gv, namespaceScoped: namespaceScoped}
}

type Resource struct {
	runtime.Object
	listObject      runtime.Object
	objectMeta      *metav1.ObjectMeta
	gv              schema.GroupVersionResource
	namespaceScoped bool
}

func (o *Resource) GetObjectMeta() *metav1.ObjectMeta {
	return o.objectMeta
}

func (o *Resource) GetGroupVersionResource() schema.GroupVersionResource {
	return o.gv
}

func (o *Resource) IsStorageVersion() bool {
	return true
}

func (o *Resource) NamespaceScoped() bool {
	return o.namespaceScoped
}

func (o *Resource) New() runtime.Object {
	return o.Object.DeepCopyObject()
}

func (o *Resource) NewList() runtime.Object {
	return o.listObject.DeepCopyObject()
}
