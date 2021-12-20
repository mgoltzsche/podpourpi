package storage

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
)

type StorageProvider func(s *runtime.Scheme, g generic.RESTOptionsGetter) (rest.Storage, error)
