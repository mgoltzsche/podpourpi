package apiserver

import (
	"context"
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
	store := NewInMemoryStore()
	apiGroupBuilder := NewAPIGroupBuilder().
		// TODO: flatten resource builder so that builder clients can pick resource by resource
		WithVersion(NewAPIGroupVersion(corev1.SchemeGroupVersion).
			WithResourceStorage("nodes", &corev1.Node{}, &corev1.NodeList{}, false, store).
			WithResourceStorage("namespaces", &corev1.Namespace{}, &corev1.NamespaceList{}, false, store).
			WithResourceStorage("configmaps", &corev1.ConfigMap{}, &corev1.ConfigMapList{}, true, store).
			WithResourceStorage("secrets", &corev1.Secret{}, &corev1.SecretList{}, true, store).
			WithResourceStorage("pods", &corev1.Pod{}, &corev1.PodList{}, true, store).
			// kube-aggregator requires Service and Endpoint
			WithResourceStorage("services", &corev1.Service{}, &corev1.ServiceList{}, true, store).
			WithResourceStorage("endpoints", &corev1.Endpoints{}, &corev1.EndpointsList{}, true, store),
		)
	b.serverConfigs = append(b.serverConfigs, func(srv *genericapiserver.GenericAPIServer) error {
		ctx := context.Background()
		defaultNamespace := &corev1.Namespace{}
		defaultNamespace.Name = "default"
		err := store.Create(ctx, storageadapter.ObjectKey(corev1.SchemeGroupVersion.WithResource("namespaces").GroupResource(), "", defaultNamespace.Name), defaultNamespace, defaultNamespace, 0)
		if err != nil {
			return fmt.Errorf("create default namespace: %w", err)
		}
		apiGroup, err := apiGroupBuilder.Build(b.scheme, srv.Serializer, b.parameterCodec)
		if err != nil {
			return err
		}
		err = srv.InstallLegacyAPIGroup(genericapiserver.DefaultLegacyAPIPrefix, apiGroup)
		if err != nil {
			return err
		}
		// TODO: Add Namespace cleanup controller?! - https://github.com/kubernetes/kubernetes/blob/v1.23.1/pkg/controller/namespace/namespace_controller.go#L66
		return nil
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
