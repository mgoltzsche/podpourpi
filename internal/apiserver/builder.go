package apiserver

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"time"

	storageprovider "github.com/mgoltzsche/podpourpi/internal/storage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/features"
	genericregistry "k8s.io/apiserver/pkg/registry/generic"
	registryrest "k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/util/feature"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsapiserver "k8s.io/apiextensions-apiserver/pkg/apiserver"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apiserver/pkg/server/filters"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/apiserver/pkg/storage/storagebackend/factory"
	"k8s.io/client-go/tools/cache"

	"k8s.io/apiserver/pkg/util/openapi"
	//kubernetesgeneratedopenapi "k8s.io/kubernetes/pkg/generated/openapi"
	generatedopenapi "github.com/mgoltzsche/podpourpi/pkg/generated/openapi"
	openapinamer "k8s.io/apiserver/pkg/endpoints/openapi"

	//apiextensionsoptions "k8s.io/apiextensions-apiserver/pkg/cmd/server/options"
	storageadapter "github.com/mgoltzsche/podpourpi/internal/storage"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	genericapifilters "k8s.io/apiserver/pkg/endpoints/filters"
	serverstorage "k8s.io/apiserver/pkg/server/storage"
	"k8s.io/apiserver/pkg/util/notfoundhandler"
	"k8s.io/apiserver/pkg/util/webhook"
	clientgoinformers "k8s.io/client-go/informers"
	clientgoclientset "k8s.io/client-go/kubernetes"
	clientcmd "k8s.io/client-go/tools/clientcmd"
	clientapi "k8s.io/client-go/tools/clientcmd/api"
	aggregatorapiserver "k8s.io/kube-aggregator/pkg/apiserver"

	//genericoptions "k8s.io/apiserver/pkg/server/options"
	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource/resourcestrategy"
)

func init() {
	utilruntime.Must(feature.DefaultMutableFeatureGate.SetFromMap(map[string]bool{
		string(features.OpenAPIEnums): true,
	}))
}

type StorageProvider = storageprovider.StorageProvider

