package web

import "net/http"

// scalarDocsHTML renders the Scalar API reference against our OpenAPI document.
// Scalar is loaded from its CDN; the host needs internet for the docs page to
// render, but the /openapi.json and /openapi.yaml specs are always served
// locally regardless.
const scalarDocsHTML = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Komitake API Reference</title>
    <link rel="icon" href="data:," />
  </head>
  <body>
    <div id="app"></div>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
    <script>
      Scalar.createApiReference('#app', {
        url: '/openapi.json',
        theme: 'default',
      })
    </script>
  </body>
</html>`

// registerDocs serves the Scalar API reference at /docs and /docs/.
func registerDocs(mux *http.ServeMux) {
	handler := func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(scalarDocsHTML))
	}
	mux.HandleFunc("GET /docs", handler)
	mux.HandleFunc("GET /docs/", handler)
}
