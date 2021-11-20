package server

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/mgoltzsche/podpourpi/internal/runner"
)

// AppController implements all server handlers.
type AppController struct {
	apps runner.Store
}

// NewAppController creates a new app controller.
func NewAppController(apps runner.Store) *AppController {
	return &AppController{apps: apps}
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
	panic("unsupported")
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
		Status: AppStatus{
			Containers: &containers,
		},
	}
}
