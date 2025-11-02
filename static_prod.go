//go:build !dev
// +build !dev

package main

import (
	"embed"
	"net/http"
)

var publicFS embed.FS

func public() http.Handler {
	return http.StripPrefix("/public/", http.FileServerFS(publicFS))
}