type Builder struct {
	err                  error
	apis                 map[schema.GroupVersionResource]StorageProvider
	scheme               *runtime.Scheme
	parameterScheme      *runtime.Scheme
	parameterCodec       runtime.ParameterCodec
	storageProvider      map[schema.GroupResource]*singletonProvider
	groupVersions        map[schema.GroupVersion]bool
	orderedGroupVersions []schema.GroupVersion
	extensionAPIEnabled  bool
	kubeconfigDestFile   string
	webDir               string
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

func (b *Builder) WithExtensionsAPI() *Builder {
	b.extensionAPIEnabled = true
	return b
}

func (b *Builder) WithWebUI(dir string) *Builder {
	b.webDir = dir
	return b
}

func (b *Builder) GenerateKubeconfig(file string) *Builder {
	b.kubeconfigDestFile = file
	return b
}

func (b *Builder) Build() (*genericapiserver.GenericAPIServer, error) {
	if b.err != nil {
		return nil, b.err
	}

	// For reference also see https://github.com/kubernetes/kubernetes/blob/v1.23.1/cmd/kube-apiserver/app/server.go#L202

	if b.extensionAPIEnabled {
		crd := &extensionsv1.CustomResourceDefinition{}
		crdRes := NewResource(crd, &crd.ObjectMeta, &extensionsv1.CustomResourceDefinitionList{}, extensionsv1.SchemeGroupVersion.WithResource("customresourcedefinitions"))
		store := NewInMemoryStore(&extensionsv1.CustomResourceDefinitionList{}, func(o runtime.Object, fn func(runtime.Object) error) error {
			for _, item := range o.(*extensionsv1.CustomResourceDefinitionList).Items {
				o := item
				err := fn(&o)
				if err != nil {
					return err
				}
			}
			return nil
		})
		b.WithResource(crdRes, storageadapter.NewRESTStorageProvider("/crdprefix", crdRes, store))
	}

	codecs := serializer.NewCodecFactory(b.scheme)
	serverConfig := genericapiserver.NewRecommendedConfig(codecs)
	serverConfig.ExternalAddress = "127.0.0.1:8080"
	serverConfig.LoopbackClientConfig = &restclient.Config{
		Host: serverConfig.ExternalAddress,
	}
	getOpenAPIDefinitions := openapi.GetOpenAPIDefinitionsWithoutDisabledFeatures(generatedopenapi.GetOpenAPIDefinitions)
	serverConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(getOpenAPIDefinitions, openapinamer.NewDefinitionNamer(b.scheme))
	serverConfig.OpenAPIConfig.Info.Title = "podpourpi"
	serverConfig.LongRunningFunc = filters.BasicLongRunningRequestCheck(
		sets.NewString("watch", "proxy"),
		sets.NewString("attach", "exec", "proxy", "log", "portforward"),
	)
	kubeClientConfig := serverConfig.LoopbackClientConfig
	clientgoExternalClient, err := clientgoclientset.NewForConfig(kubeClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create real external clientset: %w", err)
	}
	serverConfig.SharedInformerFactory = clientgoinformers.NewSharedInformerFactory(clientgoExternalClient, 10*time.Minute)
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
			return nil, fmt.Errorf("install apigroup: %w", err)
		}
	}
	genericServer.AddPostStartHookOrDie("start-apiserver-informers", func(hookCtx genericapiserver.PostStartHookContext) error {
		completeConfig.SharedInformerFactory.Start(hookCtx.StopCh)
		return nil
	})
	if b.extensionAPIEnabled {
		extensionsServer, err := buildExtensionAPIServer(completeConfig, genericServer)
		if err != nil {
			return nil, fmt.Errorf("build extensions api server: %w", err)
		}
		genericServer = extensionsServer.GenericAPIServer
	}
	if b.kubeconfigDestFile != "" {
		genericServer.AddPostStartHookOrDie("generate-kubeconfig", func(hookCtx genericapiserver.PostStartHookContext) error {
			return writeKubeconfigFile(b.kubeconfigDestFile, "podpourpi", kubeClientConfig)
		})
	}
	if b.webDir != "" {
		apiPaths := []string{"/api", "/apis", "/readyz", "/healthz", "/livez", "/metrics", "/openapi"}
		genericServer.Handler.FullHandlerChain = NewWebUIHandler(b.webDir, genericServer.Handler.FullHandlerChain, apiPaths)
	}
	return genericServer, nil
}

func writeKubeconfigFile(file string, contextName string, config *restclient.Config) error {
	conf := clientapi.NewConfig()
	cluster := clientapi.NewCluster()
	cluster.Server = config.Host
	conf.Clusters[contextName] = cluster
	ctx := clientapi.NewContext()
	ctx.Cluster = contextName
	ctx.Namespace = "default"
	conf.Contexts[contextName] = ctx
	conf.CurrentContext = contextName
	conf.APIVersion = "v1"
	conf.Kind = "Config"
	err := clientcmd.WriteToFile(*conf, file)
	if err != nil {
		return fmt.Errorf("write kubeconfig: %w", err)
	}
	return nil
}

func buildExtensionAPIServer(genericConfig genericapiserver.CompletedConfig, delegate *genericapiserver.GenericAPIServer) (*apiextensionsapiserver.CustomResourceDefinitions, error) {
	proxyTransport := createProxyTransport()
	versionedInformers := genericConfig.SharedInformerFactory
	serviceResolver, err := buildServiceResolver(genericConfig.LoopbackClientConfig.Host, versionedInformers)
	if err != nil {
		return nil, fmt.Errorf("build service resolver: %w", err)
	}
	authResolverWrapper := webhook.NewDefaultAuthenticationInfoResolverWrapper(proxyTransport, genericConfig.EgressSelector, genericConfig.LoopbackClientConfig, genericConfig.TracerProvider)
	restOptionsGetter := &restOptionsGetter{
		Config:     storagebackend.NewDefaultConfig("prefix", unstructured.UnstructuredJSONScheme),
		ListObject: &unstructured.UnstructuredList{},
		ItemsIterator: func(l runtime.Object, fn func(runtime.Object) error) error {
			return l.(*unstructured.UnstructuredList).EachListItem(fn)
		},
	}
	apiextensionsConfig := &apiextensionsapiserver.Config{
		GenericConfig: &genericapiserver.RecommendedConfig{
			Config:                *genericConfig.Config,
			SharedInformerFactory: versionedInformers,
		},
		ExtraConfig: apiextensionsapiserver.ExtraConfig{
			CRDRESTOptionsGetter: restOptionsGetter,
			MasterCount:          1,
			AuthResolverWrapper:  authResolverWrapper,
			ServiceResolver:      serviceResolver,
		},
	}
	apiextensionsConfig.GenericConfig.RESTOptionsGetter = restOptionsGetter
	apiextensionsConfig.GenericConfig.MergedResourceConfig = extensionAPIResourceConfigSource()
	//notFoundHandler := notfoundhandler.New(genericConfig.Serializer, genericapifilters.NoMuxAndDiscoveryIncompleteKey)
	//return apiextensionsConfig.Complete().New(genericapiserver.NewEmptyDelegateWithCustomHandler(notFoundHandler))
	_ = notfoundhandler.New(genericConfig.Serializer, genericapifilters.NoMuxAndDiscoveryIncompleteKey)
	return apiextensionsConfig.Complete().New(delegate)
}

