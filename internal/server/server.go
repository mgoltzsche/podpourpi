package server

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/docker/docker/client"
	"github.com/labstack/echo/v4"
	"github.com/mgoltzsche/podpourpi/internal/runner"
	"github.com/sirupsen/logrus"
)

type Options struct {
	Address    string
	DockerHost string
	UIDir      string
	Logger     *logrus.Entry
}

func RunServer(ctx context.Context, opts Options) error {
	dockerClient, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
		client.WithHost(opts.DockerHost),
	)
	if err != nil {
		return err
	}
	apps := runner.NewRepository()
	containers := runner.WatchContainers(ctx, dockerClient)
	runner.AggregateAppsFromComposeContainers(containers, apps)
	controller := &AppController{apps: apps}

	srv := &http.Server{
		Addr: opts.Address,
	}
	go func() {
		<-ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := srv.Shutdown(ctx); err != nil {
			opts.Logger.Println("error: failed to shut down server:", err)
		}
		cancel()
	}()
	ln, err := net.Listen("tcp", opts.Address)
	if err != nil {
		return err
	}
	logrus.WithField("address", ln.Addr().String()).Info("server listening")
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Listener = ln
	e.GET("/healthz", healthCheckRequestHandler())
	e.Static("/ui", opts.UIDir)
	e.Use(errorHandler(opts.Logger))
	RegisterHandlers(e, controller)
	err = e.StartServer(srv)
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func healthCheckRequestHandler() echo.HandlerFunc {
	return func(ctx echo.Context) error {
		w := ctx.Response()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("ok"))
		return err
	}
}
