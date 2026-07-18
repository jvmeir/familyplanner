//go:build !(js && wasm)

// Package main's non-wasm stub. The real kiosk SPA client is main.go, built only
// for GOOS=js GOARCH=wasm (see Taskfile `spa` target). This stub exists so that
// `go build ./...` and `go vet ./...` on a normal platform don't fail with
// "build constraints exclude all Go files".
package main

func main() {}
