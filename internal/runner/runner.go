package runner

import (
	"context"
	"io"
)

type AppRunner interface {
	Start(*App) error
	Update(*App) error
	Stop(*App) error
	Logs(context.Context, *App) io.ReadCloser
	SupportedTypes() []string
}
