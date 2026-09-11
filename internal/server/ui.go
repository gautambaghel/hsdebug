package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:ui
var uiFS embed.FS

// UIHandler serves the embedded web UI, or nil if unavailable.
func UIHandler() http.Handler {
	sub, err := fs.Sub(uiFS, "ui")
	if err != nil {
		return nil
	}
	return http.FileServer(http.FS(sub))
}
