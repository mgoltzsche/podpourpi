package kubelet

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientset "k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	kubeletapp "k8s.io/kubernetes/cmd/kubelet/app"
	kubeletappopts "k8s.io/kubernetes/cmd/kubelet/app/options"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/capabilities"
	"k8s.io/kubernetes/pkg/credentialprovider"
	"k8s.io/kubernetes/pkg/kubelet"
	"k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/pkg/kubelet/cadvisor"
	"k8s.io/kubernetes/pkg/kubelet/cm"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/config"
	kubecontainer "k8s.io/kubernetes/pkg/kubelet/container"
	kubetypes "k8s.io/kubernetes/pkg/kubelet/types"
	nodeutil "k8s.io/kubernetes/pkg/util/node"
	"k8s.io/kubernetes/pkg/util/oom"
	"k8s.io/kubernetes/pkg/volume/util/hostutil"
	"k8s.io/kubernetes/pkg/volume/util/subpath"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
	netutils "k8s.io/utils/net"

	// Volume plugins
	"k8s.io/kubernetes/pkg/volume"
	"k8s.io/kubernetes/pkg/volume/configmap"
	"k8s.io/kubernetes/pkg/volume/emptydir"
	"k8s.io/kubernetes/pkg/volume/flexvolume"
	"k8s.io/kubernetes/pkg/volume/hostpath"
	"k8s.io/kubernetes/pkg/volume/local"
	"k8s.io/kubernetes/pkg/volume/projected"
	"k8s.io/kubernetes/pkg/volume/secret"
)

const (
	// Kubelet component name
	componentKubelet = "kubelet"
)

type Options struct {
	RootDirectory       string
	MountPath           string
	FlexVolumePluginDir string
	APIServerHost       string
	Node                NodeOptions
	Runtime             *kubeletconfig.ContainerRuntimeOptions
}

type NodeOptions struct {
	Name string
	IP   string
}

func NewOptions() Options {
	// See https://github.com/kubernetes/kubernetes/blob/v1.23.4/cmd/kubelet/app/options/options.go#L161
	hostname, _ := os.Hostname()
	return Options{
		RootDirectory:       "/var/lib/kubelet",
		MountPath:           "/var/kubelet",
		FlexVolumePluginDir: "/usr/lib/kubelet/volume-plugins",
		Node: NodeOptions{
			Name: hostname,
			IP:   publicIP(),
		},
		Runtime: kubeletappopts.NewContainerRuntimeOptions(),
	}
}

func publicIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			v4 := ipnet.IP.To4()
			if v4 == nil || v4[0] == 127 { // loopback address
				continue
			}
			return v4.String()
		}
	}
	return ""
}

