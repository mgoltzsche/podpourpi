package inmemory

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/mgoltzsche/podpourpi/internal/storage"
	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	storagecodec "k8s.io/apiserver/pkg/server/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
	//builderrest "sigs.k8s.io/apiserver-runtime/pkg/builder/rest"
)

// ErrResourceNotExists means the file doesn't actually exist.
var ErrResourceNotExists = fmt.Errorf("resource doesn't exist")

// ErrNamespaceNotExists means the directory for the namespace doesn't actually exist.
var ErrNamespaceNotExists = errors.New("namespace does not exist")

// NewInMemoryStorageProvider use ephemeral in-memory storage.
func NewInMemoryStorageProvider(obj resource.Object, initObjects ...runtime.Object) storage.StorageProvider {
	return func(scheme *runtime.Scheme, getter generic.RESTOptionsGetter) (rest.Storage, error) {
		gr := obj.GetGroupVersionResource().GroupResource()
		codec, _, err := storagecodec.NewStorageCodec(storagecodec.StorageCodecConfig{
			StorageMediaType:  runtime.ContentTypeJSON,
			StorageSerializer: serializer.NewCodecFactory(scheme),
			StorageVersion:    scheme.PrioritizedVersionsForGroup(obj.GetGroupVersionResource().Group)[0],
			MemoryVersion:     scheme.PrioritizedVersionsForGroup(obj.GetGroupVersionResource().Group)[0],
			Config:            storagebackend.Config{}, // useless fields..
		})
		if err != nil {
			return nil, err
		}

		return NewInMemoryREST(
			gr,
			codec,
			obj.NamespaceScoped(),
			obj.New,
			obj.NewList,
			initObjects,
		), nil
	}
}

var _ rest.StandardStorage = &inmemoryREST{}
var _ rest.Scoper = &inmemoryREST{}
var _ rest.Storage = &inmemoryREST{}

// NewFilepathREST instantiates a new REST storage.
func NewInMemoryREST(
	groupResource schema.GroupResource,
	codec runtime.Codec,
	isNamespaced bool,
	newFunc func() runtime.Object,
	newListFunc func() runtime.Object,
	objects []runtime.Object,
) rest.Storage {
	objectMap := make(map[string]runtime.Object, 5)
	for _, o := range objects {
		objectMap[objectKey(o)] = o
	}
	rest := &inmemoryREST{
		TableConvertor: rest.NewDefaultTableConvertor(groupResource),
		codec:          codec,
		isNamespaced:   isNamespaced,
		newFunc:        newFunc,
		newListFunc:    newListFunc,
		watchers:       make(map[int]*jsonWatch, 10),
		objects:        objectMap,
	}
	return rest
}

type inmemoryREST struct {
	rest.TableConvertor
	codec        runtime.Codec
	isNamespaced bool

	objects map[string]runtime.Object
	lock    sync.RWMutex

	muWatchers sync.RWMutex
	watchers   map[int]*jsonWatch

	newFunc     func() runtime.Object
	newListFunc func() runtime.Object
}

func (f *inmemoryREST) notifyWatchers(ev watch.Event) {
	f.muWatchers.RLock()
	for _, w := range f.watchers {
		w.ch <- ev
	}
	f.muWatchers.RUnlock()
}

func (f *inmemoryREST) New() runtime.Object {
	return f.newFunc()
}

func (f *inmemoryREST) NewList() runtime.Object {
	return f.newListFunc()
}

func (f *inmemoryREST) NamespaceScoped() bool {
	return f.isNamespaced
}

func (f *inmemoryREST) Get(
	ctx context.Context,
	name string,
	options *metav1.GetOptions,
) (runtime.Object, error) {
	f.lock.RLock()
	defer f.lock.RUnlock()
	obj := f.objects[name]
	if obj == nil {
		return nil, ErrResourceNotExists
	}
	return obj, nil
}

func (f *inmemoryREST) List(
	ctx context.Context,
	options *metainternalversion.ListOptions,
) (runtime.Object, error) {
	newListObj := f.NewList()
	items, err := getListPrt(newListObj)
	if err != nil {
		return nil, err
	}

	f.lock.RLock()
	defer f.lock.RUnlock()

	keys := make([]string, 0, len(f.objects))
	for k := range f.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		appendItem(items, f.objects[k])
	}
	return newListObj, nil
}

func (f *inmemoryREST) Create(
	ctx context.Context,
	obj runtime.Object,
	createValidation rest.ValidateObjectFunc,
	options *metav1.CreateOptions,
) (runtime.Object, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	if f.isNamespaced {
		// ensures namespace dir
		_, ok := genericapirequest.NamespaceFrom(ctx)
		if !ok {
			return nil, ErrNamespaceNotExists
		}
	}

	f.objects[objectKey(obj)] = obj
	f.notifyWatchers(watch.Event{
		Type:   watch.Added,
		Object: obj,
	})

	return obj, nil
}

