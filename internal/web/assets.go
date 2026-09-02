package web

import (
	"context"
	"io/fs"
	"net/http"
	"strings"

	"github.com/Alex4386/komitake/internal/web/frontend"
)

// staticHandler serves the embedded frontend below /ui/. Unknown UI paths
// fall back to index.html so the client-side router can handle direct loads.
func staticHandler() http.Handler {
	dist, err := frontend.Dist()
	if err != nil {
		return http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte("komitake web UI is not built.\n" +
				"Build it with: cd internal/web/frontend && npm install && npm run build\n" +
				"The REST API is available under /v1.\n"))
		})
	}

	fileServer := http.StripPrefix("/ui/", http.FileServer(http.FS(dist)))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assetPath := strings.TrimPrefix(request.URL.Path, "/ui/")
		if assetPath == "" {
			assetPath = "index.html"
		}
		if _, statErr := fs.Stat(dist, assetPath); statErr != nil {
			request = request.Clone(request.Context())
			request.URL.Path = "/ui/"
		}
		fileServer.ServeHTTP(writer, request)
	})
}

type Options struct {
	ConfigPath string
}

// Handler builds the combined HTTP handler. Options is variadic for backward
// compatibility with callers that do not expose editable settings.
func Handler(client Client, options ...Options) http.Handler {
	hub := NewHub()
	go NewPoller(client, hub).Run(context.Background())

	mux := http.NewServeMux()
	resolvedOptions := Options{}
	if len(options) > 0 {
		resolvedOptions = options[0]
	}
	RegisterAPI(mux, client, resolvedOptions)
	registerRealtime(mux, client, hub)
	registerWebRTC(mux, client, resolvedOptions.ConfigPath)
	registerUIRoutes(mux)
	return mux
}

func registerUIRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/ui/", http.StatusFound)
	})
	mux.Handle("/ui/", staticHandler())
}
