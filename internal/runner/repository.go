package runner

import (
	"fmt"
	"sort"
	"sync"
)

type ResourceEventType string

const (
	EventTypeCreated = ResourceEventType("create")
	EventTypeUpdated = ResourceEventType("update")
	EventTypeDeleted = ResourceEventType("delete")
)

type NotFoundError struct {
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

type Repository struct {
	items  map[string]Resource
	mutex  *sync.RWMutex
	pubsub *Pubsub
}

func NewRepository(pubsub *Pubsub) *Repository {
	return &Repository{
		items:  map[string]Resource{},
		mutex:  &sync.RWMutex{},
		pubsub: pubsub,
	}
}

func (r *Repository) List(l ResourceList) ([]Resource, error) {
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

func (r *Repository) Get(name string, res Resource) error {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	item := r.items[name]
	if item == nil {
		return &NotFoundError{fmt.Errorf("resource %q not found", name)}
	}
	return item.DeepCopyIntoResource(res)
}

func (r *Repository) Set(key string, item Resource) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	existing := r.items[key]
	r.items[key] = item
	if existing == nil {
		r.emit(EventTypeCreated, item)
		return
	}
	r.emit(EventTypeUpdated, item)
}

func (r *Repository) Delete(key string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	existing := r.items[key]
	delete(r.items, key)
	if existing != nil {
		r.emit(EventTypeDeleted, existing)
	}
}

func (r *Repository) Upsert(key string, res Resource, modify func()) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	existing := r.items[key]
	if existing != nil { // update
		err := existing.DeepCopyIntoResource(res)
		if err != nil {
			return err
		}
	}
	modify()
	r.items[key] = res
	if existing == nil {
		r.emit(EventTypeCreated, res)
		return nil
	}
	r.emit(EventTypeUpdated, res)
	return nil
}

func (r *Repository) emit(action ResourceEventType, res Resource) {
	r.pubsub.Publish(WatchEvent{Action: action, Resource: res})
}
