package runner

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	AppTypeDockerCompose = "docker-compose"
)

var _ AppRunner = &DockerComposeRunner{}

type DockerComposeRunner struct {
}

func (r *DockerComposeRunner) Start(a *App) error {
	return r.Update(a)
}

func (r *DockerComposeRunner) Update(a *App) error {
	_, err := runCommand(a.Dir, "docker-compose", "up", "-d")
	if err != nil {
		return errors.Wrapf(err, "%s: docker-compose up", filepath.Base(a.Dir))
	}
	err = r.updateContainers(a)
	if err != nil {
		return err
	}
	return err
}

func (r *DockerComposeRunner) updateContainers(a *App) error {
	out, err := runCommand(a.Dir, "docker-compose", "ps", "-q")
	if err != nil {
		return errors.Wrapf(err, "%s: docker-compose ps", filepath.Base(a.Dir))
	}
	out = strings.TrimSpace(out)
	if out == "" {
		a.Status.Containers = nil
		return nil
	}
	// Set app.status.containers
	containerIDs := strings.Split(out, "\n")
	containerMap := map[string]AppContainer{}
	for _, c := range a.Status.Containers {
		containerMap[c.ID] = c
	}
	containers := make([]AppContainer, len(containerIDs))
	for i, id := range containerIDs {
		if c, ok := containerMap[id]; ok {
			containers[i] = c
		} else {
			containers[i] = AppContainer{ID: id}
		}
	}
	return nil
}

func (r *DockerComposeRunner) Stop(a *App) error {
	_, err := runCommand(a.Dir, "docker-compose", "down", "--remove-orphans")
	return errors.Wrapf(err, "%s: docker-compose down", filepath.Base(a.Dir))
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

func AggregateAppsFromComposeContainers(ch <-chan ContainerEvent, repo *Repository) {
	go func() {
		for evt := range ch {
			switch evt.Type {
			case EventTypeContainerAdd:
				appName, composeSvc := appNameFromContainer(evt.Container)
				var app App
				err := repo.Upsert(appName, &app, func() {
					app.Name = appName
					upsertContainer(&app.Status.Containers, evt.Container, composeSvc)
				})
				if err != nil {
					logrus.WithError(err).Errorf("failed to upsert app")
					continue
				}
			case EventTypeContainerDel:
				appName, _ := appNameFromContainer(evt.Container)
				var app App
				err := repo.Upsert(appName, &app, func() {
					containers := make([]AppContainer, 0, len(app.Status.Containers))
					for _, c := range app.Status.Containers {
						if c.ID != evt.Container.ID {
							containers = append(containers, c)
						}
					}
					app.Status.Containers = containers
				})
				if err != nil {
					logrus.WithError(err).Error("failed to delete container from app")
					continue
				}
				if len(app.Status.Containers) == 0 {
					repo.Delete(appName)
				}
			case EventTypeError:
				logrus.WithError(evt.Error).Error("received docker error event")
			}
		}
	}()
	return
}

func appNameFromContainer(c *Container) (composeProject string, composeService string) {
	if l := c.Labels; l != nil {
		composeProject = l["com.docker.compose.project"]
		composeService = l["com.docker.compose.service"]
	}
	return
}

func upsertContainer(containers *[]AppContainer, add *Container, composeSvc string) {
	newContainer := AppContainer{
		ID:    add.ID,
		Name:  composeSvc, // TODO: provide name here
		State: AppState(add.Status),
	}
	for i, c := range *containers {
		if c.ID == c.ID {
			(*containers)[i] = newContainer
			return
		}
	}
	*containers = append(*containers, newContainer)
}
