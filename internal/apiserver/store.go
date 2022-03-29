package apiserver

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"
	"sync"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/etcd3"
)

type ObjectKeyFunc func(key string, o runtime.Object) (k, name string, err error)

type inMemoryStore struct {
	objects         map[string]runtime.Object
	lock            sync.RWMutex
	watcherMutex    sync.RWMutex
	watchers        map[int]*watcher
	versioner       etcd3.APIObjectVersioner
	resourceVersion uint64
}

func NewInMemoryStore() storage.Interface {
	return &inMemoryStore{objects: map[string]runtime.Object{}, watchers: map[int]*watcher{}}
}

func (s *inMemoryStore) Count(key string) (int64, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return int64(len(s.objects)), nil
}

func (s *inMemoryStore) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	if existing := s.objects[key]; existing != nil {
		return errors.NewAlreadyExists(groupResource(obj), path.Base(key)) // TODO: provide name instead of key
	}
	if ttl > 0 {
		return fmt.Errorf("ttl > 0 is not supported, provided ttl: %d", ttl)
	}
	m, err := meta.Accessor(obj)
	if err != nil {
		return err
	}
	if len(m.GetUID()) == 0 {
		// TODO: clean this up: While apiextensions (crd) rest impl caller generates uid upfront, the docker .
		// This workaround can be removed when the initial dummy data import gets removed.
		//return fmt.Errorf("object %s does not specify metadata.uid", key)
		m.SetUID(types.UID(uuid.New().String()))
	}
	m.SetCreationTimestamp(metav1.Now())
	m.SetResourceVersion("1") // Setting this to a value >0 fixed the not found error
	fmt.Printf("## add %s\n", key)
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return fmt.Errorf("create %T: tounstructured: %w", obj, err)
	}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(u, out)
	if err != nil {
		return fmt.Errorf("create %T: fromunstructured: %w", obj, err)
	}
	s.objects[key] = obj.DeepCopyObject()
	s.notifyWatchers(watch.Event{
		Type:   watch.Added,
		Object: out,
	})
	return nil
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
	fmt.Printf("## delete %s\n", key)
	err := valid(ctx, obj)
	if err != nil {
		return err
	}
	found := s.objects[key]
	if found == nil {
		return errors.NewNotFound(groupResource(obj), path.Base(key))
	}
	found = found.DeepCopyObject()
	err = preconditions.Check(key, found)
	if err != nil {
		return err
	}
	delete(s.objects, key)
	s.notifyWatchers(watch.Event{
		Type:   watch.Deleted,
		Object: found,
	})
	return nil
}

func (s *inMemoryStore) Get(ctx context.Context, key string, opts storage.GetOptions, obj runtime.Object) error {
	s.lock.RLock()
	defer s.lock.RUnlock()
	fmt.Printf("## get %s\n", key)
	found := s.objects[key]
	if found == nil {
		fmt.Printf("##   -> not found\n")
		if opts.IgnoreNotFound {
			return nil
		}
		return errors.NewNotFound(groupResource(obj), path.Base(key))
	}
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(found)
	if err != nil {
		return fmt.Errorf("get %T: tounstructured: %w", obj, err)
	}
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(u, obj)
	if err != nil {
		return err
	}
	return nil
}

// GetToList fetches custom resources and is used to watch a specific resource for which the key is provided.
func (s *inMemoryStore) GetToList(ctx context.Context, key string, opts storage.ListOptions, obj runtime.Object) error {
	fmt.Printf("## gettolist %T\n", obj)
	l, err := getListPrt(obj)
	if err != nil {
		return err
	}
	s.lock.RLock()
	defer s.lock.RUnlock()
	fmt.Printf("## get %s\n", key)
	found := s.objects[key]
	if found == nil {
		fmt.Printf("##   -> not found\n")
		return errors.NewNotFound(groupResource(obj), path.Base(key))
	}
	appendItem(l, found.DeepCopyObject())
	return nil
}

