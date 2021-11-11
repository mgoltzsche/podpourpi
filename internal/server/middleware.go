package server

import (
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/sirupsen/logrus"
)

func apiErrorHandler(logger *logrus.Entry) echo.MiddlewareFunc {
	return echo.MiddlewareFunc(func(h echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			w := ctx.Response()
			err := runRecovered(ctx, h)
			if err != nil {
				var httpErr *echo.HTTPError
				if errors.As(err, &httpErr) && httpErr.Code == http.StatusNotFound {
					_ = writeJSONResponse(w, http.StatusNotFound, Error{
						Type:    "NotFound",
						Message: fmt.Sprintf("%s", httpErr.Message),
					})
					return err
				}
				writeJSONResponse(w, http.StatusInternalServerError, Error{
					Type:    "InternalServerError",
					Message: "unexpected error occured - see server logs",
				})
				return err
			}
			return nil
		}
	})
}

func requestLogger(logger *logrus.Entry) echo.MiddlewareFunc {
	return echo.MiddlewareFunc(func(h echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			start := time.Now()
			req := ctx.Request()
			resp := ctx.Response()
			var err error
			defer func() {
				stop := time.Now()
				l := logger.WithTime(stop).
					WithField("method", req.Method).
					WithField("uri", req.RequestURI).
					WithField("status", resp.Status).
					WithField("latency", stop.Sub(start))
				if err != nil {
					l = l.WithError(err)
				}
				if err != nil || resp.Status >= http.StatusInternalServerError {
					l.Error("request failed")
					return
				}
				l.Info("request served")
			}()
			err = h(ctx)
			return nil
		}
	})
}

func runRecovered(ctx echo.Context, h echo.HandlerFunc) (err error) {
	defer func() {
		if r := recover(); r != nil {
			recovered, ok := r.(error)
			if !ok {
				recovered = fmt.Errorf("%v", r)
			}
			stack := make([]byte, 4<<10)
			length := runtime.Stack(stack, true)
			err = fmt.Errorf("[PANIC RECOVER] %v %s", recovered, stack[:length])
		}
	}()
	return h(ctx)
}
