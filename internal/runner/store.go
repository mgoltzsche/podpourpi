package runner

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"
)

type ResourceEventType string

const (
	EventTypeCreated = ResourceEventType("create")
	EventTypeUpdated = ResourceEventType("update")
	EventTypeDeleted = ResourceEventType("delete")
)

func IsNotFound(err error) bool {
	var notFound *notFoundError
	return errors.As(err, &notFound)
}

type notFoundError struct {
	error
}

type Resource interface {
	DeepCopyIntoResource(Resource) error
}

type ResourceList interface {
	SetItems([]Resource) error
}

type WatchEvent struct {
	Action   ResourceEventType
	Resource Resource
}

type Store interface {
	List(l ResourceList) ([]Resource, error)
	Get(key string, res Resource) error
	Set(key string, res Resource)
	Delete(key string)
	Modify(key string, res Resource, modify func() (bool, error)) error
}

type store struct {
	items  map[string]Resource
	pubsub *Pubsub
	mutex  *sync.RWMutex
}

func NewStore(pubsub *Pubsub) Store {
	return &store{
		mutex:  &sync.RWMutex{},
		items:  map[string]Resource{},
		pubsub: pubsub,
	}
}

func (r *store) List(l ResourceList) ([]Resource, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	items := make([]Resource, 0, len(r.items))
	keys := make([]string, 0, len(r.items))
	for k := range r.items {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		items = append(items, r.items[k])
	}
	if l == nil {
		return items, nil
	}
	return items, l.SetItems(items)
}

func (r *store) Get(name string, res Resource) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	item := r.items[name]
	if item == nil {
		return &notFoundError{fmt.Errorf("resource %q not found", name)}
	}
	return item.DeepCopyIntoResource(res)
}

func (r *store) Set(key string, item Resource) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	existing := r.items[key]
	r.items[key] = item
	if existing == nil {
		r.emit(EventTypeCreated, item)
		return
	}
	if !reflect.DeepEqual(existing, item) {
		r.emit(EventTypeUpdated, item)
	}
}

func (r *store) Delete(key string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	existing := r.items[key]
	delete(r.items, key)
	if existing != nil {
		r.emit(EventTypeDeleted, existing)
	}
}

func (r *store) Modify(key string, res Resource, modify func() (bool, error)) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	existing := r.items[key]
	if existing != nil { // update
		err := existing.DeepCopyIntoResource(res)
		if err != nil {
			return err
		}
	}
	keep, err := modify()
	if err != nil {
		return err
	}
	if !keep {
		if existing != nil {
			delete(r.items, key)
			r.emit(EventTypeDeleted, res)
		}
		return nil
	}
	r.items[key] = res
	if existing == nil {
		r.emit(EventTypeCreated, res)
		return nil
	}
	if !reflect.DeepEqual(existing, res) {
		r.emit(EventTypeUpdated, res)
	}
	return nil
}

func (r *store) emit(action ResourceEventType, res Resource) {
	r.pubsub.Publish(WatchEvent{Action: action, Resource: res})
}
