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

func errorHandler(logger *logrus.Entry) echo.MiddlewareFunc {
	return echo.MiddlewareFunc(func(h echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			start := time.Now()
			req := ctx.Request()
			resp := ctx.Response()
			err := runRecovered(ctx, h)
			stop := time.Now()
			logger = logger.WithTime(stop).
				WithField("method", req.Method).
				WithField("uri", req.RequestURI).
				WithField("status", resp.Status).
				WithField("latency", stop.Sub(start))
			if err != nil {
				logger = logger.WithError(err)
				logger.Error("request failed")
				var httpErr *echo.HTTPError
				if errors.As(err, &httpErr) {
					if httpErr.Code == http.StatusNotFound {
						_ = writeJSONResponse(resp, http.StatusNotFound, Error{
							Type:    "NotFound",
							Message: fmt.Sprintf("%s", httpErr.Message),
						})
						return nil
					}
					_ = writeJSONResponse(resp, httpErr.Code, Error{
						Type:    "Generic",
						Message: fmt.Sprintf("%s", httpErr.Message),
					})
				}
				_ = writeJSONResponse(resp, http.StatusInternalServerError, Error{
					Type:    "InternalServerError",
					Message: "(see server logs)",
				})
				return nil
			}
			logger.Info("request succeeded")
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
