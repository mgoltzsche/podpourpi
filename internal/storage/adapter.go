package storage

import (
	"context"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
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
func NewRESTStorageProvider(key string, obj resource.Object, target storage.Interface) StorageProvider {
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

		return NewStorageAdapterREST(
			gr,
			codec,
			obj.NamespaceScoped(),
			key,
			obj.New,
			obj.NewList,
			target,
		), nil
	}
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
	key string,
	newFunc func() runtime.Object,
	newListFunc func() runtime.Object,
	store storage.Interface,
) rest.Storage {
	rest := &storageAdapterREST{
		TableConvertor: rest.NewDefaultTableConvertor(groupResource),
		groupResource:  groupResource,
		codec:          codec,
		isNamespaced:   isNamespaced,
		key:            key,
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
	err = f.store.Get(ctx, f.key, opts, obj)
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
	f.store.Create(ctx, f.key, obj, out, 0)

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
	oldObj, err := f.Get(ctx, name, nil)
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

	updatedObj, err := objInfo.UpdatedObject(ctx, oldObj)
	if err != nil {
		return nil, false, err
	}

	if isCreate {
		out := f.newFunc()
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
	f.store.GuaranteedUpdate(ctx, f.key, obj, false, storage.NewUIDPreconditions(string(m.GetUID())), func(input runtime.Object, res storage.ResponseMeta) (out runtime.Object, ttl *uint64, err error) {
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
	oldObj, err := f.Get(ctx, name, nil)
	if err != nil {
		return nil, false, err
	}

	if deleteValidation != nil {
		if err := deleteValidation(ctx, oldObj); err != nil {
			return nil, false, err
		}
	}

	err = f.store.Delete(ctx, f.key, oldObj, nil, nil, oldObj)
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
	panic("TODO: implement DeleteCollection")
}

func (f *storageAdapterREST) Watch(ctx context.Context, options *metainternalversion.ListOptions) (watch.Interface, error) {
	return f.store.Watch(ctx, f.key, storage.ListOptions{
		ResourceVersion: options.ResourceVersion,
		// TODO: set predicate
	})
}