type restOptionsGetter struct {
	Config        *storagebackend.Config
	ListObject    runtime.Object
	ItemsIterator ItemsIterator
}

func (g *restOptionsGetter) GetRESTOptions(resource schema.GroupResource) (genericregistry.RESTOptions, error) {
	resourceStorageConfig := g.Config.ForResource(resource)
	return genericregistry.RESTOptions{
		StorageConfig:             resourceStorageConfig,
		Decorator:                 g.newInMemoryStore,
		EnableGarbageCollection:   false,
		DeleteCollectionWorkers:   0,
		ResourcePrefix:            "resourceprefix",
		CountMetricPollPeriod:     time.Minute,
		StorageObjectCountTracker: resourceStorageConfig.StorageObjectCountTracker,
	}, nil
}

func (g *restOptionsGetter) newInMemoryStore(
	config *storagebackend.ConfigForResource,
	resourcePrefix string,
	keyFunc func(obj runtime.Object) (string, error),
	newFunc func() runtime.Object,
	newListFunc func() runtime.Object,
	getAttrsFunc storage.AttrFunc,
	trigger storage.IndexerFuncs,
	indexers *cache.Indexers) (storage.Interface, factory.DestroyFunc, error) {
	return NewInMemoryStore(g.ListObject, g.ItemsIterator), func() {}, nil
}

func extensionAPIResourceConfigSource() *serverstorage.ResourceConfig {
	ret := serverstorage.NewResourceConfig()
	// NOTE: GroupVersions listed here will be enabled by default. Don't put alpha versions in the list.
	ret.EnableVersions(
		v1beta1.SchemeGroupVersion,
		v1.SchemeGroupVersion,
	)
	return ret
}

func createProxyTransport() *http.Transport {
	var proxyDialerFn utilnet.DialFunc
	// Proxying to pods and services is IP-based... don't expect to be able to verify the hostname
	proxyTLSClientConfig := &tls.Config{InsecureSkipVerify: true}
	proxyTransport := utilnet.SetTransportDefaults(&http.Transport{
		DialContext:     proxyDialerFn,
		TLSClientConfig: proxyTLSClientConfig,
	})
	return proxyTransport
}

func buildServiceResolver(hostname string, informer clientgoinformers.SharedInformerFactory) (webhook.ServiceResolver, error) {
	/*var serviceResolver webhook.ServiceResolver
	serviceResolver = aggregatorapiserver.NewEndpointServiceResolver(
		informer.Core().V1().Services().Lister(),
		informer.Core().V1().Endpoints().Lister(),
	)*/
	// resolve kubernetes.default.svc locally
	localHost, err := url.Parse(fmt.Sprintf("http://%s", hostname))
	if err != nil {
		return nil, err
	}
	return aggregatorapiserver.NewLoopbackServiceResolver(noopServiceResolver("noop"), localHost), nil
}

type noopServiceResolver string

func (r noopServiceResolver) ResolveEndpoint(namespace, name string, port int32) (*url.URL, error) {
	return nil, fmt.Errorf("cannot resolve service %q with port %d within namespace %q because endpoint resolution is not supported", name, port, namespace)
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
