package server

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/mgoltzsche/podpourpi/internal/runner"
)

// AppController implements all server handlers.
type AppController struct {
	apps *runner.Repository
}

// NewAppController creates a new app controller.
func NewAppController(apps *runner.Repository) *AppController {
	return &AppController{apps: apps}
}

// ListApps lists all apps.
func (c *AppController) ListApps(ctx echo.Context) error {
	var apps runner.AppList
	err := c.apps.List(&apps)
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
	var app runner.App
	err := c.apps.Get(name, &app)
	if err != nil {
		var notFound runner.NotFoundError
		if errors.As(err, &notFound) {
			return writeJSONResponse(ctx.Response(), http.StatusNotFound, Error{
				Type:    "NotFound",
				Message: err.Error(),
			})
		}
		return writeJSONResponse(ctx.Response(), http.StatusNotFound, Error{
			Type:    "unexpected",
			Message: err.Error(),
		})
	}
	return err
}

// UpdateApp updates an app.
func (c *AppController) UpdateApp(ctx echo.Context, name string) error {
	panic("unsupported")
}

func toAppDTO(a *runner.App) App {
	return App{
		Metadata: Metadata{Name: a.Name},
		// TODO: map container status
	}
}
