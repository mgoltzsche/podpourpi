// Package v1alpha1 contains API Schema definitions for the podpourpi.mgoltzsche.github.com v1alpha1 API group

//go:generate deepcopy-gen -O zz_generated.deepcopy -i . -h ../../../../boilerplate.go.txt
//go:generate defaulter-gen -O zz_generated.defaults -i . -h ../../../../boilerplate.go.txt
//go:generate conversion-gen -O zz_generated.conversion -i . -h ../../../../boilerplate.go.txt

// +k8s:openapi-gen=true
// +k8s:deepcopy-gen=package,register
// +k8s:conversion-gen=github.com/mgoltzsche/podpourpi/pkg/apis/apps
// +k8s:defaulter-gen=TypeMeta
// +groupName=podpourpi.mgoltzsche.github.com
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is group version used to register these objects
	GroupVersion = schema.GroupVersion{Group: "podpourpi.mgoltzsche.github.com", Version: "v1alpha1"}
)
