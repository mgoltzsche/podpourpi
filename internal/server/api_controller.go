package server

import (
	"github.com/labstack/echo/v4"
	"github.com/mgoltzsche/podpourpi/internal/runner"
)

// APIController implements all server handlers.
type APIController struct {
	*AppController
	pubsub *runner.Pubsub
	stores []runner.Store
}

// NewAPIController creates a new app controller.
func NewAPIController(pubsub *runner.Pubsub, apps runner.Store, appRunner runner.AppRunner) *APIController {
	nodes := runner.NewStore(pubsub)
	return &APIController{
		AppController: NewAppController(apps, appRunner),
		stores:        []runner.Store{nodes, apps},
		pubsub:        pubsub,
	}
}

// Watch streams all changes
func (c *APIController) Watch(ctx echo.Context) error {
	ch := make(chan EventList)
	changes := c.pubsub.Subscribe(ctx.Request().Context())
	go func() {
		syncState := make([]Event, 0, len(c.stores))
		for _, store := range c.stores {
			items, _ := store.List(nil)
			for _, item := range items {
				// Emit event for each object the store contains
				syncState = append(syncState, Event{
					Action: EventActionCreate,
					Object: toEventObjectDTO(item),
				})
			}
		}
		ch <- EventList{Items: syncState}
		for change := range changes {
			// Emit subsequent event whenever a change occurs
			ch <- EventList{Items: []Event{{
				// TODO: map evt.action properly
				Action: EventAction(change.Action),
				Object: toEventObjectDTO(change.Resource),
			}}}
			// TODO: emit keep-alive message (empty event list)
		}
		close(ch)
	}()
	w := stream(ctx.Response())
	var err error
	for evt := range ch {
		e := w.Write(evt)
		if e != nil && err == nil {
			err = e
		}
	}
	return err
}

func toEventObjectDTO(r runner.Resource) EventObject {
	switch o := r.(type) {
	case *runner.App:
		appDTO := toAppDTO(o)
		return EventObject{
			App: &appDTO,
		}
	default:
		return EventObject{}
	}
}
