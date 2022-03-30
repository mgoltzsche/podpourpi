package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/docker/docker/client"
	"github.com/mgoltzsche/podpourpi/internal/apiserver"
	"github.com/mgoltzsche/podpourpi/internal/kubelet"
	"github.com/mgoltzsche/podpourpi/internal/runner"
	"github.com/mgoltzsche/podpourpi/internal/server"
	"github.com/mgoltzsche/podpourpi/internal/storage"

	appapi "github.com/mgoltzsche/podpourpi/pkg/apis/app/v1alpha1"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"sigs.k8s.io/apiserver-runtime/pkg/builder"
	//"github.com/mgoltzsche/podpourpi/internal/storage/inmemory"
)

func newServeCommand(ctx context.Context, logger *logrus.Entry) *cobra.Command {
	kubeletOpts := kubelet.NewOptions()
	dockerAddress := kubeletOpts.Runtime.DockerEndpoint
	serverOpts := server.Options{
		Address:        "127.0.0.1:8080",
		DockerHost:     dockerAddress,
		UIDir:          "./ui",
		ComposeAppRoot: "/etc/podpourpi/apps",
		Logger:         logger,
	}
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "run the API and web UI server",
		Args:  cobra.ExactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			//return server.RunServer(ctx, opts)
			serverOpts.DockerHost = dockerAddress
			kubeletOpts.Runtime.DockerEndpoint = dockerAddress
			return runAPIServer(ctx, serverOpts, kubeletOpts)
		},
	}
	f := cmd.Flags()
	f.StringVar(&serverOpts.Address, "address", serverOpts.Address, "server listen address")
	f.StringVar(&dockerAddress, "docker-address", dockerAddress, "docker client address")
	f.StringVar(&serverOpts.ComposeAppRoot, "compose-apps", serverOpts.ComposeAppRoot, "directory containing the web UI")
	f.StringVar(&serverOpts.UIDir, "ui", serverOpts.UIDir, "directory containing the web UI")
	f.StringVar(&kubeletOpts.RootDirectory, "kubelet-root-dir", kubeletOpts.RootDirectory, "kubelet root directory")
	f.StringVar(&kubeletOpts.MountPath, "kubelet-mount-path", kubeletOpts.MountPath, "kubelet mount path")
	f.StringVar(&kubeletOpts.Node.Name, "node-name", kubeletOpts.Node.Name, "node name")
	f.StringVar(&kubeletOpts.Node.IP, "node-ip", kubeletOpts.Node.IP, "node IP")
	return cmd
}

func runAPIServer(ctx context.Context, opts server.Options, kubeletOpts kubelet.Options) error {
	/*err := builder.APIServer.
	WithoutEtcd().
	DisableAdmissionControllers().
	DisableAuthorization().
	//ExposeLoopbackAuthorizer().
	//ExposeLoopbackClientConfig().
	WithServerFns(func(opts *builder.GenericAPIServer) *builder.GenericAPIServer {
		//opts.ExternalAddress = "127.0.0.1:8080"
		//opts.SecureServingInfo = nil
		return opts
	}).
	WithOptionsFns(func(options *builder.ServerOptions) *builder.ServerOptions {
		options.RecommendedOptions.CoreAPI = nil
		options.RecommendedOptions.Admission = nil
		// Plain HTTP does not work - HTTPS only:
		//options.RecommendedOptions.SecureServing.WithLoopback()
		//options.RecommendedOptions.SecureServing.Required = false
		//options.RecommendedOptions.SecureServing = nil
		options.RecommendedOptions.Authentication.SkipInClusterLookup = true
		//options.RecommendedOptions.Authentication = nil
		//options.RecommendedOptions.Authorization = nil
		return options
	}).
	WithFlagFns(func(flags *pflag.FlagSet) *pflag.FlagSet {
		securePortFlag := flags.Lookup("secure-port")
		securePortFlag.DefValue = "8443"
		_ = securePortFlag.Value.Set(securePortFlag.DefValue)
		return flags
	}).
	WithResourceAndHandler(&appapi.App{}, filepath.NewJSONFilepathStorageProvider(&v1alpha1.App{}, "data")).
	WithLocalDebugExtension().
	Execute()*/

	app := &appapi.App{}
	appStore := apiserver.NewInMemoryStore()
	sampleApp := &appapi.App{
		ObjectMeta: metav1.ObjectMeta{
			Name: "some-app",
		},
	}
	appKey := storage.ObjectKey(app.GetGroupVersionResource().GroupResource(), sampleApp.GetNamespace(), sampleApp.GetName())
	err := appStore.Create(ctx, appKey, sampleApp, &appapi.App{}, 0)
	if err != nil {
		panic(err)
	}

	//configMap := apiserver.NewResource(&corev1.ConfigMap{}, &corev1.ConfigMapList{}, true, corev1.SchemeGroupVersion.WithResource("configmaps"))

	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
		client.WithHost(opts.DockerHost),
	)
	if err != nil {
		return err
	}
	_, err = runner.NewDockerComposeRunner(ctx, opts.ComposeAppRoot, dockerClient, appStore)
	if err != nil {
		return err
	}

	server, err := apiserver.New().
		/*WithResource(app, inmemory.NewInMemoryStorageProvider(app, sampleApp)).*/
		WithResourceStorage(app, appStore).
		//WithResourceStorage(configMap, apiserver.NewInMemoryStore()).
		// TODO: when enabling this, make sure all paths are mapped within the extension-apiserver since base apiserver openapi schemes are not included within the /openapi/v2 endpoint
		WithExtensionsAPI().
		WithCoreAPI().
		WithWebUI(opts.UIDir).
		GenerateKubeconfig("kubeconfig.yaml").
		Build()
	if err != nil {
		return err
	}
	prepared := server.PrepareRun()
	srv := &http.Server{
		Addr: opts.Address,
	}
	srv.Handler = prepared.Handler
	/*k, err := kubelet.NewKubelet()
	if err != nil {
		return err
	}*/
	return parallelize(ctx,
		func(ctx context.Context) error {
			go func() {
				<-ctx.Done()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := srv.Shutdown(ctx); err != nil {
					opts.Logger.Println("error: failed to shut down server:", err)
				}
				cancel()
			}()
			err := srv.ListenAndServe()
			if err != nil {
				return fmt.Errorf("http server: %w", err)
			}
			return nil
		},
		func(ctx context.Context) error {
			err := prepared.Run(ctx.Done())
			if err != nil {
				return fmt.Errorf("api server: %w", err)
			}
			return nil
		},
		func(ctx context.Context) error {
			time.Sleep(3 * time.Second)
			err := kubelet.RunKubelet(ctx, kubeletOpts)
			if err != nil {
				logrus.Error(err)
				return fmt.Errorf("kubelet: %w", err)
			}
			return nil
		},
	)
}

// parallelize runs the provided methods concurrently and cancels the context when any of them returns.
func parallelize(ctx context.Context, daemons ...func(context.Context) error) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan error, len(daemons))
	for _, fn := range daemons {
		go func(fn func(context.Context) error) {
			err := fn(ctx)
			done <- err
			cancel()
		}(fn)
	}
	for i := 0; i < len(daemons); i++ {
		e := <-done
		if err == nil {
			err = e
		}
	}
	cancel()
	close(done)
	return err
}