func (f *inmemoryREST) Update(
	ctx context.Context,
	name string,
	objInfo rest.UpdatedObjectInfo,
	createValidation rest.ValidateObjectFunc,
	updateValidation rest.ValidateObjectUpdateFunc,
	forceAllowCreate bool,
	options *metav1.UpdateOptions,
) (runtime.Object, bool, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	isCreate := false
	oldObj, err := f.Get(ctx, name, nil)
	if err != nil {
		if !forceAllowCreate {
			return nil, false, err
		}
		isCreate = true
	}

	// TODO: should not be necessary, verify Get works before creating filepath
	if f.isNamespaced {
		// ensures namespace dir
		_, ok := genericapirequest.NamespaceFrom(ctx)
		if !ok {
			return nil, false, ErrNamespaceNotExists
		}
	}

	updatedObj, err := objInfo.UpdatedObject(ctx, oldObj)
	if err != nil {
		return nil, false, err
	}

	if isCreate {
		if createValidation != nil {
			if err := createValidation(ctx, updatedObj); err != nil {
				return nil, false, err
			}
		}

		f.objects[objectKey(updatedObj)] = updatedObj

		f.notifyWatchers(watch.Event{
			Type:   watch.Added,
			Object: updatedObj,
		})
		return updatedObj, true, nil
	}

	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, oldObj); err != nil {
			return nil, false, err
		}
	}

	f.objects[objectKey(updatedObj)] = updatedObj
	f.notifyWatchers(watch.Event{
		Type:   watch.Modified,
		Object: updatedObj,
	})
	return updatedObj, false, nil
}

func (f *inmemoryREST) Delete(
	ctx context.Context,
	name string,
	deleteValidation rest.ValidateObjectFunc,
	options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	oldObj, err := f.Get(ctx, name, nil)
	if err != nil {
		return nil, false, err
	}

	f.lock.Lock()
	defer f.lock.Unlock()

	if deleteValidation != nil {
		if err := deleteValidation(ctx, oldObj); err != nil {
			return nil, false, err
		}
	}

	delete(f.objects, objectKey(oldObj))
	f.notifyWatchers(watch.Event{
		Type:   watch.Deleted,
		Object: oldObj,
	})
	return oldObj, true, nil
}

func (f *inmemoryREST) DeleteCollection(
	ctx context.Context,
	deleteValidation rest.ValidateObjectFunc,
	options *metav1.DeleteOptions,
	listOptions *metainternalversion.ListOptions,
) (runtime.Object, error) {
	f.lock.Lock()
	defer f.lock.Unlock()
	newListObj := f.NewList()
	_, err := getListPrt(newListObj)
	if err != nil {
		return nil, err
	}
	f.objects = map[string]runtime.Object{}
	return newListObj, nil
}

func objectKey(o runtime.Object) string {
	obj := o.(metav1.Object)
	return obj.GetName()
}

func appendItem(v reflect.Value, obj runtime.Object) {
	v.Set(reflect.Append(v, reflect.ValueOf(obj).Elem()))
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

func (f *inmemoryREST) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	jw := &jsonWatch{
		id: len(f.watchers),
		f:  f,
		ch: make(chan watch.Event, 10),
	}
	// On initial watch, send all the existing objects
	list, err := f.List(ctx, options)
	if err != nil {
		return nil, err
	}

	danger := reflect.ValueOf(list).Elem()
	items := danger.FieldByName("Items")

	var lastItem runtime.Object
	for i := 0; i < items.Len(); i++ {
		lastItem = items.Index(i).Addr().Interface().(runtime.Object)
		jw.ch <- watch.Event{
			Type:   watch.Added,
			Object: lastItem,
		}
	}

	if lastItem != nil {
		// Indicate to the client that synchronization completed
		jw.ch <- watch.Event{
			Type:   watch.Bookmark,
			Object: lastItem,
		}
	}

	f.muWatchers.Lock()
	f.watchers[jw.id] = jw
	f.muWatchers.Unlock()

	return jw, nil
}

type jsonWatch struct {
	f  *inmemoryREST
	id int
	ch chan watch.Event
}

func (w *jsonWatch) Stop() {
	w.f.muWatchers.Lock()
	delete(w.f.watchers, w.id)
	w.f.muWatchers.Unlock()
}

func (w *jsonWatch) ResultChan() <-chan watch.Event {
	return w.ch
}

func (f *inmemoryREST) ConvertToTable(ctx context.Context, object runtime.Object, tableOptions runtime.Object) (*metav1.Table, error) {
	return &metav1.Table{}, nil
}
