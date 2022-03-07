package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	storageadapter "github.com/mgoltzsche/podpourpi/internal/storage"

	"github.com/docker/docker/client"
	"github.com/mgoltzsche/podpourpi/internal/store"
	appapi "github.com/mgoltzsche/podpourpi/pkg/apis/app/v1alpha1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/storage"
)

const (
	AppTypeDockerCompose = "docker-compose"
)

var _ AppRunner = &DockerComposeRunner{}

type DockerComposeRunner struct {
	dir  string
	apps store.Store
}

func NewDockerComposeRunner(ctx context.Context, dir string, dockerClient *client.Client, apps storage.Interface) (*DockerComposeRunner, error) {
	composeApps, err := composeAppsFromDirectories(dir)
	if err != nil {
		return nil, err
	}
	for _, a := range composeApps {
		app := a
		appKey := storageadapter.ObjectKey(appapi.GroupVersion.WithResource("apps").GroupResource(), a.Namespace, a.Name)
		err = apps.Create(ctx, appKey, &app, &appapi.App{}, 0)
		if err != nil {
			return nil, err
		}
	}
	containers := watchDockerContainers(ctx, dockerClient)
	aggregateAppsFromComposeContainers(containers, apps)
	// TODO: adjust
	//return &DockerComposeRunner{dir: dir, apps: apps}, nil
	return nil, nil
}

func (r *DockerComposeRunner) Start(a *App, profile string) error {
	envFile, err := r.activateProfile(a, profile)
	if err != nil {
		return errors.Wrapf(err, "start app %s", a.Name)
	}
	if profile != "" {
		r.apps.Modify(a.Name, a, func() (bool, error) {
			a.Status.ActiveProfile = profile
			return true, nil
		})
	}
	cmd := []string{"up", "-d"}
	if envFile != "" {
		cmd = append(cmd, fmt.Sprintf("--env-file=%s", envFile))
	}
	dir := filepath.Join(r.dir, a.Name)
	_, err = runCommand(dir, "docker-compose", cmd...)
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

func (r *DockerComposeRunner) activateProfile(a *App, profile string) (string, error) {
	activeProfilePath := activeProfileLinkPath(r.dir)
	if profile == "" {
		if _, err := os.Stat(activeProfilePath); err != nil {
			if os.IsNotExist(err) {
				return "", nil
			}
			return "", err
		}
		return "", nil
	}
	profileFile := fmt.Sprintf("%s.env", profile)
	profilePath := filepath.Join(r.dir, profileFile)
	if _, err := os.Stat(profilePath); err != nil {
		if os.IsNotExist(err) {
			return "", errors.Errorf("profile %q does not exist for app %s", profile, a.Name)
		}
		return "", errors.Wrap(err, "activate profile")
	}
	err := os.Symlink(activeProfilePath, profileFile)
	return activeProfilePath, errors.Wrap(err, "activate profile")
}

func (r *DockerComposeRunner) GetProfile(a *App, p *Profile) error {
	if a.Name == "" {
		return fmt.Errorf("no app name provided")
	}
	if p.Name == "" {
		return fmt.Errorf("no profile name provided")
	}
	envFile := filepath.Join(r.dir, a.Name, "profiles", fmt.Sprintf("%s.env", p.Name))
	b, err := ioutil.ReadFile(envFile)
	if err != nil {
		return fmt.Errorf("read app profile: %w", err)
	}
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 || kv[0] == "" {
			return fmt.Errorf("unexpected entry within env file %s at line %d. expected KEY=VALUE entry", envFile, i)
		}
		p.Properties = append(p.Properties, Property{Name: kv[0], Value: kv[1]})
	}
	return nil
}

