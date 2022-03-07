package storage

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	genericapirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	storagecodec "k8s.io/apiserver/pkg/server/storage"
	"k8s.io/apiserver/pkg/storage"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"sigs.k8s.io/apiserver-runtime/pkg/builder/resource"
	//builderrest "sigs.k8s.io/apiserver-runtime/pkg/builder/rest"
)

// NewRESTStorageProvider use ephemeral in-memory storage.
func NewRESTStorageProvider(obj resource.Object, target storage.Interface) StorageProvider {
	return func(scheme *runtime.Scheme, _ generic.RESTOptionsGetter) (rest.Storage, error) {
		return NewRESTStorageAdapter(obj, target, scheme)
	}
}

func NewRESTStorageAdapter(obj resource.Object, target storage.Interface, scheme *runtime.Scheme) (rest.Storage, error) {
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

	return NewStorageAdapterREST(
		gr,
		codec,
		obj.NamespaceScoped(),
		obj.New,
		obj.NewList,
		target,
	), nil
}

var _ rest.StandardStorage = &storageAdapterREST{}
var _ rest.Scoper = &storageAdapterREST{}
var _ rest.Storage = &storageAdapterREST{}

// ErrNamespaceNotExists means the directory for the namespace doesn't actually exist.
var ErrNamespaceNotExists = errors.New("namespace does not exist")

// NewStorageAdapterREST instantiates a new REST storage.
func NewStorageAdapterREST(
	groupResource schema.GroupResource,
	codec runtime.Codec,
	isNamespaced bool,
	newFunc func() runtime.Object,
	newListFunc func() runtime.Object,
	store storage.Interface,
) rest.Storage {
	rest := &storageAdapterREST{
		TableConvertor: rest.NewDefaultTableConvertor(groupResource),
		groupResource:  groupResource,
		codec:          codec,
		isNamespaced:   isNamespaced,
		key:            fmt.Sprintf("/%s.%s", groupResource.Resource, groupResource.Group),
		newFunc:        newFunc,
		newListFunc:    newListFunc,
		store:          store,
	}
	return rest
}

type storageAdapterREST struct {
	rest.TableConvertor
	groupResource schema.GroupResource
	codec         runtime.Codec
	isNamespaced  bool
	key           string

	newFunc     func() runtime.Object
	newListFunc func() runtime.Object
	store       storage.Interface
}

func (f *storageAdapterREST) New() runtime.Object {
	return f.newFunc()
}

func (f *storageAdapterREST) NewList() runtime.Object {
	return f.newListFunc()
}

func (f *storageAdapterREST) NamespaceScoped() bool {
	return f.isNamespaced
}

func (f *storageAdapterREST) Get(
	ctx context.Context,
	name string,
	options *metav1.GetOptions,
) (runtime.Object, error) {
	obj := f.newFunc()
	m, err := meta.Accessor(obj)
	if err != nil {
		return nil, fmt.Errorf("get: %w", err)
	}
	m.SetName(name)
	opts := storage.GetOptions{
		// TODO: set ignoreNotFound?!
		ResourceVersion: options.ResourceVersion,
	}
	key, err := f.objectKey(ctx, m.GetName())
	if err != nil {
		return nil, err
	}
	err = f.store.Get(ctx, key, opts, obj)
	return obj, err
}

func (f *storageAdapterREST) List(
	ctx context.Context,
	options *metainternalversion.ListOptions,
) (runtime.Object, error) {
	newListObj := f.NewList()
	opts := storage.ListOptions{
		ResourceVersion: options.ResourceVersion,
		// TODO: set predicate
	}
	err := f.store.List(ctx, f.key, opts, newListObj)
	return newListObj, err
}

