package kubelet

import (
	"context"
	"path"
	"time"

	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	kubeinformers "k8s.io/client-go/informers"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
)

type Options struct {
	APIServer             *rest.Config
	StartupTimeout        time.Duration
	InformerResyncPeriod  time.Duration
	StreamCreationTimeout time.Duration
	StreamIdleTimeout     time.Duration
	PodSyncWorkers        int
	NodeName              string
	Log                   log.Logger
}

func Start(ctx context.Context, opts *Options) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Setup a clientset.
	client, err := kubeclient.NewForConfig(opts.APIServer)
	if err != nil {
		return err
	}

	// Create a shared informer factory for Kubernetes Pods assigned to this Node.
	podInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(
		client,
		opts.InformerResyncPeriod,
		kubeinformers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", opts.NodeName).String()
		}),
	)
	podInformer := podInformerFactory.Core().V1().Pods()

	// Create another shared informer factory for Kubernetes secrets and configmaps (not subject to any selectors).
	informerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(client, opts.InformerResyncPeriod)
	secretInformer := informerFactory.Core().V1().Secrets()
	configMapInformer := informerFactory.Core().V1().ConfigMaps()
	serviceInformer := informerFactory.Core().V1().Services() // TODO(pires) why Services?

	// Setup the known Pods related resources manager.
	podResourceWatcher := newPodResourceWatcher(informerFactory, opts.Log)

	// Setup the provider.
	// TODO: implement provider
	p, err := provider.New(ctx, opts, podResourceWatcher)
	if err != nil {
		return err
	}

	// Set up event handlers for ConfigMap events.
	configMapInformer.Informer().AddEventHandler(podResourceWatcher.EventHandlerFuncs(ctx, p))
	// Set up event handlers for Secret events.
	secretInformer.Informer().AddEventHandler(podResourceWatcher.EventHandlerFuncs(ctx, p))

	// Setup Node object.
	pNode, err := p.ConfigureNode(ctx, opts)
	if err != nil {
		return err
	}
	// And the Node provider. No need to go fancy here just yet.
	np := node.NewNaiveNodeProvider()
	nodeLog := opts.Log.WithField("node", opts.NodeName)
	additionalOptions := []node.NodeControllerOpt{
		node.WithNodeStatusUpdateErrorHandler(func(ctx context.Context, err error) error {
			if !k8serrors.IsNotFound(err) {
				return err
			}
			nodeLog.Debug("node not found")
			newNode := pNode.DeepCopy()
			newNode.ResourceVersion = ""
			_, err = client.CoreV1().Nodes().Create(ctx, newNode, metav1.CreateOptions{})
			if err != nil {
				return err
			}
			nodeLog.Debug("registered node")
			return nil
		}),
		node.WithNodeEnableLeaseV1(client.CoordinationV1().Leases("kube-node-lease"), 0),
	}
	// Set up the Node controller.
	nodeRunner, err := node.NewNodeController(
		np,
		pNode,
		client.CoreV1().Nodes(),
		additionalOptions...,
	)
	if err != nil {
		nodeLog.Fatal(errors.Wrap(err, "failed to set up node controller"))
	}

	// An event recorder is needed for the Pod controller.
	eb := record.NewBroadcaster()
	// Event recorder logging happens at debug level.
	eb.StartLogging(opts.Log.Debugf)
	eb.StartRecordingToSink(&corev1client.EventSinkImpl{Interface: client.CoreV1().Events(metav1.NamespaceAll)})
	// Set up the Pod controller.
	pc, err := node.NewPodController(node.PodControllerConfig{
		PodClient:         client.CoreV1(),
		PodInformer:       podInformer,
		EventRecorder:     eb.NewRecorder(scheme.Scheme, corev1.EventSource{Component: path.Join(pNode.Name, "pod-controller")}),
		Provider:          p,
		SecretInformer:    secretInformer,
		ConfigMapInformer: configMapInformer,
		ServiceInformer:   serviceInformer,
	})
	if err != nil {
		return errors.Wrap(err, "error setting up pod controller")
	}

	// Finally, start the informers.
	podInformerFactory.Start(ctx.Done())
	podInformerFactory.WaitForCacheSync(ctx.Done())
	informerFactory.Start(ctx.Done())
	informerFactory.WaitForCacheSync(ctx.Done())

	// Serve the kubelet API.
	cancelHTTP, err := setupKubeletServer(ctx, opts, p, func(context.Context) ([]*corev1.Pod, error) {
		return podInformer.Lister().List(labels.Everything())
	}, opts.Log)
	if err != nil {
		return err
	}
	defer cancelHTTP()

	// Start the Pod controller.
	go func() {
		if err := pc.Run(ctx, opts.PodSyncWorkers); err != nil && errors.Cause(err) != context.Canceled {
			nodeLog.Fatal(errors.Wrap(err, "failed to start pod controller"))
		}
	}()

	if opts.StartupTimeout > 0 {
		ctx, cancel := context.WithTimeout(ctx, opts.StartupTimeout)
		nodeLog.Info("waiting for pod controller to be ready")
		select {
		case <-ctx.Done():
			cancel()
			return ctx.Err()
		case <-pc.Ready():
		}
		cancel()
		if err := pc.Err(); err != nil {
			return err
		}
	}

	// Start the Node controller.
	go func() {
		if err := nodeRunner.Run(ctx); err != nil {
			nodeLog.Fatal(errors.Wrap(err, "failed to start node controller"))
		}
	}()

	// If we got here, set Node condition Ready.
	setNodeReady(pNode)
	if err := np.UpdateStatus(ctx, pNode); err != nil {
		return errors.Wrap(err, "error marking the node as ready")
	}
	nodeLog.Info("systemk initialized")

	<-ctx.Done()
	return nil
}

func setNodeReady(n *corev1.Node) {
	for i, c := range n.Status.Conditions {
		if c.Type != "Ready" {
			continue
		}

		c.Message = "systemk is ready"
		c.Reason = "KubeletReady"
		c.Status = corev1.ConditionTrue
		c.LastHeartbeatTime = metav1.Now()
		c.LastTransitionTime = metav1.Now()
		n.Status.Conditions[i] = c
		return
	}
}
