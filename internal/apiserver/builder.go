package apiserver

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsapiserver "k8s.io/apiextensions-apiserver/pkg/apiserver"
	apiextensionsinformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apiserver/pkg/server/filters"
	"k8s.io/apiserver/pkg/server/healthz"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/apiserver/pkg/storage/storagebackend/factory"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	apiregistrationv1helper "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1/helper"
	apiregistrationclient "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1"
	apiregistrationinformers "k8s.io/kube-aggregator/pkg/client/informers/externalversions/apiregistration/v1"
	"k8s.io/kubernetes/pkg/controlplane/controller/crdregistration"

	"k8s.io/apiserver/pkg/util/openapi"
	//kubernetesgeneratedopenapi "k8s.io/kubernetes/pkg/generated/openapi"
	generatedopenapi "github.com/mgoltzsche/podpourpi/pkg/generated/openapi"
	openapinamer "k8s.io/apiserver/pkg/endpoints/openapi"

	"k8s.io/kube-aggregator/pkg/controllers/autoregister"
	//apiextensionsoptions "k8s.io/apiextensions-apiserver/pkg/cmd/server/options"
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
	serverConfigs        []func(*genericapiserver.GenericAPIServer) error
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
	clientgoExternalClient, err := clientgoclientset.NewForConfig(serverConfig.LoopbackClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create real external clientset: %w", err)
	}
	versionedInformer := clientgoinformers.NewSharedInformerFactory(clientgoExternalClient, 10*time.Minute)
	serverConfig.SharedInformerFactory = versionedInformer
	apiGroups, err := b.buildAPIGroupInfos(serverConfig.RESTOptionsGetter, codecs)
	if err != nil {
		return nil, err
	}

	// TODO: Make generic server fallback to extensions server - not the other way around.
	// (However reordering make the extension server controller not receive changes - informer overwritten or sth else missing in the server Config when it is provided to the extension server before the generic server was started?)
	// => Actually, the crd controller seems not to be triggered since the generic API server also has the CRD resource registered, overlaying the extensions apiserver that is used as fallback.
	//    Potential FIX: figure out how to include the fallback server's api resources (kubectl api-resources; apiextensions path is listed properly by http://localhost:8080/).

	notFoundHandler := notfoundhandler.New(serverConfig.Config.Serializer, genericapifilters.NoMuxAndDiscoveryIncompleteKey)
	delegate := genericapiserver.NewEmptyDelegateWithCustomHandler(notFoundHandler)

	// Create API extensions server
	// TODO: Make custom resource cleanup controller work.
	//       Eventually the cleanup is just not triggered since the generation did not change.
	proxyTransport := createProxyTransport()
	serviceResolver, err := buildServiceResolver(serverConfig.Config.LoopbackClientConfig.Host, serverConfig.SharedInformerFactory)
	if err != nil {
		return nil, fmt.Errorf("build service resolver: %w", err)
	}
	var apiExtensionsServer *apiextensionsapiserver.CustomResourceDefinitions
	if b.extensionAPIEnabled {
		apiExtensionsServer, err = buildExtensionAPIServer(serverConfig.Config, serverConfig.SharedInformerFactory, serviceResolver, proxyTransport, delegate)
		if err != nil {
			return nil, fmt.Errorf("build api extensions server: %w", err)
		}
		//genericServer = apiExtensionsServer.GenericAPIServer
		delegate = apiExtensionsServer.GenericAPIServer
	}

	// Create generic server
	genericServer, err := serverConfig.Complete().New("podpourpi-apiserver", delegate)
	if err != nil {
		return nil, err
	}
	// Install API groups
	for _, apiGroup := range apiGroups {
		/*if apiGroup.OptionsExternalVersion.Group == "" {
			//if apiGroup.Scheme.IsGroupRegistered("") {
			// Handle e.g. corev1
			if err := genericServer.InstallLegacyAPIGroup(genericapiserver.DefaultLegacyAPIPrefix, apiGroup); err != nil {
				return nil, fmt.Errorf("install legacy apigroup: %w", err)
			}
		} else {*/
		if err := genericServer.InstallAPIGroup(apiGroup); err != nil {
			return nil, fmt.Errorf("install apigroup: %w", err)
		}
		//}
	}
	for _, fn := range b.serverConfigs {
		err := fn(genericServer)
		if err != nil {
			return nil, err
		}
	}
	genericServer.AddPostStartHookOrDie("start-apiserver-informers", func(hookCtx genericapiserver.PostStartHookContext) error {
		versionedInformer.Start(hookCtx.StopCh)
		return nil
	})

	if b.extensionAPIEnabled {
		// Create aggregator server
		aggregatorServer, err := buildAggregatorServer(serverConfig.Config, serverConfig.SharedInformerFactory, serviceResolver, proxyTransport, genericServer, apiExtensionsServer.Informers)
		if err != nil {
			return nil, fmt.Errorf("build api aggregator server: %w", err)
		}
		genericServer = aggregatorServer.GenericAPIServer
	}

	// Generate kubeconfig file
	if b.kubeconfigDestFile != "" {
		genericServer.AddPostStartHookOrDie("generate-kubeconfig", func(hookCtx genericapiserver.PostStartHookContext) error {
			return writeKubeconfigFile(b.kubeconfigDestFile, "podpourpi", serverConfig.LoopbackClientConfig)
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

/*func enableAPIVersion(server *genericapiserver.GenericAPIServer, gv schema.GroupVersion) {
	av := metav1.GroupVersionForDiscovery{
		Version:      gv.Version,
		GroupVersion: gv.String(),
	}
	server.DiscoveryGroupManager.AddGroup(metav1.APIGroup{
		Name:             apiextensionsv1.SchemeGroupVersion.Group,
		Versions:         []metav1.GroupVersionForDiscovery{av},
		PreferredVersion: av,
	})
}*/

func buildExtensionAPIServer(kubeAPIServerConfig genericapiserver.Config,
	externalInformers clientgoinformers.SharedInformerFactory,
	serviceResolver aggregatorapiserver.ServiceResolver,
	proxyTransport *http.Transport,
	delegate genericapiserver.DelegationTarget) (*apiextensionsapiserver.CustomResourceDefinitions, error) {
	authResolverWrapper := webhook.NewDefaultAuthenticationInfoResolverWrapper(proxyTransport, kubeAPIServerConfig.EgressSelector, kubeAPIServerConfig.LoopbackClientConfig, kubeAPIServerConfig.TracerProvider)
	apiextensionsConfig := &apiextensionsapiserver.Config{
		GenericConfig: &genericapiserver.RecommendedConfig{
			Config:                kubeAPIServerConfig,
			SharedInformerFactory: externalInformers,
		},
		ExtraConfig: apiextensionsapiserver.ExtraConfig{
			CRDRESTOptionsGetter: &restOptionsGetter{
				Config: storagebackend.NewDefaultConfig("/customprefix", unstructured.UnstructuredJSONScheme),
			},
			MasterCount:         1,
			AuthResolverWrapper: authResolverWrapper,
			ServiceResolver:     serviceResolver,
		},
	}
	crdResource := apiextensionsv1.SchemeGroupVersion.WithResource("customresourcedefinitions").String()
	apiextensionsConfig.GenericConfig.RESTOptionsGetter = &restOptionsGetter{
		Config: storagebackend.NewDefaultConfig(crdResource, unstructured.UnstructuredJSONScheme),
	}
	apiextensionsConfig.GenericConfig.MergedResourceConfig = extensionAPIResourceConfigSource()
	//notFoundHandler := notfoundhandler.New(kubeAPIServerConfig.Serializer, genericapifilters.NoMuxAndDiscoveryIncompleteKey)
	//return apiextensionsConfig.Complete().New(genericapiserver.NewEmptyDelegateWithCustomHandler(notFoundHandler))
	//_ = notfoundhandler.New(kubeAPIServerConfig.Serializer, genericapifilters.NoMuxAndDiscoveryIncompleteKey)
	apiExtensionsServer, err := apiextensionsConfig.Complete().New(delegate)
	if err != nil {
		return nil, fmt.Errorf("new api extensions server: %w", err)
	}
	apiextensionsConfig.GenericConfig.PostStartHooks = map[string]genericapiserver.PostStartHookConfigEntry{}
	return apiExtensionsServer, nil
}

func buildAggregatorServer(
	kubeAPIServerConfig genericapiserver.Config,
	externalInformers clientgoinformers.SharedInformerFactory,
	serviceResolver aggregatorapiserver.ServiceResolver,
	proxyTransport *http.Transport,
	delegateAPIServer genericapiserver.DelegationTarget,
	apiExtensionInformers apiextensionsinformers.SharedInformerFactory) (*aggregatorapiserver.APIAggregator, error) {
	// make a shallow copy to let us twiddle a few things
	// most of the config actually remains the same.  We only need to mess with a couple items related to the particulars of the aggregator
	genericConfig := kubeAPIServerConfig
	genericConfig.PostStartHooks = map[string]genericapiserver.PostStartHookConfigEntry{}
	apiregistrationResource := apiregistrationv1.SchemeGroupVersion.WithResource("apiservices").String()
	genericConfig.RESTOptionsGetter = &restOptionsGetter{
		Config: storagebackend.NewDefaultConfig(apiregistrationResource, unstructured.UnstructuredJSONScheme),
	}
	// prevent generic API server from installing the OpenAPI handler. Aggregator server
	// has its own customized OpenAPI handler.
	genericConfig.SkipOpenAPIInstallation = true
	aggregatorConfig := &aggregatorapiserver.Config{
		GenericConfig: &genericapiserver.RecommendedConfig{
			Config:                genericConfig,
			SharedInformerFactory: externalInformers,
		},
		ExtraConfig: aggregatorapiserver.ExtraConfig{
			ServiceResolver: serviceResolver,
			ProxyTransport:  proxyTransport,
		},
	}
	// we need to clear the poststarthooks so we don't add them multiple times to all the servers (that fails)
	aggregatorConfig.GenericConfig.PostStartHooks = map[string]genericapiserver.PostStartHookConfigEntry{}
	aggregatorConfig.GenericConfig.MergedResourceConfig = aggregatorAPIResourceConfigSource()

	// createAggregatorServer
	aggregatorServer, err := aggregatorConfig.Complete().NewWithDelegate(delegateAPIServer)
	if err != nil {
		return nil, err
	}

	// create controllers for auto-registration
	apiRegistrationClient, err := apiregistrationclient.NewForConfig(aggregatorConfig.GenericConfig.LoopbackClientConfig)
	if err != nil {
		return nil, err
	}
	autoRegistrationController := autoregister.NewAutoRegisterController(aggregatorServer.APIRegistrationInformers.Apiregistration().V1().APIServices(), apiRegistrationClient)
	apiServices := apiServicesToRegister(delegateAPIServer, autoRegistrationController)
	crdRegistrationController := crdregistration.NewCRDRegistrationController(
		apiExtensionInformers.Apiextensions().V1().CustomResourceDefinitions(),
		autoRegistrationController)

	err = aggregatorServer.GenericAPIServer.AddPostStartHook("kube-apiserver-autoregistration", func(context genericapiserver.PostStartHookContext) error {
		go crdRegistrationController.Run(5, context.StopCh)
		go func() {
			// let the CRD controller process the initial set of CRDs before starting the autoregistration controller.
			// this prevents the autoregistration controller's initial sync from deleting APIServices for CRDs that still exist.
			// we only need to do this if CRDs are enabled on this server.  We can't use discovery because we are the source for discovery.
			if aggregatorConfig.GenericConfig.MergedResourceConfig.AnyVersionForGroupEnabled("apiextensions.k8s.io") {
				crdRegistrationController.WaitForInitialSync()
			}
			autoRegistrationController.Run(5, context.StopCh)
		}()
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = aggregatorServer.GenericAPIServer.AddBootSequenceHealthChecks(
		makeAPIServiceAvailableHealthCheck(
			"autoregister-completion",
			apiServices,
			aggregatorServer.APIRegistrationInformers.Apiregistration().V1().APIServices(),
		),
	)
	if err != nil {
		return nil, err
	}

	return aggregatorServer, nil
}

func aggregatorAPIResourceConfigSource() *serverstorage.ResourceConfig {
	ret := serverstorage.NewResourceConfig()
	// NOTE: GroupVersions listed here will be enabled by default. Don't put alpha versions in the list.
	ret.EnableVersions(
		apiregistrationv1.SchemeGroupVersion,
	)
	return ret
}

// makeAPIServiceAvailableHealthCheck returns a healthz check that returns healthy
// once all of the specified services have been observed to be available at least once.
func makeAPIServiceAvailableHealthCheck(name string, apiServices []*apiregistrationv1.APIService, apiServiceInformer apiregistrationinformers.APIServiceInformer) healthz.HealthChecker {
	// Track the auto-registered API services that have not been observed to be available yet
	pendingServiceNamesLock := &sync.RWMutex{}
	pendingServiceNames := sets.NewString()
	for _, service := range apiServices {
		pendingServiceNames.Insert(service.Name)
	}

	// When an APIService in the list is seen as available, remove it from the pending list
	handleAPIServiceChange := func(service *apiregistrationv1.APIService) {
		pendingServiceNamesLock.Lock()
		defer pendingServiceNamesLock.Unlock()
		if !pendingServiceNames.Has(service.Name) {
			return
		}
		if apiregistrationv1helper.IsAPIServiceConditionTrue(service, apiregistrationv1.Available) {
			pendingServiceNames.Delete(service.Name)
		}
	}

	// Watch add/update events for APIServices
	apiServiceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    func(obj interface{}) { handleAPIServiceChange(obj.(*apiregistrationv1.APIService)) },
		UpdateFunc: func(old, new interface{}) { handleAPIServiceChange(new.(*apiregistrationv1.APIService)) },
	})

	// Don't return healthy until the pending list is empty
	return healthz.NamedCheck(name, func(r *http.Request) error {
		pendingServiceNamesLock.RLock()
		defer pendingServiceNamesLock.RUnlock()
		if pendingServiceNames.Len() > 0 {
			return fmt.Errorf("missing APIService: %v", pendingServiceNames.List())
		}
		return nil
	})
}

func apiServicesToRegister(delegateAPIServer genericapiserver.DelegationTarget, registration autoregister.AutoAPIServiceRegistration) []*apiregistrationv1.APIService {
	apiServices := []*apiregistrationv1.APIService{}

	for _, curr := range delegateAPIServer.ListedPaths() {
		if curr == "/api/v1" {
			apiService := makeAPIService(schema.GroupVersion{Group: "", Version: "v1"})
			if apiService == nil {
				continue
			}
			registration.AddAPIServiceToSyncOnStart(apiService)
			apiServices = append(apiServices, apiService)
			continue
		}

		if !strings.HasPrefix(curr, "/apis/") {
			continue
		}
		// this comes back in a list that looks like /apis/rbac.authorization.k8s.io/v1alpha1
		tokens := strings.Split(curr, "/")
		if len(tokens) != 4 {
			continue
		}

		apiService := makeAPIService(schema.GroupVersion{Group: tokens[2], Version: tokens[3]})
		if apiService == nil {
			continue
		}
		registration.AddAPIServiceToSyncOnStart(apiService)
		apiServices = append(apiServices, apiService)
	}

	return apiServices
}

// See https://github.com/kubernetes/kubernetes/blob/v1.23.0/cmd/kube-apiserver/app/aggregator.go#L268
var apiVersionPriorities = map[schema.GroupVersion]priority{
	{Group: "apiextensions.k8s.io", Version: "v1"}: {group: 16700, version: 15},
	//{Group: "apiextensions.k8s.io", Version: "v1beta1"}: {group: 16700, version: 9},
}

// priority defines group priority that is used in discovery. This controls
// group position in the kubectl output.
type priority struct {
	// group indicates the order of the group relative to other groups.
	group int32
	// version indicates the relative order of the version inside of its group.
	version int32
}

func makeAPIService(gv schema.GroupVersion) *apiregistrationv1.APIService {
	apiServicePriority, ok := apiVersionPriorities[gv]
	if !ok {
		// if we aren't found, then we shouldn't register ourselves because it could result in a CRD group version
		// being permanently stuck in the APIServices list.
		klog.Infof("Skipping APIService creation for %v", gv)
		return nil
	}
	return &apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{Name: gv.Version + "." + gv.Group},
		Spec: apiregistrationv1.APIServiceSpec{
			Group:                gv.Group,
			Version:              gv.Version,
			GroupPriorityMinimum: apiServicePriority.group,
			VersionPriority:      apiServicePriority.version,
		},
	}
}

type restOptionsGetter struct {
	Config *storagebackend.Config
}

func (g *restOptionsGetter) GetRESTOptions(resource schema.GroupResource) (genericregistry.RESTOptions, error) {
	resourceStorageConfig := g.Config.ForResource(resource)
	return genericregistry.RESTOptions{
		StorageConfig:             resourceStorageConfig,
		Decorator:                 g.newInMemoryStore,
		EnableGarbageCollection:   false,
		DeleteCollectionWorkers:   0,
		ResourcePrefix:            resource.String(),
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
	return NewInMemoryStore(), func() {}, nil
}

func extensionAPIResourceConfigSource() *serverstorage.ResourceConfig {
	ret := serverstorage.NewResourceConfig()
	// NOTE: GroupVersions listed here will be enabled by default. Don't put alpha versions in the list.
	ret.EnableVersions(
		apiextensionsv1.SchemeGroupVersion,
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
