package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUIRootRedirectsToUI(t *testing.T) {
	mux := http.NewServeMux()
	registerUIRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	response, err := client.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", response.StatusCode)
	}
	if location := response.Header.Get("Location"); location != "/ui/" {
		t.Fatalf("Location = %q, want /ui/", location)
	}
}

func TestUIRoutesServeSPAIndex(t *testing.T) {
	mux := http.NewServeMux()
	registerUIRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	for _, requestPath := range []string{"/ui/", "/ui/karts/XKW123", "/ui/unknown"} {
		response, err := http.Get(server.URL + requestPath)
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d", requestPath, response.StatusCode)
		}
		if !strings.Contains(string(body), `<div id="root"></div>`) {
			t.Fatalf("GET %s did not serve SPA index", requestPath)
		}
	}
}

func TestUIAssetsUseUIBasePath(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	response := httptest.NewRecorder()
	staticHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, `src="/ui/assets/`) || !strings.Contains(body, `href="/ui/assets/`) {
		t.Fatalf("index does not use /ui/ asset base: %s", body)
	}
}