func (s *inMemoryStore) GuaranteedUpdate(ctx context.Context, key string, obj runtime.Object, ignoreNotFound bool, preconditions *storage.Preconditions, modify storage.UpdateFunc, cachedExistingObject runtime.Object) error {
	s.lock.Lock()
	defer s.lock.Unlock()
	fmt.Printf("## mod %s\n", key)
	found := s.objects[key]
	if found == nil {
		fmt.Printf("##   -> not found (ignore: %v)\n", ignoreNotFound)
		if ignoreNotFound {
			return nil
		}
		return errors.NewNotFound(groupResource(obj), path.Base(key))
	}
	found = found.DeepCopyObject()
	resVer, err := s.versioner.ObjectResourceVersion(found)
	if err != nil {
		return err
	}
	err = preconditions.Check(key, found)
	if err != nil {
		return err
	}
	out, ttl, err := modify(found, storage.ResponseMeta{
		ResourceVersion: resVer,
	})
	if err != nil {
		return err
	}
	if ttl != nil && *ttl > 0 {
		return fmt.Errorf("update: ttl > 0 is not supported, modifier returned ttl: %d", ttl)
	}
	s.resourceVersion++
	s.versioner.UpdateObject(out, s.resourceVersion)
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(out)
	if err != nil {
		return fmt.Errorf("update %T: tounstructured: %w", out, err)
	}
	runtime.DefaultUnstructuredConverter.FromUnstructured(u, obj)
	s.objects[key] = out.DeepCopyObject()
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
	fmt.Println("## list", key)
	keys := make([]string, 0, len(s.objects))
	for k := range s.objects {
		if strings.HasPrefix(k, fmt.Sprintf("%s/", key)) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		appendItem(l, s.objects[k])
	}
	return nil
}

func (s *inMemoryStore) WatchList(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	fmt.Printf("## watchlist %s %s\n", key, opts.ResourceVersion)
	return s.watch(ctx, key, opts), nil
}

func (s *inMemoryStore) watch(ctx context.Context, key string, opts storage.ListOptions) *watcher {
	w := &watcher{
		id:    len(s.watchers),
		store: s,
		ch:    make(chan watch.Event, 10),
	}
	s.watcherMutex.Lock()
	defer s.watcherMutex.Unlock()
	s.watchers[w.id] = w
	return w
}

func (s *inMemoryStore) Watch(ctx context.Context, key string, opts storage.ListOptions) (watch.Interface, error) {
	fmt.Printf("## watch %s %s\n", key, opts.ResourceVersion)
	// TODO: verify difference between Watch() and WatchList() and/or unify both impls
	w := s.watch(ctx, key, opts)
	return w, nil

	// On initial watch, send all the existing objects
	/*l := s.listObject.DeepCopyObject()
	//l := &metav1.List{}
	err := s.List(ctx, key, opts, l)
	if err != nil {
		return nil, err
	}
	ch := make(chan watch.Event)
	go func() {
		//for _, o := range l.Items {
		_ = s.itemsIterator(l, func(o runtime.Object) error {
			ch <- watch.Event{
				Type:   watch.Added,
				Object: o,
			}
			return nil
		})
		//}
		for evt := range w.ch {
			ch <- evt
		}
		close(ch)
	}()
	return &watcherWrapper{watcher: w, ch: ch}, nil*/
}

func (s *inMemoryStore) Versioner() storage.Versioner {
	return &s.versioner
}

func (s *inMemoryStore) notifyWatchers(evt watch.Event) {
	s.watcherMutex.RLock()
	defer s.watcherMutex.RUnlock()
	for _, w := range s.watchers {
		m, _ := meta.Accessor(evt.Object)
		t, _ := meta.TypeAccessor(evt.Object)
		fmt.Printf("## notify %s %s.%s %s %T\n", evt.Type, t.GetKind(), t.GetAPIVersion(), m.GetName(), evt.Object)
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
