package server

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/mgoltzsche/podpourpi/internal/runner"
	"github.com/mgoltzsche/podpourpi/internal/store"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// AppController implements all server handlers.
type AppController struct {
	apps   store.Store
	runner runner.AppRunner
}

// NewAppController creates a new app controller.
func NewAppController(apps store.Store, runner runner.AppRunner) *AppController {
	return &AppController{apps: apps, runner: runner}
}

// ListApps lists all apps.
func (c *AppController) ListApps(ctx echo.Context) error {
	var apps runner.AppList
	_, err := c.apps.List(&apps)
	if err != nil {
		return err
	}
	dtos := make([]App, len(apps.Items))
	for i, a := range apps.Items {
		dtos[i] = toAppDTO(&a)
	}
	return writeJSONResponse(ctx.Response(), http.StatusOK, AppList{Items: dtos})
}

// GetApp gets an app by name.
func (c *AppController) GetApp(ctx echo.Context, name string) error {
	w := ctx.Response()
	var app runner.App
	err := c.apps.Get(name, &app)
	if err != nil {
		if store.IsNotFound(err) {
			return notFound(w, err.Error())
		}
		return internalServerError(w, err)
	}
	return writeJSONResponse(w, http.StatusOK, toAppDTO(&app))
}

// UpdateApp updates an app.
func (c *AppController) UpdateApp(ctx echo.Context, name string) error {
	panic("app update not supported - if anything, it would need to configure the autoStart for the app")
}

// StartApp starts an app.
func (c *AppController) StartApp(ctx echo.Context, name string, params StartAppParams) error {
	w := ctx.Response()
	var app runner.App
	err := c.apps.Get(name, &app)
	if err != nil {
		return notFound(w, err.Error())
	}
	profile := ""
	if params.Profile != nil {
		profile = *params.Profile
	}
	err = c.runner.Start(&app, profile)
	if err != nil {
		_ = writeJSONResponse(w, http.StatusInternalServerError, Error{
			Type:    "AppStartFailed",
			Message: fmt.Sprintf("Failed to start app %s (see server log for details)", app.Name),
		})
		return errors.Wrap(err, "start/update app")
	}
	return writeJSONResponse(w, http.StatusOK, toAppDTO(&app))
}

// StopApp starts an app.
func (c *AppController) StopApp(ctx echo.Context, name string) error {
	w := ctx.Response()
	var app runner.App
	err := c.apps.Get(name, &app)
	if err != nil {
		return notFound(w, err.Error())
	}
	err = c.runner.Stop(&app)
	if err != nil {
		_ = writeJSONResponse(w, http.StatusInternalServerError, Error{
			Type:    "AppStartFailed",
			Message: fmt.Sprintf("Failed to start app %s (see server log for details)", app.Name),
		})
		return errors.Wrap(err, "stop app")
	}
	return writeJSONResponse(w, http.StatusOK, toAppDTO(&app))
}

// GetAppProfile get an app profile
func (c *AppController) GetAppProfile(ctx echo.Context, appName, profileName string) error {
	w := ctx.Response()
	var app runner.App
	err := c.apps.Get(appName, &app)
	if err != nil {
		return notFound(w, err.Error())
	}
	profile := runner.Profile{Name: profileName}
	err = c.runner.GetProfile(&app, &profile)
	if err != nil {
		return internalServerError(w, err)
	}
	return writeJSONResponse(w, http.StatusOK, profileToDTO(&profile))
}

func profileToDTO(profile *runner.Profile) (dto AppProfile) {
	dto.Name = profile.Name
	dto.App = profile.App
	dto.Properties = make([]AppProperty, len(profile.Properties))
	for i, p := range profile.Properties {
		value := p.Value
		dto.Properties[i] = AppProperty{Name: p.Name, Value: &value, Type: AppPropertyTypeString}
	}
	return
}

// CreateAppProfile creates a new app profile
func (c *AppController) CreateAppProfile(ctx echo.Context, appName string) error {
	w := ctx.Response()
	err := c.writeAppProfile(ctx, appName)
	if err != nil {
		return err
	}
	// TODO: don't return password properties contained within profile
	return writeJSONResponse(w, http.StatusCreated, AppProfile{})
}

// UpdateAppProfile update an app profile
func (c *AppController) UpdateAppProfile(ctx echo.Context, appName, _ string) error {
	w := ctx.Response()
	err := c.writeAppProfile(ctx, appName)
	if err != nil {
		return err
	}
	// TODO: don't return password properties contained within profile
	return writeJSONResponse(w, http.StatusOK, AppProfile{})
}

func (c *AppController) writeAppProfile(ctx echo.Context, appName string) error {
	w := ctx.Response()
	var dto AppProfile
	err := ctx.Bind(&dto)
	if err != nil {
		return badRequest(w, err.Error())
	}
	var app runner.App
	err = c.apps.Get(appName, &app)
	if err != nil {
		return notFound(w, err.Error())
	}
	profile, err := profileFromDTO(&dto)
	if err != nil {
		return badRequest(w, err.Error())
	}
	err = c.runner.SetProfile(&app, profile)
	if err != nil {
		_ = writeJSONResponse(w, http.StatusInternalServerError, Error{
			Type:    "AppProfileWriteFailed",
			Message: fmt.Sprintf("Failed to write profile %s for app %s (see server log for details)", dto.Name, app.Name),
		})
		return errors.Wrapf(err, "write profile %s for app %s", dto.Name, app.Name)
	}
	return nil
}

func profileFromDTO(dto *AppProfile) (*runner.Profile, error) {
	profile := runner.Profile{
		Name:       dto.Name,
		App:        dto.App,
		Properties: make([]runner.Property, len(dto.Properties)),
	}
	for i, p := range dto.Properties {
		if p.Name == "" {
			return nil, fmt.Errorf("empty property name provided")
		}
		if p.Value == nil {
			return nil, fmt.Errorf("no value specified for property %s", p.Name)
		}
		profile.Properties[i] = runner.Property{Name: p.Name, Value: *p.Value}
	}
	return &profile, nil
}

func toAppDTO(a *runner.App) App {
	containers := make([]Container, len(a.Status.Containers))
	for i, c := range a.Status.Containers {
		containers[i] = Container{
			Id:   c.ID,
			Name: c.Name,
			Status: ContainerStatus{
				State: toAppState(c.State),
			},
		}
	}
	return App{
		Metadata: Metadata{Name: a.Name},
		Status: AppStatus{
			ActiveProfile: a.Status.ActiveProfile,
			State:         toAppState(a.Status.State),
			Containers:    containers,
		},
	}
}

func toAppState(state runner.AppState) AppState {
	switch state {
	case runner.AppStateStarting:
		return AppStateStarting
	case runner.AppStateReady, runner.AppStateStopping:
		return AppStateRunning
	case runner.AppStateError:
		return AppStateError
	case runner.AppStateExited:
		return AppStateExited
	default:
		logrus.Warnf("unmapped internal app state value %q", state)
		return AppStateUnknown
	}
}