func (f *storageAdapterREST) Create(
	ctx context.Context,
	obj runtime.Object,
	createValidation rest.ValidateObjectFunc,
	options *metav1.CreateOptions,
) (runtime.Object, error) {
	if createValidation != nil {
		if err := createValidation(ctx, obj); err != nil {
			return nil, err
		}
	}

	if f.isNamespaced {
		_, ok := genericapirequest.NamespaceFrom(ctx)
		if !ok {
			return nil, ErrNamespaceNotExists
		}
	}

	out := f.newFunc()
	m, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	key, err := f.objectKey(ctx, m.GetName())
	if err != nil {
		return nil, err
	}
	if len(m.GetUID()) != 0 {
		return nil, fmt.Errorf("cannot create object %s because metadata.uid is already set", key)
	}
	m.SetUID(types.UID(uuid.New().String()))
	err = f.store.Create(ctx, key, obj, out, 0)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (f *storageAdapterREST) Update(
	ctx context.Context,
	name string,
	objInfo rest.UpdatedObjectInfo,
	createValidation rest.ValidateObjectFunc,
	updateValidation rest.ValidateObjectUpdateFunc,
	forceAllowCreate bool,
	options *metav1.UpdateOptions,
) (runtime.Object, bool, error) {
	isCreate := false
	oldObj, err := f.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		if !forceAllowCreate {
			return nil, false, err
		}
		isCreate = true
	}

	// TODO: should not be necessary, verify Get works before creating filepath
	if f.isNamespaced {
		_, ok := genericapirequest.NamespaceFrom(ctx)
		if !ok {
			return nil, false, ErrNamespaceNotExists
		}
	}

	fmt.Printf("## restadapter.Update(): %#v\n", oldObj)

	updatedObj, err := objInfo.UpdatedObject(ctx, oldObj)
	if err != nil {
		return nil, false, fmt.Errorf("rest storage adapter: get updated object: %w", err)
	}

	if isCreate {
		out, err := f.Create(ctx, updatedObj, createValidation, &metav1.CreateOptions{})
		if err != nil {
			return nil, false, err
		}
		return out, true, nil
	}

	if updateValidation != nil {
		if err := updateValidation(ctx, updatedObj, oldObj); err != nil {
			return nil, false, err
		}
	}

	m, err := meta.Accessor(oldObj)
	if err != nil {
		return nil, false, err
	}
	obj := f.newFunc()
	mo, err := meta.Accessor(obj)
	if err != nil {
		return nil, false, err
	}
	key, err := f.objectKey(ctx, name)
	if err != nil {
		return nil, false, err
	}
	mo.SetName(name)
	f.store.GuaranteedUpdate(ctx, key, obj, false, storage.NewUIDPreconditions(string(m.GetUID())), func(input runtime.Object, res storage.ResponseMeta) (out runtime.Object, ttl *uint64, err error) {
		// TODO: polish update, set resourceVersion
		out = updatedObj
		return
	}, oldObj)
	return obj, false, nil
}

func (f *storageAdapterREST) Delete(
	ctx context.Context,
	name string,
	deleteValidation rest.ValidateObjectFunc,
	options *metav1.DeleteOptions) (runtime.Object, bool, error) {
	oldObj, err := f.Get(ctx, name, &metav1.GetOptions{})
	if err != nil {
		return nil, false, err
	}

	if deleteValidation != nil {
		if err := deleteValidation(ctx, oldObj); err != nil {
			return nil, false, err
		}
	}

	key, err := f.objectKey(ctx, name)
	if err != nil {
		return nil, false, err
	}
	err = f.store.Delete(ctx, key, oldObj, nil, nil, oldObj)
	if err != nil {
		return nil, false, err
	}
	return oldObj, true, nil
}

func (f *storageAdapterREST) DeleteCollection(
	ctx context.Context,
	deleteValidation rest.ValidateObjectFunc,
	options *metav1.DeleteOptions,
	listOptions *metainternalversion.ListOptions,
) (runtime.Object, error) {
	return nil, fmt.Errorf("TODO: implement storageAdapterREST.DeleteCollection()")
}

func (f *storageAdapterREST) Watch(ctx context.Context, options *metainternalversion.ListOptions) (w watch.Interface, err error) {
	w, err = f.store.Watch(ctx, f.key, storage.ListOptions{
		ResourceVersion: options.ResourceVersion,
		// TODO: set predicate
	})
	if err != nil {
		return w, err
	}
	defer func() {
		if err != nil {
			w.Stop()
		}
	}()
	// TODO: support deletion event replay when opts.ResourceVersion is set
	if options.ResourceVersion != "" {
		list, err := f.List(ctx, options)
		if err != nil {
			return w, err
		}
		l, err := getListPrt(list)
		if err != nil {
			return w, err
		}
		ch := make(chan watch.Event)
		go func() {
			for i := 0; i < l.Len(); i++ {
				ch <- watch.Event{
					Type:   watch.Added,
					Object: l.Index(i).Addr().Interface().(runtime.Object),
				}
			}
			for evt := range w.ResultChan() {
				ch <- evt
			}
		}()
		return &watcherWrapper{Interface: w, ch: ch}, nil
	}
	return w, nil
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

type watcherWrapper struct {
	watch.Interface
	ch chan watch.Event
}

func (w *watcherWrapper) ResultChan() <-chan watch.Event {
	return w.ch
}

func (f *storageAdapterREST) objectKey(ctx context.Context, name string) (string, error) {
	if f.isNamespaced {
		ns, ok := genericapirequest.NamespaceFrom(ctx)
		if !ok {
			return "", ErrNamespaceNotExists
		}
		return objectKey(f.key, ns, name), nil
	}
	return objectKey(f.key, "_", name), nil
}

func objectKey(prefix, namespace, name string) string {
	if namespace == "" {
		namespace = "_"
	}
	return fmt.Sprintf("%s/%s/%s", prefix, namespace, name)
}

func ObjectKey(gr schema.GroupResource, namespace, name string) string {
	if gr.Group == "" {
		return objectKey(gr.Resource, namespace, name)
	}
	return objectKey(fmt.Sprintf("/%s.%s", gr.Resource, gr.Group), namespace, name)
}
