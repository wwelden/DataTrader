//+build dev
//go:build dev
// +build dev

package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func public() http.Handler {
	fmt.Println("building static files for development")
	fs := http.FileServer(http.FS(os.DirFS("public")))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/public/")

		// Set proper MIME types
		if strings.HasSuffix(path, ".css") {
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		} else if strings.HasSuffix(path, ".js") {
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		}

		http.StripPrefix("/public/", fs).ServeHTTP(w, r)
	})
}
