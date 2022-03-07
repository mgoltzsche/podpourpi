package apiserver

import (
	//"github.com/mgoltzsche/podpourpi/internal/storage"
	storageadapter "github.com/mgoltzsche/podpourpi/internal/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	registryrest "k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/storage"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
)

type APIGroupBuilder struct {
	versions       map[string]map[string]registryrest.Storage
	schemeBuilders []func(scheme *runtime.Scheme) error
	err            error
}

func NewAPIGroupBuilder() *APIGroupBuilder {
	return &APIGroupBuilder{versions: map[string]map[string]registryrest.Storage{}}
}

func (b *APIGroupBuilder) WithVersion(version *APIGroupVersion) *APIGroupBuilder {
	b.versions[version.groupVersion.Version] = version.resources
	b.schemeBuilders = append(b.schemeBuilders, version.schemeBuilders...)
	return b
}

func (b *APIGroupBuilder) Build(scheme *runtime.Scheme, codecs runtime.NegotiatedSerializer, parameterCodec runtime.ParameterCodec) (*genericapiserver.APIGroupInfo, error) {
	for _, fn := range b.schemeBuilders {
		err := fn(scheme)
		if err != nil {
			return nil, err
		}
	}
	return &genericapiserver.APIGroupInfo{
		PrioritizedVersions:          scheme.PrioritizedVersionsForGroup(""),
		VersionedResourcesStorageMap: b.versions,
		Scheme:                       scheme,
		ParameterCodec:               parameterCodec,
		NegotiatedSerializer:         codecs,
	}, nil
}

type RESTFactory func(*runtime.Scheme, resource.Object) (registryrest.Storage, error)

type APIGroupVersion struct {
	groupVersion   schema.GroupVersion
	resources      map[string]registryrest.Storage
	schemeBuilders []func(scheme *runtime.Scheme) error
}

func NewAPIGroupVersion(version schema.GroupVersion) *APIGroupVersion {
	return &APIGroupVersion{groupVersion: version, resources: map[string]registryrest.Storage{}}
}

// TODO: map this differently: Flatten WithResource() and design it so that multiple other versions could be converted

func (b *APIGroupVersion) WithResource(resource resource.Object, storefn RESTFactory) *APIGroupVersion {
	obj := resource.New()
	list := resource.NewList()
	b.schemeBuilders = append(b.schemeBuilders, func(scheme *runtime.Scheme) error {
		store, err := storefn(scheme, resource)
		if err != nil {
			return err
		}
		scheme.AddKnownTypes(resource.GetGroupVersionResource().GroupVersion(), obj, list)
		scheme.AddKnownTypes(schema.GroupVersion{
			Group:   resource.GetGroupVersionResource().Group,
			Version: runtime.APIVersionInternal,
		}, obj, list)
		metav1.AddToGroupVersion(scheme, resource.GetGroupVersionResource().GroupVersion())
		b.resources[resource.GetGroupVersionResource().Resource] = store
		return nil
	})
	return b
}

func (b *APIGroupVersion) WithResourceStorage(name string, obj runtime.Object, list runtime.Object, namespaced bool, store storage.Interface) *APIGroupVersion {
	res := NewResource(obj, list, namespaced, b.groupVersion.WithResource(name))
	return b.WithResource(res, func(scheme *runtime.Scheme, obj resource.Object) (registryrest.Storage, error) {
		store, err := storageadapter.NewRESTStorageAdapter(obj, store, scheme)
		if err != nil {
			return nil, err
		}
		return store, nil
	})
}
