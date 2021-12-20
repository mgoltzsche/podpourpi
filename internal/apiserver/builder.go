package apiserver

import (
	"net/url"

	"github.com/mgoltzsche/podpourpi/internal/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	genericregistry "k8s.io/apiserver/pkg/registry/generic"
	registryrest "k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"

	//genericoptions "k8s.io/apiserver/pkg/server/options"
	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource/resourcestrategy"
)

type StorageProvider = storage.StorageProvider

type Builder struct {
	err                  error
	apis                 map[schema.GroupVersionResource]StorageProvider
	scheme               *runtime.Scheme
	parameterScheme      *runtime.Scheme
	parameterCodec       runtime.ParameterCodec
	storageProvider      map[schema.GroupResource]*singletonProvider
	groupVersions        map[schema.GroupVersion]bool
	orderedGroupVersions []schema.GroupVersion
}

func New() *Builder {
	scheme := runtime.NewScheme()
	metav1.AddToGroupVersion(scheme, schema.GroupVersion{Version: "v1"})
	unversioned := schema.GroupVersion{Group: "", Version: "v1"}
	scheme.AddUnversionedTypes(unversioned,
		&metav1.Status{},
		&metav1.APIVersions{},
		&metav1.APIGroupList{},
		&metav1.APIGroup{},
		&metav1.APIResourceList{},
	)
	paramScheme := runtime.NewScheme()
	return &Builder{
		apis:            map[schema.GroupVersionResource]StorageProvider{},
		scheme:          scheme,
		parameterScheme: paramScheme,
		parameterCodec:  runtime.NewParameterCodec(paramScheme),
		storageProvider: map[schema.GroupResource]*singletonProvider{},
		groupVersions:   map[schema.GroupVersion]bool{},
	}
}

func (b *Builder) Build() (*genericapiserver.GenericAPIServer, error) {
	if b.err != nil {
		return nil, b.err
	}
	codecs := serializer.NewCodecFactory(b.scheme)
	serverConfig := genericapiserver.NewRecommendedConfig(codecs)
	serverConfig.ExternalAddress = "127.0.0.1:8080"
	serverConfig.LoopbackClientConfig = &restclient.Config{}
	completeConfig := serverConfig.Complete()
	genericServer, err := completeConfig.New("podpourpi-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}
	apiGroups, err := b.buildAPIGroupInfos(completeConfig.RESTOptionsGetter, codecs)
	if err != nil {
		return nil, err
	}
	for _, apiGroup := range apiGroups {
		if err := genericServer.InstallAPIGroup(apiGroup); err != nil {
			return nil, err
		}
	}
	genericServer.AddPostStartHookOrDie("start-podpourpi-informers", func(context genericapiserver.PostStartHookContext) error {
		if completeConfig.SharedInformerFactory != nil {
			completeConfig.SharedInformerFactory.Start(context.StopCh)
		}
		return nil
	})
	return genericServer, nil
}

func (b *Builder) buildAPIGroupInfos(g genericregistry.RESTOptionsGetter, codecs serializer.CodecFactory) ([]*genericapiserver.APIGroupInfo, error) {
	resourcesByGroupVersion := make(map[schema.GroupVersion]sets.String)
	groups := sets.NewString()
	for gvr := range b.apis {
		groups.Insert(gvr.Group)
		if resourcesByGroupVersion[gvr.GroupVersion()] == nil {
			resourcesByGroupVersion[gvr.GroupVersion()] = sets.NewString()
		}
		resourcesByGroupVersion[gvr.GroupVersion()].Insert(gvr.Resource)
	}
	apiGroups := []*genericapiserver.APIGroupInfo{}
	for _, group := range groups.List() {
		apis := map[string]map[string]registryrest.Storage{}
		for gvr, storageProviderFunc := range b.apis {
			if gvr.Group == group {
				if _, found := apis[gvr.Version]; !found {
					apis[gvr.Version] = map[string]registryrest.Storage{}
				}
				storage, err := storageProviderFunc(b.scheme, g)
				if err != nil {
					return nil, err
				}
				apis[gvr.Version][gvr.Resource] = storage
				if _, ok := storage.(resourcestrategy.Defaulter); ok {
					if obj, ok := storage.(runtime.Object); ok {
						b.scheme.AddTypeDefaultingFunc(obj, func(obj interface{}) {
							obj.(resourcestrategy.Defaulter).Default()
						})
					}
				}
				if c, ok := storage.(registryrest.Connecter); ok {
					optionsObj, _, _ := c.NewConnectOptions()
					if optionsObj != nil {
						b.parameterScheme.AddKnownTypes(gvr.GroupVersion(), optionsObj)
						b.scheme.AddKnownTypes(gvr.GroupVersion(), optionsObj)
						if _, ok := optionsObj.(resource.QueryParameterObject); ok {
							if err := b.parameterScheme.AddConversionFunc(&url.Values{}, optionsObj, func(src interface{}, dest interface{}, s conversion.Scope) error {
								return dest.(resource.QueryParameterObject).ConvertFromUrlValues(src.(*url.Values))
							}); err != nil {
								return nil, err
							}
						}
					}
				}
			}
		}
		apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(group, b.scheme, b.parameterCodec, codecs)
		apiGroupInfo.VersionedResourcesStorageMap = apis
		apiGroups = append(apiGroups, &apiGroupInfo)
	}
	return apiGroups, nil
}

func (b *Builder) orderedGroupVersionsFromScheme() []schema.GroupVersion {
	var (
		orderedGroupVersions []schema.GroupVersion
		schemes              []*runtime.Scheme
		schemeBuilder        runtime.SchemeBuilder
	)
	schemes = append(schemes, b.scheme)
	schemeBuilder.Register(
		func(scheme *runtime.Scheme) error {
			groupVersions := make(map[string]sets.String)
			for gvr := range b.apis {
				if groupVersions[gvr.Group] == nil {
					groupVersions[gvr.Group] = sets.NewString()
				}
				groupVersions[gvr.Group].Insert(gvr.Version)
			}
			for g, versions := range groupVersions {
				gvs := []schema.GroupVersion{}
				for _, v := range versions.List() {
					gvs = append(gvs, schema.GroupVersion{
						Group:   g,
						Version: v,
					})
				}
				err := scheme.SetVersionPriority(gvs...)
				if err != nil {
					return err
				}
			}
			for i := range orderedGroupVersions {
				metav1.AddToGroupVersion(scheme, orderedGroupVersions[i])
			}
			return nil
		},
	)
	for i := range schemes {
		if err := schemeBuilder.AddToScheme(schemes[i]); err != nil {
			panic(err)
		}
	}
	return orderedGroupVersions
}
