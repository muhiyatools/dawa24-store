package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/* templates/*
var Files embed.FS

// StaticFS returns the sub filesystem for /static/ assets.
func StaticFS() http.FileSystem {
	sub, err := fs.Sub(Files, "static")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}

// IndexHTML returns the raw index.html template bytes.
func IndexHTML() ([]byte, error) {
	return Files.ReadFile("templates/index.html")
}
