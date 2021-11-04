// Run oapi-codegen to generate code from OpenAPI spec
//go:generate oapi-codegen --package=server --generate types,server -o server.gen.go ../../spec/openapi.yaml
package server
