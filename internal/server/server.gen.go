// Package server provides primitives to interact with the openapi HTTP API.
//
// Code generated by github.com/deepmap/oapi-codegen version v1.9.0 DO NOT EDIT.
package server

import (
	"fmt"
	"net/http"

	"github.com/deepmap/oapi-codegen/pkg/runtime"
	"github.com/labstack/echo/v4"
)

// Defines values for AppPropertyType.
const (
	AppPropertyTypePassword AppPropertyType = "password"

	AppPropertyTypeService AppPropertyType = "service"

	AppPropertyTypeString AppPropertyType = "string"
)

// Defines values for AppState.
const (
	AppStateError AppState = "error"

	AppStateExited AppState = "exited"

	AppStateRunning AppState = "running"

	AppStateStarting AppState = "starting"

	AppStateUnknown AppState = "unknown"
)

// Defines values for EventAction.
const (
	EventActionCreate EventAction = "create"

	EventActionDelete EventAction = "delete"

	EventActionUpdate EventAction = "update"
)

// Represents a set of capabilities that a host provides.
type App struct {
	Metadata Metadata  `json:"metadata"`
	Spec     AppSpec   `json:"spec"`
	Status   AppStatus `json:"status"`
}

// AppList defines model for AppList.
type AppList struct {
	Items []App `json:"items"`
}

// AppProfile defines model for AppProfile.
type AppProfile struct {
	App        string        `json:"app"`
	Name       string        `json:"name"`
	Properties []AppProperty `json:"properties"`
}

// AppProperty defines model for AppProperty.
type AppProperty struct {
	Name  string          `json:"name"`
	Type  AppPropertyType `json:"type"`
	Value *string         `json:"value,omitempty"`
}

// AppPropertyType defines model for AppPropertyType.
type AppPropertyType string

// AppSpec defines model for AppSpec.
type AppSpec map[string]interface{}

// AppState defines model for AppState.
type AppState string

// AppStatus defines model for AppStatus.
type AppStatus struct {
	ActiveProfile string      `json:"activeProfile"`
	Containers    []Container `json:"containers"`
	Node          *string     `json:"node,omitempty"`
	State         AppState    `json:"state"`
}

// Container defines model for Container.
type Container struct {
	Id     string          `json:"id"`
	Name   string          `json:"name"`
	Status ContainerStatus `json:"status"`
}

// ContainerStatus defines model for ContainerStatus.
type ContainerStatus struct {
	Message *string  `json:"message,omitempty"`
	State   AppState `json:"state"`
}

// Error defines model for Error.
type Error struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Event defines model for Event.
type Event struct {
	Action EventAction `json:"action"`
	Object EventObject `json:"object"`
}

// EventAction defines model for EventAction.
type EventAction string

// EventList defines model for EventList.
type EventList struct {
	Items []Event `json:"items"`
}

// EventObject defines model for EventObject.
type EventObject struct {
	// Represents a set of capabilities that a host provides.
	App     *App        `json:"app,omitempty"`
	Profile *AppProfile `json:"profile,omitempty"`
}

// Metadata defines model for Metadata.
type Metadata struct {
	// The object identifier
	Name string `json:"name"`
}

// UpdateAppJSONBody defines parameters for UpdateApp.
type UpdateAppJSONBody App

// StartAppParams defines parameters for StartApp.
type StartAppParams struct {
	// Name of an app profile
	Profile *string `json:"profile,omitempty"`
}

// UpdateAppJSONRequestBody defines body for UpdateApp for application/json ContentType.
type UpdateAppJSONRequestBody UpdateAppJSONBody

// ServerInterface represents all server handlers.
type ServerInterface interface {
	// List apps
	// (GET /v1/apps)
	ListApps(ctx echo.Context) error
	// Get an app by name
	// (GET /v1/apps/{app})
	GetApp(ctx echo.Context, app string) error
	// Update app
	// (PUT /v1/apps/{app})
	UpdateApp(ctx echo.Context, app string) error
	// Create app profile
	// (POST /v1/apps/{app}/profile)
	CreateAppProfile(ctx echo.Context, app string) error
	// Get app profile
	// (GET /v1/apps/{app}/profile/{profile})
	GetAppProfile(ctx echo.Context, app string, profile string) error
	// Update app profile
	// (PUT /v1/apps/{app}/profile/{profile})
	UpdateAppProfile(ctx echo.Context, app string, profile string) error
	// Start an app
	// (GET /v1/apps/{app}/start)
	StartApp(ctx echo.Context, app string, params StartAppParams) error
	// Stop an app
	// (GET /v1/apps/{app}/stop)
	StopApp(ctx echo.Context, app string) error
	// Watch changes
	// (GET /v1/events)
	Watch(ctx echo.Context) error
}

// ServerInterfaceWrapper converts echo contexts to parameters.
type ServerInterfaceWrapper struct {
	Handler ServerInterface
}

// ListApps converts echo context to params.
func (w *ServerInterfaceWrapper) ListApps(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.ListApps(ctx)
	return err
}