func (r *DockerComposeRunner) SetProfile(a *App, p *Profile) error {
	if a.Name == "" {
		return fmt.Errorf("no app name provided")
	}
	if p.Name == "" {
		return fmt.Errorf("no profile name provided")
	}
	profilesDir := filepath.Join(r.dir, a.Name, "profiles")
	envFile := filepath.Join(profilesDir, fmt.Sprintf("%s.env", p.Name))
	envFileContents := ""
	for _, p := range p.Properties {
		envFileContents = fmt.Sprintf("%s%s=%s", envFileContents, p.Name, p.Value)
	}
	err := os.MkdirAll(profilesDir, 0750)
	if err != nil {
		return fmt.Errorf("write profile: %w", err)
	}
	err = ioutil.WriteFile(envFile, []byte(envFileContents), 0640)
	if err != nil {
		return fmt.Errorf("write profile env file: %w", err)
	}
	return nil
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

func activeProfileLinkPath(dir string) string {
	return filepath.Join(dir, "env.active")
}

func composeAppsFromDirectories(dir string) ([]appapi.App, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read compose app root dir: %w", err)
	}
	apps := make([]appapi.App, 0, len(files))
	for _, file := range files {
		if file.IsDir() {
			composeDir := filepath.Join(dir, file.Name())
			_, f2 := os.Stat(filepath.Join(composeDir, "docker-compose.yaml"))
			_, f1 := os.Stat(filepath.Join(composeDir, "docker-compose.yml"))
			if f1 == nil || f2 == nil {
				activeProfile, err := deriveActiveProfileName(composeDir)
				if err != nil && !os.IsNotExist(err) {
					return nil, errors.Wrapf(err, "detect active profile of app %s", file.Name())
				}
				var a appapi.App
				a.Name = file.Name()
				//a.Type = AppTypeDockerCompose
				a.Status.ActiveProfile = activeProfile
				a.Status.State = appapi.AppStateUnknown
				apps = append(apps, a)
			}
		}
	}
	return apps, nil
}

func deriveActiveProfileName(profileDir string) (string, error) {
	profileLink := activeProfileLinkPath(profileDir)
	path, err := os.Readlink(profileLink)
	if err != nil {
		return "", err
	}
	path = filepath.Base(path)
	envFileSuffix := ".env"
	if !strings.HasSuffix(path, envFileSuffix) {
		return "", errors.Errorf("active profile link %s points to file with unexpected name %q, expected *.env", profileLink, path)
	}
	return path[:len(path)-len(envFileSuffix)], nil
}

func aggregateAppsFromComposeContainers(ch <-chan ContainerEvent, store storage.Interface) {
	go func() {
		for evt := range ch {
			appName, composeSvc := appNameFromContainer(evt.Container)
			if appName == "" {
				continue
			}
			var a appapi.App
			key := fmt.Sprintf("apps.%s", appapi.GroupVersion.Group)
			a.Name = appName
			// TODO: align key
			err := store.GuaranteedUpdate(context.Background(), key, &a, true, nil, func(input runtime.Object, res storage.ResponseMeta) (output runtime.Object, ttl *uint64, err error) {
				//a.Type = AppTypeDockerCompose
				a := input.(*appapi.App)
				output = a
				switch evt.Type {
				case EventTypeContainerAdd:
					upsertContainer(&a.Status.Containers, evt.Container, composeSvc)
					a.Status.State = appStateFromContainers(a.Status.Containers)
					return
				case EventTypeContainerDel:
					containers := make([]appapi.Container, 0, len(a.Status.Containers))
					for _, c := range a.Status.Containers {
						if c.ID != evt.Container.ID {
							containers = append(containers, c)
						}
					}
					a.Status.Containers = containers
					return
				case EventTypeError:
					return nil, ttl, fmt.Errorf("received docker error event: %w", evt.Error)
				}
				return nil, ttl, fmt.Errorf("received unexpected event type %q", evt.Type)
			}, nil)
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

func appStateFromContainers(containers []appapi.Container) appapi.AppState {
	currState := appapi.AppStateUnknown
	for _, c := range containers {
		// TODO: fix state resolution
		if c.Status.State > currState {
			currState = c.Status.State
		}
	}
	return currState
}

func upsertContainer(containers *[]appapi.Container, add *Container, composeSvc string) {
	newContainer := appapi.Container{
		ID:   add.ID,
		Name: composeSvc,
		Status: appapi.ContainerStatus{
			State: toAppState(add.Status),
		},
	}
	for i, c := range *containers {
		if c.ID == add.ID {
			(*containers)[i] = newContainer
			return
		}
	}
	*containers = append(*containers, newContainer)
}

func toAppState(containerStatus string) appapi.AppState {
	switch containerStatus {
	case "exited":
		return appapi.AppStateExited
	case "created":
		return appapi.AppStateStarting
		//case "stop":
		//		return AppStateStopping
	case "restarting":
		return appapi.AppStateError
	case "running":
		return appapi.AppStateRunning
	default:
		logrus.Warnf("Unexpected container status %q", containerStatus)
		return appapi.AppStateUnknown
	}
}
