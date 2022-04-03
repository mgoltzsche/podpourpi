package kubelet

import (
	"context"
	"io"
	"net"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	nodeapi "github.com/virtual-kubelet/virtual-kubelet/node/api"
)

// setupKubeletServer configures and brings up the kubelet API server.
func setupKubeletServer(ctx context.Context, config *Options, p Provider, getPodsFromKubernetes nodeapi.PodListerFunc, logger log.Logger) (_ func(), retErr error) {
	var closers []io.Closer
	cancel := func() {
		for _, c := range closers {
			c.Close()
		}
	}
	defer func() {
		if retErr != nil {
			cancel()
		}
	}()

	// Setup path routing.
	r := mux.NewRouter()

	// This matches the behaviour in the reference kubelet
	r.StrictSlash(true)

	// Setup routes.
	r.HandleFunc("/pods", nodeapi.HandleRunningPods(getPodsFromKubernetes)).Methods("GET")
	r.HandleFunc("/containerLogs/{namespace}/{pod}/{container}", p.GetContainerLogsHandler).Methods("GET")
	r.HandleFunc(
		"/exec/{namespace}/{pod}/{container}",
		nodeapi.HandleContainerExec(
			p.RunInContainer,
			nodeapi.WithExecStreamCreationTimeout(config.StreamCreationTimeout),
			nodeapi.WithExecStreamIdleTimeout(config.StreamIdleTimeout),
		),
	).Methods("POST", "GET")

	// TODO(pires) uncomment this when VK imports k8s.io/kubelet v0.20+
	//if p.GetStatsSummary != nil {
	//	f := nodeapi.HandlePodStatsSummary(p.GetStatsSummary)
	//	r.HandleFunc("/stats/summary", f).Methods("GET")
	//	r.HandleFunc("/stats/summary/", f).Methods("GET")
	//}

	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	})

	// Start the server.
	s := &http.Server{
		Handler: r,
	}
	l, err := net.Listen("", "127.0.0.1:8081")
	if err != nil {
		return nil, err
	}
	go serveHTTP(ctx, s, l, logger)
	closers = append(closers, s)

	// TODO(pires) metrics are disabled until VK supports k8s.io/kubelet v0.20+
	// This is so that we don't import k8s.io/kubernetes.
	//
	// We're keeping the code commented so to not forget and to make it easier
	// to enable later.
	//if cfg.MetricsAddr == "" {
	//	log.G(ctx).Info("pod metrics server not setup due to empty metrics address")
	//} else {
	//	l, err := net.Listen("tcp", cfg.MetricsAddr)
	//	if err != nil {
	//		return nil, errors.Wrap(err, "could not setup listener for pod metrics http server")
	//	}
	//
	//	mux := http.NewServeMux()
	//
	//	var summaryHandlerFunc api.PodStatsSummaryHandlerFunc
	//	if mp, ok := p.(provider.PodMetricsProvider); ok {
	//		summaryHandlerFunc = mp.GetStatsSummary
	//	}
	//	podMetricsRoutes := api.PodMetricsConfig{
	//		GetStatsSummary: summaryHandlerFunc,
	//	}
	//	api.AttachPodMetricsRoutes(podMetricsRoutes, mux)
	//	s := &http.Server{
	//		Handler: mux,
	//	}
	//	go serveHTTP(ctx, s, l, "pod metrics")
	//	closers = append(closers, s)
	//}

	return cancel, nil
}

func serveHTTP(ctx context.Context, s *http.Server, l net.Listener, logger log.Logger) {
	if err := s.Serve(l); err != nil {
		select {
		case <-ctx.Done():
		default:
			logger.WithError(err).Error("failed to setup the kubelet API server")
		}
	}
	l.Close()
}