func RunKubelet(ctx context.Context, opts Options) error {
	// See https://github.com/kubernetes/kubernetes/blob/v1.23.4/cmd/kubelet/app/server.go#L1128
	// and https://github.com/kubernetes/kubernetes/blob/v1.23.4/cmd/kubelet/app/server.go#L520
	conf, err := kubeletappopts.NewKubeletConfiguration()
	if err != nil {
		return err
	}

	nodeName, err := nodeutil.GetHostname(opts.Node.Name)
	if err != nil {
		return err
	}
	// TODO: derive default IP?!
	var nodeIPs []net.IP
	if opts.Node.IP != "" {
		for _, ip := range strings.Split(opts.Node.IP, ",") {
			parsedNodeIP := netutils.ParseIPSloppy(strings.TrimSpace(ip))
			if parsedNodeIP == nil {
				klog.InfoS("Could not parse --node-ip ignoring", "IP", ip)
			} else {
				nodeIPs = append(nodeIPs, parsedNodeIP)
			}
		}
	}
	if len(nodeIPs) > 2 || (len(nodeIPs) == 2 && netutils.IsIPv6(nodeIPs[0]) == netutils.IsIPv6(nodeIPs[1])) {
		return fmt.Errorf("bad --node-ip %q; must contain either a single IP or a dual-stack pair of IPs", opts.Node.IP)
	} else if len(nodeIPs) == 2 && (nodeIPs[0].IsUnspecified() || nodeIPs[1].IsUnspecified()) {
		return fmt.Errorf("dual-stack --node-ip %q cannot include '0.0.0.0' or '::'", opts.Node.IP)
	}

	capabilities.Initialize(capabilities.Capabilities{
		AllowPrivileged: true,
	})

	credentialprovider.SetPreferredDockercfgPath(opts.RootDirectory)
	klog.V(2).InfoS("Using root directory", "path", opts.RootDirectory)

	deps, err := dependencies(ctx, nodeName, *conf, opts)
	if err != nil {
		return err
	}
	certDir := "/etc/ssl/certs"
	imageCredentialProviderConfigFile := "" // TODO: specify?
	imageCredentialProviderBinDir := ""
	// TODO: create container runtime somehow
	k, err := kubelet.NewMainKubelet(
		conf,
		deps,
		opts.Runtime,
		opts.Runtime.ContainerRuntime,
		nodeName,
		len(opts.Node.Name) > 0,
		types.NodeName(nodeName),
		nodeIPs,
		"", // no provider
		"", // no cloud provider
		certDir,
		opts.RootDirectory,
		imageCredentialProviderConfigFile,
		imageCredentialProviderBinDir,
		true,
		nil, // no taints
		nil, // no unsafe sysctls
		opts.MountPath,
		true,                         // kernel mem gc notification
		false,                        // don't check node capabilities before mount
		false,                        // ignore node eviction threshold
		metav1.Duration{Duration: 0}, // min gc age
		1,                            // max container count per pod
		-1,                           // max container count
		metav1.NamespaceDefault,      // master service namespace
		true,                         // register schedulable
		true,                         // keep terminated pod volumes
		map[string]string{},          // node labels
		1000,                         // max images
		true,                         // seccomp default
	)
	if err != nil {
		return err
	}
	k.BirthCry()
	k.StartGarbageCollection()
	k.ListenAndServe(conf, nil, deps.Auth)
	return nil
}

