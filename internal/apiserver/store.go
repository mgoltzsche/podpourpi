package apiserver

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	//extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
)

type ItemsIterator func(l runtime.Object, each func(runtime.Object) error) error

type inMemoryStore struct {
	objects       map[string]runtime.Object
	listObject    runtime.Object
	itemsIterator ItemsIterator
	lock          sync.RWMutex
	watcherMutex  sync.RWMutex
	watchers      map[int]*watcher
}

func NewInMemoryStore(listObject runtime.Object, itemsFn ItemsIterator) storage.Interface {
	return &inMemoryStore{objects: map[string]runtime.Object{}, listObject: listObject, itemsIterator: itemsFn, watchers: map[int]*watcher{}}
}

func (s *inMemoryStore) Count(key string) (int64, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return int64(len(s.objects)), nil
}

func (s *inMemoryStore) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	k, name, err := objectKey(key, obj)
	if err != nil {
		return fmt.Errorf("create %s: %w", k, err)
	}
	if existing := s.objects[k]; existing != nil {
		return errors.NewAlreadyExists(groupResource(obj), name)
	}
	if ttl > 0 {
		return fmt.Errorf("ttl > 0 is not supported, provided ttl: %d", ttl)
	}
	m, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	m.SetCreationTimestamp(metav1.Now())
	m.SetResourceVersion("0")
	s.objects[k] = obj.DeepCopyObject()
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return fmt.Errorf("create %T: tounstructured: %w", obj, err)
	}
	runtime.DefaultUnstructuredConverter.FromUnstructured(u, out)
	s.notifyWatchers(watch.Event{
		Type:   watch.Added,
		Object: out,
	})
	return nil
}

func objectKey(key string, o runtime.Object) (k, name string, err error) {
	meta, err := meta.Accessor(o)
	if err != nil {
		return "", "", err
	}
	gr := groupResource(o)
	// TODO: derive the object key properly
	if gr.Resource == "customresourcedefinitions" {
		return fmt.Sprintf("%s/%s", key, gr.Group), gr.Group, nil
	}
	name = meta.GetName()
	return fmt.Sprintf("%s/%s%s/%s", key, gr.Group, gr.Resource, name), name, nil
}

func groupResource(obj runtime.Object) schema.GroupResource {
	resource := fmt.Sprintf("%Ts", obj)
	dotPos := strings.Index(resource, ".")
	resource = strings.ToLower(resource[dotPos+1:])
	return obj.GetObjectKind().GroupVersionKind().GroupVersion().WithResource(resource).GroupResource()
}

func (s *inMemoryStore) Delete(ctx context.Context, key string, obj runtime.Object, preconditions *storage.Preconditions, valid storage.ValidateObjectFunc, cachedExistingObj runtime.Object) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	k, name, err := objectKey(key, obj)
	if err != nil {
		return fmt.Errorf("delete %s: %w", k, err)
	}
	// TODO: run preconditions and validators
	found := s.objects[k]
	if found == nil {
		return errors.NewNotFound(groupResource(obj), name)
	}
	delete(s.objects, k)
	return nil
}

