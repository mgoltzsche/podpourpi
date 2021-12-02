package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mgoltzsche/podpourpi/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	AppTypeDockerCompose = "docker-compose"
)

var _ AppRunner = &DockerComposeRunner{}

type DockerComposeRunner struct {
	dir string
}

func NewDockerComposeRunner(dir string) *DockerComposeRunner {
	return &DockerComposeRunner{dir: dir}
}

func (r *DockerComposeRunner) Apps() ([]App, error) {
	files, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, fmt.Errorf("read compose app root dir: %w", err)
	}
	apps := make([]App, 0, len(files))
	for _, file := range files {
		if file.IsDir() {
			composeDir := filepath.Join(r.dir, file.Name())
			_, f2 := os.Stat(filepath.Join(composeDir, "docker-compose.yaml"))
			_, f1 := os.Stat(filepath.Join(composeDir, "docker-compose.yml"))
			if f1 == nil || f2 == nil {
				apps = append(apps, App{
					Name: file.Name(),
					Type: AppTypeDockerCompose,
					Status: AppStatus{
						State: AppStateDisabled,
					},
				})
			}
		}
	}
	return apps, nil
}

func (r *DockerComposeRunner) Start(a *App) error {
	dir := filepath.Join(r.dir, a.Name)
	_, err := runCommand(dir, "docker-compose", "up", "-d")
	return errors.Wrapf(err, "%s: docker-compose up", dir)
}

func (r *DockerComposeRunner) Stop(a *App) error {
	dir := filepath.Join(r.dir, a.Name)
	_, err := runCommand(dir, "docker-compose", "down", "--remove-orphans")
	return errors.Wrapf(err, "%s: docker-compose down", dir)
}

func (r *DockerComposeRunner) Logs(context.Context, *App) io.ReadCloser {
	panic("not yet supported")
}

func (r *DockerComposeRunner) SupportedTypes() []string {
	return []string{AppTypeDockerCompose}
}

func runCommand(dir, cmd string, args ...string) (string, error) {
	c := exec.Command(cmd, args...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	c.Dir = dir
	err := c.Run()
	if err != nil {
		return stdout.String(), errors.Errorf("%s, stderr: %s", err, stderr.String())
	}
	return stdout.String(), nil
}

func AggregateAppsFromComposeContainers(ch <-chan ContainerEvent, repo store.Store) {
	go func() {
		for evt := range ch {
			appName, composeSvc := appNameFromContainer(evt.Container)
			if appName == "" {
				continue
			}
			var app App
			err := repo.Modify(appName, &app, func() (bool, error) {
				switch evt.Type {
				case EventTypeContainerAdd:
					app.Name = appName
					upsertContainer(&app.Status.Containers, evt.Container, composeSvc)
					app.Status.State = appStateFromContainers(app.Status.Containers)
					return true, nil
				case EventTypeContainerDel:
					containers := make([]AppContainer, 0, len(app.Status.Containers))
					for _, c := range app.Status.Containers {
						if c.ID != evt.Container.ID {
							containers = append(containers, c)
						}
					}
					app.Status.Containers = containers
					return true, nil
				case EventTypeError:
					return false, fmt.Errorf("received docker error event: %w", evt.Error)
				}
				return false, fmt.Errorf("received unexpected event type %q", evt.Type)
			})
			if err != nil {
				logrus.WithError(err).Error("failed to stream container into app status")
			}
		}
	}()
	return
}

func appNameFromContainer(c *Container) (composeProject string, composeService string) {
	if c == nil {
		return
	}
	if l := c.Labels; l != nil {
		composeProject = l["com.docker.compose.project"]
		composeService = l["com.docker.compose.service"]
	}
	return
}

func appStateFromContainers(containers []AppContainer) AppState {
	currState := AppStateUnknown
	for _, c := range containers {
		if c.State > currState {
			currState = c.State
		}
	}
	return currState
}

func upsertContainer(containers *[]AppContainer, add *Container, composeSvc string) {
	newContainer := AppContainer{
		ID:    add.ID,
		Name:  composeSvc,
		State: toAppState(add.Status),
	}
	for i, c := range *containers {
		if c.ID == add.ID {
			(*containers)[i] = newContainer
			return
		}
	}
	*containers = append(*containers, newContainer)
}

func toAppState(containerStatus string) AppState {
	switch containerStatus {
	case "exited":
		return AppStateExited
	case "created":
		return AppStateStarting
		//case "stop":
		//		return AppStateStopping
	case "restarting":
		return AppStateError
	case "running":
		return AppStateReady
	default:
		logrus.Warnf("Unexpected container status %q", containerStatus)
		return AppStateUnknown
	}
}
