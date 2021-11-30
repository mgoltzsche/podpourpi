package runner

import (
	"context"
	"fmt"
	"log"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
)

const (
	EventTypeError        EventType = "error"
	EventTypeContainerAdd EventType = "container-add"
	EventTypeContainerDel EventType = "container-del"
)

type EventType string

type ContainerEvent struct {
	Type      EventType
	Container *Container
	Error     error
}

type Container struct {
	ID     string
	Labels map[string]string
	Status string
}

func WatchContainers(ctx context.Context, dockerClient *client.Client) <-chan ContainerEvent {
	ch := make(chan ContainerEvent)
	msgCh, errCh := dockerClient.Events(ctx, types.EventsOptions{})

	go func() {
		containers, err := dockerClient.ContainerList(ctx, types.ContainerListOptions{
			All:    true,
			Latest: true,
		})
		if err != nil {
			ch <- ContainerEvent{
				Type:  EventTypeError,
				Error: fmt.Errorf("list containers: %w", err),
			}
			close(ch)
			return
		}
		// Emit all existing containers first to ensure the receiver has the complete state.
		for _, c := range containers {
			ch <- ContainerEvent{
				Type:      EventTypeContainerAdd,
				Container: &Container{ID: c.ID, Labels: c.Labels, Status: c.State},
			}
		}

		// Emit a new container whenever it was added or changed
		for {
			if errCh == nil && msgCh == nil {
				break
			}
			select {
			case err, ok := <-errCh:
				if !ok {
					errCh = nil
					continue
				}
				if err != context.Canceled {
					ch <- ContainerEvent{
						Type:  EventTypeError,
						Error: fmt.Errorf("received docker error event: %w", err),
					}
				}
			case msg, ok := <-msgCh:
				if !ok {
					msgCh = nil
					continue
				}
				if msg.Scope == "local" && msg.Type == events.ContainerEventType && msg.Actor.ID != "" {
					// TODO: unify container status - load container from disk?
					id := msg.Actor.ID
					status := msg.Action
					c, err := dockerClient.ContainerInspect(ctx, id)
					if err == nil && c.State != nil {
						status = c.State.Status
					}

					evtType := EventTypeContainerAdd
					if msg.Action == "destroy" {
						evtType = EventTypeContainerDel
					}
					ch <- ContainerEvent{
						Type:      evtType,
						Container: &Container{ID: id, Labels: msg.Actor.Attributes, Status: status},
					}
				} else {
					log.Printf("ignored docker event: %#v\n", msg)
				}
			}
		}
		close(ch)
	}()
	return ch
}
