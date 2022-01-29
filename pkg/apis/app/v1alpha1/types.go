package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
)

// AppState specifies the state of an application or its container.
// +enum
type AppState string

const (
	AppStateUnknown  AppState = "unknown"
	AppStateStarting AppState = "starting"
	AppStateRunning  AppState = "running"
	AppStateError    AppState = "error"
	AppStateExited   AppState = "exited"
)

// +k8s:openapi-gen=true
// AppSpec defines the desired state of Cache
type AppSpec struct {
	Enabled bool `json:"enabled"`
}

// +k8s:openapi-gen=true
// AppStatus defines the observed state of Cache
type AppStatus struct {
	ActiveProfile string      `json:"activeProfile,omitempty"`
	State         AppState    `json:"state,omitempty"`
	Containers    []Container `json:"containers,omitempty"`
}

// +k8s:openapi-gen=true
// Container defines an app container.
type Container struct {
	ID     string          `json:"id"`
	Name   string          `json:"name,omitempty"`
	Status ContainerStatus `json:"status"`
}

// +k8s:openapi-gen=true
// ContainerStatus defines a container status.
type ContainerStatus struct {
	State   AppState `json:"state"`
	Message string   `json:"message,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// App is the Schema for the apps API
// +k8s:openapi-gen=true
// +resource:path=apps,strategy=AppStrategy,shortname=apps,rest=AppREST
type App struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   AppSpec   `json:"spec"`
	Status AppStatus `json:"status"`
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
// +k8s:openapi-gen=true
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

func EachApp(l runtime.Object, fn func(runtime.Object) error) error {
	for _, item := range l.(*AppList).Items {
		o := item
		err := fn(&o)
		if err != nil {
			return err
		}
	}
	return nil
}
