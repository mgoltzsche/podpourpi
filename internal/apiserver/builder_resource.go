package apiserver

import (
	"fmt"
	"strings"

	storageadapter "github.com/mgoltzsche/podpourpi/internal/storage"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericregistry "k8s.io/apiserver/pkg/registry/generic"
	registryrest "k8s.io/apiserver/pkg/registry/rest"

	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/storage"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource/resourcerest"
	//"sigs.k8s.io/apiserver-runtime/pkg/builder/rest"
)

func (b *Builder) WithCoreAPI() *Builder {
	/*obj := &corev1.ConfigMap{}
	list := &corev1.ConfigMapList{}
	resource := NewResource(obj, list, true, corev1.SchemeGroupVersion.WithResource("configmaps"))
	//b.scheme.SetVersionPriority(corev1.SchemeGroupVersion)
	// See https://github.com/kubernetes-sigs/apiserver-runtime/blob/v1.0.2/pkg/builder/resource/register.go#L20
	b.scheme.AddKnownTypes(resource.GetGroupVersionResource().GroupVersion(), obj, list)
	b.scheme.AddKnownTypes(schema.GroupVersion{
		Group:   resource.GetGroupVersionResource().Group,
		Version: runtime.APIVersionInternal,
	}, obj, list)
	metav1.AddToGroupVersion(b.scheme, resource.GetGroupVersionResource().GroupVersion())
	store, err := storageadapter.NewRESTStorageAdapter(resource, NewInMemoryStore(ObjectKeyFromGroupAndName), b.scheme)
	if err != nil {
		panic(err)
	}
	b.serverConfigs = append(b.serverConfigs, func(srv *genericapiserver.GenericAPIServer) error {
		// apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(group, b.scheme, b.parameterCodec, codecs)
		apiGroupInfo := genericapiserver.APIGroupInfo{
			PrioritizedVersions:          b.scheme.PrioritizedVersionsForGroup(""),
			VersionedResourcesStorageMap: map[string]map[string]registryrest.Storage{},
			Scheme:                       b.scheme,
			ParameterCodec:               b.parameterCodec,
			NegotiatedSerializer:         srv.Serializer,
		}
		apiGroupInfo.VersionedResourcesStorageMap["v1"] = map[string]registryrest.Storage{
			resource.GetGroupVersionResource().Resource: store,
		}
		return srv.InstallLegacyAPIGroup(genericapiserver.DefaultLegacyAPIPrefix, &apiGroupInfo)
	})*/
	store := NewInMemoryStore()
	apiGroupBuilder := NewAPIGroupBuilder().
		WithVersion(NewAPIGroupVersion(corev1.SchemeGroupVersion).
			WithResourceStorage("configmaps", &corev1.ConfigMap{}, &corev1.ConfigMapList{}, true, store).
			WithResourceStorage("secrets", &corev1.Secret{}, &corev1.SecretList{}, true, store).
			WithResourceStorage("pods", &corev1.Pod{}, &corev1.PodList{}, true, store),
		)
	b.serverConfigs = append(b.serverConfigs, func(srv *genericapiserver.GenericAPIServer) error {
		apiGroup, err := apiGroupBuilder.Build(b.scheme, srv.Serializer, b.parameterCodec)
		if err != nil {
			return err
		}
		return srv.InstallLegacyAPIGroup(genericapiserver.DefaultLegacyAPIPrefix, apiGroup)
	})
	return b
}

func (b *Builder) WithResourceStorage(obj resource.Object, store storage.Interface) *Builder {
	b.WithResource(obj, storageadapter.NewRESTStorageProvider(obj, store))
	return b
}

