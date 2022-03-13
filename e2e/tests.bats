#!/usr/bin/env bats

: ${BINARY:=./build/bin/podpourpi}
: ${ADDRESS:=http://127.0.0.1:8080}

TESTDATA=./e2e/test-data
TESTEE_PID_FILE=./build/testee.pid
TESTEE_LOG_FILE=./build/testee.log

setup_file() {
	echo "# setup_file: start server" >&3
	$BINARY serve --ui=./ui/dist --address=127.0.0.1:8080 --compose-apps=./build >"$TESTEE_LOG_FILE" 2>&1 &
	echo $! > "$TESTEE_PID_FILE"
	export KUBECONFIG=./kubeconfig.yaml
	sleep 3
}

teardown_file() {
	echo "# teardown_file: stop server" >&3
	kill -9 "$(cat "$TESTEE_PID_FILE")" 2>/dev/null
}

# ARGS: APIRESOURCE
assertAPIResourceSupported() {
	kubectl api-resources | grep -Eq "^$1 " || (kubectl api-resources; false)
}

@test "server is healthy" {
	curl -fsS "$ADDRESS"
}

@test "server supports ConfigMap api resource" {
	assertAPIResourceSupported configmaps
}

@test "server supports Secret api resource" {
	assertAPIResourceSupported secrets
}

@test "server supports CustomResourceDefinition api resource" {
	assertAPIResourceSupported customresourcedefinitions
}

@test "create ConfigMap" {
	kubectl create configmap some-config --from-literal=somekey=somevalue
	VALUE="$(kubectl get configmap some-config -o jsonpath='{.data.somekey}')"
	[ "$VALUE" = somevalue ]
}

@test "list ConfigMaps" {
	kubectl get configmaps | grep -q some-config || (kubectl get configmaps; false)
}

@test "update ConfigMap" {
	kubectl create configmap some-config --from-literal=somekey=somechangedvalue --dry-run=client -o yaml | kubectl apply -f -
	VALUE="$(kubectl get configmap some-config -o jsonpath='{.data.somekey}')"
	[ "$VALUE" = somechangedvalue ] || (echo "$VALUE"; false)
}

@test "delete ConfigMap" {
	kubectl delete configmap some-config
	! kubectl get configmap some-config
}

@test "create Secret" {
	kubectl create secret generic some-secret --from-literal=somekey=somevalue
	VALUE="$(kubectl get secret some-secret -o jsonpath='{.data.somekey}' | base64 -d)"
	[ "$VALUE" = somevalue ]
}

@test "list Secrets" {
	kubectl get secrets
	kubectl get secrets | grep -q some-secret
}

@test "create CustomResourceDefinition" {
	kubectl apply -f $TESTDATA/customresourcedefinition.yaml
	kubectl get crd sampleresources.some.group.example.org
	kubectl get sampleresources.some.group.example.org
	kubectl get sampleresources
	kubectl get sr
}

@test "add label to CustomResourceDefinition SampleResource" {
	kubectl label crd sampleresources.some.group.example.org somelabel=some-value
	[ "$(kubectl get crd sampleresources.some.group.example.org -o jsonpath='{.metadata.labels.somelabel}')" = some-value ]
	[ "$(kubectl get crd sampleresources.some.group.example.org -o jsonpath='{.metadata.generation}')" = 1 ]
}

@test "wait for CustomResourceDefinition condition Established" {
	kubectl wait --for condition=Established --timeout 7s crd/sampleresources.some.group.example.org
}

@test "create custom resource SampleResource" {
	kubectl apply -f $TESTDATA/customresource.yaml
	kubectl get sr some-example -o yaml
	[ "$(kubectl get sr some-example -o jsonpath='{.spec.somekey}')" = 'some value' ]
	[ "$(kubectl get sr some-example -o jsonpath='{.metadata.generation}')" = 1 ]
}

@test "add label to custom resource SampleResource" {
	kubectl label sr some-example somelabel=some-value
	[ "$(kubectl get sr some-example -o jsonpath='{.metadata.labels.somelabel}')" = 'some-value' ]
	[ "$(kubectl get sr some-example -o jsonpath='{.metadata.generation}')" = 1 ]
}

@test "update custom resource SampleResource spec" {
	kubectl patch sr some-example --type merge --patch '{"spec":{"somekey":"changed value"}}'
	kubectl get sr some-example -o yaml
	[ "$(kubectl get sr some-example -o jsonpath='{.spec.somekey}')" = 'changed value' ]
	[ "$(kubectl get sr some-example -o jsonpath='{.metadata.generation}')" = 2 ]
}

@test "delete custom resource SampleResource" {
	kubectl delete --timeout 7s -f $TESTDATA/customresource.yaml
	! kubectl get sr some-example
}

@test "delete CustomResourceDefinition SampleResource" {
	kubectl delete --timeout 7s -f $TESTDATA/customresourcedefinition.yaml
}

@test "apply cert-manager CustomResourceDefinitions" {
	kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.7.1/cert-manager.crds.yaml
	kubectl get customresourcedefinitions issuers.cert-manager.io
}

@test "list cert-manager CustomResourceDefinitions" {
	kubectl get crds
	kubectl get crds | grep -q issuers.cert-manager.io
}

@test "delete cert-manager CustomResourceDefinitions" {
	kubectl delete --timeout 10s -f https://github.com/cert-manager/cert-manager/releases/download/v1.7.1/cert-manager.crds.yaml
	! kubectl get crd issuers.cert-manager.io
}
