package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
)

// AppSpec defines the desired state of Cache
type AppSpec struct {
}

// AppStatus defines the observed state of Cache
type AppStatus struct {
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// App is the Schema for the apps API
// +k8s:openapi-gen=true
// +resource:path=apps,strategy=AppStrategy,shortname=apps,rest=AppREST
type App struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AppSpec   `json:"spec,omitempty"`
	Status AppStatus `json:"status,omitempty"`
}

var _ resource.Object = &App{}

func (in *App) GetObjectMeta() *metav1.ObjectMeta {
	return &in.ObjectMeta
}

func (in *App) NamespaceScoped() bool {
	return false
}

func (in *App) IsStorageVersion() bool {
	return true
}

func (in *App) New() runtime.Object {
	return &App{}
}

func (in *App) NewList() runtime.Object {
	return &AppList{}
}

func (in *App) GetGroupVersionResource() schema.GroupVersionResource {
	return GroupVersion.WithResource("apps")
}

// AppList contains a list of Cache
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []App `json:"items"`
}

var _ resource.ObjectList = &AppList{}

func (in *AppList) GetListMeta() *metav1.ListMeta {
	return &in.ListMeta
}