func (b *Builder) WithResource(obj resource.Object, storageProvider StorageProvider) *Builder {
	gvr := obj.GetGroupVersionResource()
	err := resource.AddToScheme(obj)(b.scheme)
	if err != nil {
		b.err = err
		return b
	}
	// add WatchEvent kind to each apigroup version once
	if _, ok := b.scheme.AllKnownTypes()[gvr.GroupVersion().WithKind("WatchEvent")]; !ok {
		b.scheme.AddKnownTypes(gvr.GroupVersion(), &metav1.WatchEvent{})
	}
	// reuse the storage if this resource has already been registered
	if s, found := b.storageProvider[gvr.GroupResource()]; found {
		return b.forGroupVersionResource(gvr, s.Get)
	}
	utilruntime.Must(b.scheme.SetVersionPriority(gvr.GroupVersion()))

	// TODO: fix build (compatibility) error or avoid
	// If the type implements it's own storage, then use that
	switch s := obj.(type) {
	case resourcerest.Creator:
		return b.forGroupVersionResource(gvr, StaticHandlerProvider{Storage: s.(registryrest.Storage)}.Get)
	case resourcerest.Updater:
		return b.forGroupVersionResource(gvr, StaticHandlerProvider{Storage: s.(registryrest.Storage)}.Get)
	case resourcerest.Getter:
		return b.forGroupVersionResource(gvr, StaticHandlerProvider{Storage: s.(registryrest.Storage)}.Get)
	case resourcerest.Lister:
		return b.forGroupVersionResource(gvr, StaticHandlerProvider{Storage: s.(registryrest.Storage)}.Get)
	}

	//storageProvider := filepath.NewJSONFilepathStorageProvider(obj, "data")
	_ = b.forGroupVersionResource(gvr, storageProvider)

	// automatically create status subresource if the object implements the status interface
	b.withSubResourceIfExists(obj, storageProvider)
	return b
}

// forGroupVersionResource manually registers storage for a specific resource.
func (b *Builder) forGroupVersionResource(
	gvr schema.GroupVersionResource, sp StorageProvider) *Builder {
	// register the group version
	b.withGroupVersions(gvr.GroupVersion())

	// TODO: make sure folks don't register multiple storageProvider instance for the same group-resource
	// don't replace the existing instance otherwise it will chain wrapped singletonProviders when
	// fetching from the map before calling this function
	if _, found := b.storageProvider[gvr.GroupResource()]; !found {
		b.storageProvider[gvr.GroupResource()] = &singletonProvider{Provider: sp}
	}
	// add the API with its storageProvider
	b.apis[gvr] = sp
	return b
}

func (b *Builder) withGroupVersions(versions ...schema.GroupVersion) *Builder {
	if b.groupVersions == nil {
		b.groupVersions = map[schema.GroupVersion]bool{}
	}
	for _, gv := range versions {
		if _, found := b.groupVersions[gv]; found {
			continue
		}
		b.groupVersions[gv] = true
		b.orderedGroupVersions = append(b.orderedGroupVersions, gv)
	}
	return b
}

func (b *Builder) withSubResourceIfExists(obj resource.Object, parentStorageProvider StorageProvider) {
	parentGVR := obj.GetGroupVersionResource()
	// automatically create status subresource if the object implements the status interface
	if _, ok := obj.(resource.ObjectWithStatusSubResource); ok {
		statusGVR := parentGVR.GroupVersion().WithResource(parentGVR.Resource + "/status")
		b.forGroupVersionSubResource(statusGVR, parentStorageProvider, nil)
	}
	if _, ok := obj.(resource.ObjectWithScaleSubResource); ok {
		subResourceGVR := parentGVR.GroupVersion().WithResource(parentGVR.Resource + "/scale")
		b.forGroupVersionSubResource(subResourceGVR, parentStorageProvider, nil)
	}
	if sgs, ok := obj.(resource.ObjectWithArbitrarySubResource); ok {
		for _, sub := range sgs.GetArbitrarySubResources() {
			sub := sub
			subResourceGVR := parentGVR.GroupVersion().WithResource(parentGVR.Resource + "/" + sub.SubResourceName())
			b.forGroupVersionSubResource(subResourceGVR, parentStorageProvider, StaticHandlerProvider{Storage: sub}.Get)
		}
	}
}

// forGroupVersionSubResource manually registers storageProvider for a specific subresource.
func (b *Builder) forGroupVersionSubResource(
	gvr schema.GroupVersionResource, parentProvider StorageProvider, subResourceProvider StorageProvider) {
	isSubResource := strings.Contains(gvr.Resource, "/")
	if !isSubResource {
		fmt.Errorf("Expected status subresource but received %v/%v/%v", gvr.Group, gvr.Version, gvr.Resource)
	}

	// add the API with its storageProvider for subresource
	b.apis[gvr] = (&subResourceStorageProvider{
		subResourceGVR:             gvr,
		parentStorageProvider:      parentProvider,
		subResourceStorageProvider: subResourceProvider,
	}).Get
}

// StaticHandlerProvider returns itself as the request handler.
type StaticHandlerProvider struct { // TODO: privatize
	registryrest.Storage
}

// Get returns itself as the handler
func (p StaticHandlerProvider) Get(s *runtime.Scheme, g genericregistry.RESTOptionsGetter) (registryrest.Storage, error) {
	return p.Storage, nil
}
