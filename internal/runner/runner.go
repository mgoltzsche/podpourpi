package runner

import (
	"context"
	"io"
)

type AppRunner interface {
	Start(a *App, profile string) error
	Stop(*App) error
	Logs(context.Context, *App) io.ReadCloser
	SupportedTypes() []string
	GetProfile(*App, *Profile) error
	SetProfile(*App, *Profile) error
}