func dependencies(ctx context.Context, nodeName string, cfg config.KubeletConfiguration, opts Options) (*kubelet.Dependencies, error) {
	// See https://github.com/kubernetes/kubernetes/blob/v1.23.4/cmd/kubelet/app/server.go#L396
	clientConfig := &rest.Config{Host: opts.APIServerHost}
	client, err := clientset.NewForConfig(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kubelet apiserver client: %w", err)
	}
	eventClientConfig := *clientConfig
	eventClient, err := v1core.NewForConfig(&eventClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kubelet event client: %w", err)
	}
	heartbeatClientConfig := *clientConfig
	heartbeatClientConfig.Timeout = cfg.NodeStatusUpdateFrequency.Duration
	// The timeout is the minimum of the lease duration and status update frequency
	leaseTimeout := time.Duration(cfg.NodeLeaseDurationSeconds) * time.Second
	if heartbeatClientConfig.Timeout > leaseTimeout {
		heartbeatClientConfig.Timeout = leaseTimeout
	}
	heartbeatClientConfig.QPS = float32(-1)
	heartbeatClient, err := clientset.NewForConfig(&heartbeatClientConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize kubelet heartbeat client: %w", err)
	}
	recorder := makeEventRecorder(eventClient, types.NodeName(nodeName))

	// Init auth
	auth, runAuthenticatorCAReload, err := kubeletapp.BuildAuth(types.NodeName(nodeName), client, cfg)
	if err != nil {
		return nil, err
	}
	runAuthenticatorCAReload(ctx.Done())

	// Derive cgroup config
	var cgroupRoots []string
	cgroupRoot := "/pods"
	cgroupPerQOS := false
	cgroupDriver := "cgroupfs"
	kubeletCgroups := "/kubelet"
	nodeAllocatableRoot := cm.NodeAllocatableRoot(cgroupRoot, cgroupPerQOS, cgroupDriver)
	cgroupRoots = append(cgroupRoots, nodeAllocatableRoot)
	kubeletCgroup, err := cm.GetKubeletContainer(kubeletCgroups)
	if err != nil {
		klog.InfoS("Failed to get the kubelet's cgroup. Kubelet system container metrics may be missing.", "err", err)
	} else if kubeletCgroup != "" {
		cgroupRoots = append(cgroupRoots, kubeletCgroup)
	}
	dockerCgroups := "/docker"
	runtimeCgroup, err := cm.GetRuntimeContainer(kubetypes.DockerContainerRuntime, dockerCgroups)
	if err != nil {
		klog.InfoS("Failed to get the container runtime's cgroup. Runtime system container metrics may be missing.", "err", err)
	} else if runtimeCgroup != "" {
		// RuntimeCgroups is optional, so ignore if it isn't specified
		cgroupRoots = append(cgroupRoots, runtimeCgroup)
	}

	// Init cAdvisor (node stats)
	imageFsInfoProvider := cadvisor.NewImageFsInfoProvider(opts.Runtime.ContainerRuntime, opts.Runtime.DockerEndpoint)
	stats := cadvisor.UsingLegacyCadvisorStats(opts.Runtime.ContainerRuntime, opts.Runtime.DockerEndpoint)
	cadvisor, err := cadvisor.New(imageFsInfoProvider, opts.RootDirectory, cgroupRoots, stats)
	if err != nil {
		return nil, err
	}

	// Configure mount path
	mounter := mount.New(opts.MountPath)
	subpather := subpath.New(mounter)
	hu := hostutil.NewHostUtil()
	pluginRunner := exec.New()
	oomAdjuster := oom.NewOOMAdjuster()

	return &kubelet.Dependencies{
		Auth:              auth,     // default does not enforce auth[nz]
		CAdvisorInterface: cadvisor, // cadvisor.New launches background processes (bg http.ListenAndServe, and some bg cleaners)
		Cloud:             nil,      // cloud provider might start background processes
		ContainerManager:  nil,
		DockerOptions: &kubelet.DockerOptions{
			DockerEndpoint:            opts.Runtime.DockerEndpoint,
			RuntimeRequestTimeout:     10 * time.Second,
			ImagePullProgressDeadline: opts.Runtime.ImagePullProgressDeadline.Duration,
		},
		KubeClient:          client,
		HeartbeatClient:     heartbeatClient,
		EventClient:         eventClient,
		Recorder:            recorder,
		HostUtil:            hu,
		Mounter:             mounter,
		Subpather:           subpather,
		OOMAdjuster:         oomAdjuster,
		OSInterface:         kubecontainer.RealOS{},
		VolumePlugins:       volumePlugins(),
		DynamicPluginProber: flexvolume.GetDynamicPluginProber(opts.FlexVolumePluginDir, pluginRunner),
		TLSOptions:          nil,
	}, nil
}

func volumePlugins() []volume.VolumePlugin {
	// See https://github.com/kubernetes/kubernetes/blob/v1.23.4/cmd/kubelet/app/plugins.go#L50
	allPlugins := []volume.VolumePlugin{}
	allPlugins = append(allPlugins, emptydir.ProbeVolumePlugins()...)
	allPlugins = append(allPlugins, hostpath.ProbeVolumePlugins(volume.VolumeConfig{})...)
	allPlugins = append(allPlugins, secret.ProbeVolumePlugins()...)
	allPlugins = append(allPlugins, configmap.ProbeVolumePlugins()...)
	allPlugins = append(allPlugins, projected.ProbeVolumePlugins()...)
	allPlugins = append(allPlugins, local.ProbeVolumePlugins()...)
	return allPlugins
}

// makeEventRecorder sets up the event recorder
func makeEventRecorder(eventClient *v1core.CoreV1Client, nodeName types.NodeName) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(legacyscheme.Scheme, corev1.EventSource{Component: componentKubelet, Host: string(nodeName)})
	eventBroadcaster.StartStructuredLogging(3)
	eventBroadcaster.StartRecordingToSink(&v1core.EventSinkImpl{Interface: eventClient.Events("")})
	return recorder
}
