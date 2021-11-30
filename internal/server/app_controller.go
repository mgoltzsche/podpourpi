package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/mgoltzsche/podpourpi/internal/runner"
	"github.com/sirupsen/logrus"
)

// AppController implements all server handlers.
type AppController struct {
	apps   runner.Store
	runner runner.AppRunner
}

// NewAppController creates a new app controller.
func NewAppController(apps runner.Store, runner runner.AppRunner) *AppController {
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
		if runner.IsNotFound(err) {
			return notFound(w, err.Error())
		}
		return internalServerError(w, err)
	}
	return writeJSONResponse(w, http.StatusOK, toAppDTO(&app))
}

// UpdateApp updates an app.
func (c *AppController) UpdateApp(ctx echo.Context, name string) error {
	w := ctx.Response()
	var dto App
	err := ctx.Bind(&dto)
	if err != nil {
		return badRequest(w, err.Error())
	}
	var app runner.App
	err = c.apps.Get(name, &app)
	if err != nil {
		return notFound(w, err.Error())
	}
	app.Enabled = dto.Spec.Enabled
	// TODO: set/apply profile
	action, err := c.applyApp(ctx.Request().Context(), &app)
	if err != nil {
		_ = writeJSONResponse(w, http.StatusInternalServerError, Error{
			Type:    "AppActionFailed",
			Message: fmt.Sprintf("Failed to %s %s (see server log for details)", action, app.Name),
		})
		return err
	}
	c.apps.Set(name, &app)
	return writeJSONResponse(w, http.StatusAccepted, toAppDTO(&app))
}

func (c *AppController) applyApp(ctx context.Context, a *runner.App) (string, error) {
	if a.Enabled {
		return "start/update", c.runner.Start(a)
	}
	return "stop", c.runner.Stop(a)
}

func toAppDTO(a *runner.App) App {
	containers := make([]Container, len(a.Status.Containers))
	for i, c := range a.Status.Containers {
		containers[i] = Container{
			Name: c.Name,
		}
	}
	return App{
		Metadata: Metadata{Name: a.Name},
		Spec: AppSpec{
			Enabled:       a.Enabled,
			ActiveProfile: "default",
		},
		Status: AppStatus{
			State:      toAppState(a.Status.State),
			Containers: &containers,
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
