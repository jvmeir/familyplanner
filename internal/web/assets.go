package web

import (
	"embed"
	"io/fs"
)

//go:embed assets/*
var assetsFS embed.FS

// Assets returns the embedded static asset filesystem rooted at assets/.
func Assets() fs.FS {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err) // embedded path is a compile-time constant; cannot fail at runtime
	}
	return sub
}
