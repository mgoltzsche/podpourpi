package runner

import (
	"fmt"
	"sort"
	"sync"
)

type NotFoundError struct {
	error
}

type Resource interface {
	GetName() string
	DeepCopyIntoResource(Resource) error
}

type ResourceList interface {
	SetItems([]Resource) error
}

type Repository struct {
	items map[string]Resource
	mutex *sync.RWMutex
}

func NewRepository() *Repository {
	return &Repository{items: map[string]Resource{}, mutex: &sync.RWMutex{}}
}

func (r *Repository) List(l ResourceList) error {
	items := make([]Resource, 0, len(r.items))
	names := make([]string, 0, len(r.items))
	for _, item := range r.items {
		names = append(names, item.GetName())
	}
	sort.Strings(names)
	for _, k := range names {
		items = append(items, r.items[k])
	}
	return l.SetItems(items)
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

func (r *Repository) Set(name string, item Resource) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.items[name] = item
}

func (r *Repository) Upsert(name string, res Resource, modify func()) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if existing := r.items[name]; existing != nil {
		err := existing.DeepCopyIntoResource(res)
		if err != nil {
			return err
		}
	}
	modify()
	r.items[name] = res
	return nil
}

/*func Copy(from, to interface{}) error {
	if from == nil {
		return nil
	}
	b, err := json.Marshal(from)
	if err != nil {
		return fmt.Errorf("copy struct: marshal: %w", err)
	}
	err = json.Unmarshal(b, to)
	if err != nil {
		return fmt.Errorf("copy struct: unmarshal: %w", err)
	}
	return nil
}*/