// GetApp converts echo context to params.
func (w *ServerInterfaceWrapper) GetApp(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "app" -------------
	var app string

	err = runtime.BindStyledParameterWithLocation("simple", false, "app", runtime.ParamLocationPath, ctx.Param("app"), &app)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter app: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetApp(ctx, app)
	return err
}

// UpdateApp converts echo context to params.
func (w *ServerInterfaceWrapper) UpdateApp(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "app" -------------
	var app string

	err = runtime.BindStyledParameterWithLocation("simple", false, "app", runtime.ParamLocationPath, ctx.Param("app"), &app)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter app: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.UpdateApp(ctx, app)
	return err
}

// CreateAppProfile converts echo context to params.
func (w *ServerInterfaceWrapper) CreateAppProfile(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "app" -------------
	var app string

	err = runtime.BindStyledParameterWithLocation("simple", false, "app", runtime.ParamLocationPath, ctx.Param("app"), &app)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter app: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.CreateAppProfile(ctx, app)
	return err
}

// GetAppProfile converts echo context to params.
func (w *ServerInterfaceWrapper) GetAppProfile(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "app" -------------
	var app string

	err = runtime.BindStyledParameterWithLocation("simple", false, "app", runtime.ParamLocationPath, ctx.Param("app"), &app)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter app: %s", err))
	}

	// ------------- Path parameter "profile" -------------
	var profile string

	err = runtime.BindStyledParameterWithLocation("simple", false, "profile", runtime.ParamLocationPath, ctx.Param("profile"), &profile)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter profile: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.GetAppProfile(ctx, app, profile)
	return err
}

// UpdateAppProfile converts echo context to params.
func (w *ServerInterfaceWrapper) UpdateAppProfile(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "app" -------------
	var app string

	err = runtime.BindStyledParameterWithLocation("simple", false, "app", runtime.ParamLocationPath, ctx.Param("app"), &app)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter app: %s", err))
	}

	// ------------- Path parameter "profile" -------------
	var profile string

	err = runtime.BindStyledParameterWithLocation("simple", false, "profile", runtime.ParamLocationPath, ctx.Param("profile"), &profile)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter profile: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.UpdateAppProfile(ctx, app, profile)
	return err
}

// StartApp converts echo context to params.
func (w *ServerInterfaceWrapper) StartApp(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "app" -------------
	var app string

	err = runtime.BindStyledParameterWithLocation("simple", false, "app", runtime.ParamLocationPath, ctx.Param("app"), &app)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter app: %s", err))
	}

	// Parameter object where we will unmarshal all parameters from the context
	var params StartAppParams
	// ------------- Optional query parameter "profile" -------------

	err = runtime.BindQueryParameter("form", true, false, "profile", ctx.QueryParams(), &params.Profile)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter profile: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.StartApp(ctx, app, params)
	return err
}

// StopApp converts echo context to params.
func (w *ServerInterfaceWrapper) StopApp(ctx echo.Context) error {
	var err error
	// ------------- Path parameter "app" -------------
	var app string

	err = runtime.BindStyledParameterWithLocation("simple", false, "app", runtime.ParamLocationPath, ctx.Param("app"), &app)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("Invalid format for parameter app: %s", err))
	}

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.StopApp(ctx, app)
	return err
}

// Watch converts echo context to params.
func (w *ServerInterfaceWrapper) Watch(ctx echo.Context) error {
	var err error

	// Invoke the callback with all the unmarshalled arguments
	err = w.Handler.Watch(ctx)
	return err
}

// This is a simple interface which specifies echo.Route addition functions which
// are present on both echo.Echo and echo.Group, since we want to allow using
// either of them for path registration
type EchoRouter interface {
	CONNECT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	DELETE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	GET(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	HEAD(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	OPTIONS(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PATCH(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	POST(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	PUT(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
	TRACE(path string, h echo.HandlerFunc, m ...echo.MiddlewareFunc) *echo.Route
}

// RegisterHandlers adds each server route to the EchoRouter.
func RegisterHandlers(router EchoRouter, si ServerInterface) {
	RegisterHandlersWithBaseURL(router, si, "")
}

// Registers handlers, and prepends BaseURL to the paths, so that the paths
// can be served under a prefix.
func RegisterHandlersWithBaseURL(router EchoRouter, si ServerInterface, baseURL string) {

	wrapper := ServerInterfaceWrapper{
		Handler: si,
	}

	router.GET(baseURL+"/v1/apps", wrapper.ListApps)
	router.GET(baseURL+"/v1/apps/:app", wrapper.GetApp)
	router.PUT(baseURL+"/v1/apps/:app", wrapper.UpdateApp)
	router.POST(baseURL+"/v1/apps/:app/profile", wrapper.CreateAppProfile)
	router.GET(baseURL+"/v1/apps/:app/profile/:profile", wrapper.GetAppProfile)
	router.PUT(baseURL+"/v1/apps/:app/profile/:profile", wrapper.UpdateAppProfile)
	router.GET(baseURL+"/v1/apps/:app/start", wrapper.StartApp)
	router.GET(baseURL+"/v1/apps/:app/stop", wrapper.StopApp)
	router.GET(baseURL+"/v1/events", wrapper.Watch)

}
