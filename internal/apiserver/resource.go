package apiserver

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
)

func NewResource(obj runtime.Object, meta *metav1.ObjectMeta, list runtime.Object, gv schema.GroupVersionResource) resource.Object {
	return &apiServerObject{Object: obj, objectMeta: meta, listObject: list, gv: gv}
}

type apiServerObject struct {
	runtime.Object
	listObject runtime.Object
	objectMeta *metav1.ObjectMeta
	gv         schema.GroupVersionResource
}

func (o *apiServerObject) GetObjectMeta() *metav1.ObjectMeta {
	return o.objectMeta
}

func (o *apiServerObject) GetGroupVersionResource() schema.GroupVersionResource {
	return o.gv
}

func (o *apiServerObject) IsStorageVersion() bool {
	return true
}

func (o *apiServerObject) NamespaceScoped() bool {
	return true
}

func (o *apiServerObject) New() runtime.Object {
	return o.Object.DeepCopyObject()
}

func (o *apiServerObject) NewList() runtime.Object {
	return o.listObject.DeepCopyObject()
}