func (s *inMemoryStore) Get(ctx context.Context, key string, opts storage.GetOptions, obj runtime.Object) error {
	s.lock.RLock()
	defer s.lock.RUnlock()
	k, name, err := objectKey(key, obj)
	if err != nil {
		return fmt.Errorf("get %s: %w", k, err)
	}
	found := s.objects[k]
	if found == nil {
		if opts.IgnoreNotFound {
			return nil
		}
		return errors.NewNotFound(groupResource(obj), name)
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(found)
	if err != nil {
		return fmt.Errorf("get %T: tounstructured: %w", obj, err)
	}
	runtime.DefaultUnstructuredConverter.FromUnstructured(u, obj)
	return nil
}

func (s *inMemoryStore) GetToList(ctx context.Context, key string, opts storage.ListOptions, obj runtime.Object) error {
	return s.List(ctx, key, opts, obj)
}

func (s *inMemoryStore) GuaranteedUpdate(ctx context.Context, key string, obj runtime.Object, ignoreNotFound bool, preconditions *storage.Preconditions, modify storage.UpdateFunc, cachedExistingObject runtime.Object) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	k, name, err := objectKey(key, obj)
	if err != nil {
		return fmt.Errorf("update %s: %w", k, err)
	}
	found := s.objects[k]
	if found == nil {
		if ignoreNotFound {
			return nil
		}
		return errors.NewNotFound(groupResource(obj), name)
	}
	found = found.DeepCopyObject()
	// TODO: run preconditions and validator
	out, ttl, err := modify(found, storage.ResponseMeta{
		// TODO: set resourceVersion
	})
	if err != nil {
		return fmt.Errorf("update %s: modifier: %w", k, err)
	}
	if ttl != nil && *ttl > 0 {
		return fmt.Errorf("update: ttl > 0 is not supported, modifier returned ttl: %d", ttl)
	}
	s.objects[k] = out.DeepCopyObject()
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(out)
	if err != nil {
		return fmt.Errorf("update %T: tounstructured: %w", out, err)
	}
	runtime.DefaultUnstructuredConverter.FromUnstructured(u, obj)
	s.notifyWatchers(watch.Event{
		Type:   watch.Added,
		Object: out,
	})
	return nil
}

func (s *inMemoryStore) List(ctx context.Context, key string, opts storage.ListOptions, obj runtime.Object) error {
	l, err := getListPrt(obj)
	if err != nil {
		return err
	}
	s.lock.RLock()
	defer s.lock.RUnlock()
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		appendItem(l, s.objects[k])
	}
	return nil
}

func (s *inMemoryStore) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	return s.WatchList(ctx, key, opts)
}

func (s *inMemoryStore) WatchList(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	w := &watcher{
		id:    len(s.watchers),
		store: s,
		ch:    make(chan watch.Event, 10),
	}

	s.watcherMutex.Lock()
	s.watchers[w.id] = w
	s.watcherMutex.Unlock()

	// On initial watch, send all the existing objects
	l := s.listObject.DeepCopyObject()
	err := s.List(ctx, key, opts, l)
	if err != nil {
		return nil, err
	}
	ch := make(chan watch.Event)
	go func() {
		_ = s.itemsIterator(l, func(o runtime.Object) error {
			ch <- watch.Event{
				Type:   watch.Added,
				Object: o,
			}
			return nil
		})
		for evt := range w.ch {
			ch <- evt
		}
		close(ch)
	}()
	return &watcherWrapper{watcher: w, ch: ch}, nil
}

func (s *inMemoryStore) Versioner() storage.Versioner {
	// TODO: impl
	panic("TODO: Versioner()")
}

func (s *inMemoryStore) notifyWatchers(evt watch.Event) {
	s.watcherMutex.RLock()
	defer s.watcherMutex.RUnlock()
	for _, w := range s.watchers {
		w.ch <- evt
	}
}

func getListPrt(listObj runtime.Object) (reflect.Value, error) {
	listPtr, err := meta.GetItemsPtr(listObj)
	if err != nil {
		return reflect.Value{}, err
	}
	v, err := conversion.EnforcePtr(listPtr)
	if err != nil || v.Kind() != reflect.Slice {
		return reflect.Value{}, fmt.Errorf("need ptr to slice: %v", err)
	}
	return v, nil
}

func appendItem(v reflect.Value, obj runtime.Object) {
	v.Set(reflect.Append(v, reflect.ValueOf(obj).Elem()))
}

type watcher struct {
	store *inMemoryStore
	id    int
	ch    chan watch.Event
}

func (w *watcher) Stop() {
	w.store.watcherMutex.Lock()
	delete(w.store.watchers, w.id)
	w.store.watcherMutex.Unlock()
}

func (w *watcher) ResultChan() <-chan watch.Event {
	return w.ch
}

type watcherWrapper struct {
	*watcher
	ch chan watch.Event
}

func (w *watcherWrapper) ResultChan() <-chan watch.Event {
	return w.ch
}
