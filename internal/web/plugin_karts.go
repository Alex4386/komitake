package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Alex4386/komitake/internal/deviceselect"
	"github.com/Alex4386/komitake/pkg/komitake"
	"github.com/danielgtaylor/huma/v2"
)

// kartsPlugin serves the complete connected-kart API under /karts.
type kartsPlugin struct {
	client Client
}

func (plugin *kartsPlugin) Prefix() string { return "/karts" }

func (plugin *kartsPlugin) Register(api huma.API) {
	huma.Get(api, "", func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Karts []kartDTO `json:"karts"`
		}
	}, error) {
		karts, err := plugin.client.Karts(ctx)
		if err != nil {
			return nil, daemonError(err)
		}
		out := &struct {
			Body struct {
				Karts []kartDTO `json:"karts"`
			}
		}{}
		out.Body.Karts = make([]kartDTO, 0, len(karts))
		for _, kart := range karts {
			out.Body.Karts = append(out.Body.Karts, kartToDTO(kart))
		}
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-kart-by-id",
		Method:      http.MethodGet,
		Path:        "/by-id/{id}",
		Summary:     "Get a connected kart by RCD ident",
	}, func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*struct{ Body kartDTO }, error) {
		kart, err := resolveKart(ctx, plugin.client, input.ID)
		if err != nil {
			return nil, err
		}
		return &struct{ Body kartDTO }{Body: kartToDTO(kart)}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-kart-by-serial",
		Method:      http.MethodGet,
		Path:        "/by-serial/{serial}",
		Summary:     "Redirect to /karts/by-id/{id} for the matching serial",
	}, func(ctx context.Context, input *struct {
		Serial string `path:"serial"`
	}) (*struct {
		Status   int
		Location string `header:"Location"`
	}, error) {
		kart, err := resolveKart(ctx, plugin.client, input.Serial)
		if err != nil {
			return nil, err
		}
		return &struct {
			Status   int
			Location string `header:"Location"`
		}{
			Status:   http.StatusTemporaryRedirect,
			Location: "/v1/karts/by-id/" + url.PathEscape(kart.Ident),
		}, nil
	})

	Mount(api,
		&pairingPlugin{client: plugin.client},
		&drivePlugin{client: plugin.client},
	)
}

// registerKartSerialRedirects keeps nested resources addressable by serial.
func registerKartSerialRedirects(mux *http.ServeMux, client Client) {
	mux.HandleFunc("GET /v1/karts/by-serial/{serial}/{rest...}", func(writer http.ResponseWriter, request *http.Request) {
		kart, err := resolveKart(request.Context(), client, request.PathValue("serial"))
		if err != nil {
			writeResolveError(writer, err)
			return
		}
		target := "/v1/karts/by-id/" + url.PathEscape(kart.Ident)
		if rest := request.PathValue("rest"); rest != "" {
			target += "/" + rest
		}
		if request.URL.RawQuery != "" {
			target += "?" + request.URL.RawQuery
		}
		http.Redirect(writer, request, target, http.StatusTemporaryRedirect)
	})
}

func resolveKart(ctx context.Context, client Client, selector string) (komitake.Kart, error) {
	karts, err := client.Karts(ctx)
	if err != nil {
		return komitake.Kart{}, daemonError(err)
	}
	candidates := make([]deviceselect.Device, 0, len(karts))
	for _, kart := range karts {
		candidates = append(candidates, deviceselect.Device{
			Ident:      kart.Ident,
			Serial:     kart.Serial,
			Kind:       kart.Kind,
			Address:    kart.Address,
			MACAddress: kart.MACAddress,
			SignalDBM:  kart.SignalDBM,
		})
	}
	match, err := deviceselect.Resolve(selector, candidates)
	if err != nil {
		message := err.Error()
		if strings.Contains(message, "ambiguous") {
			return komitake.Kart{}, huma.Error409Conflict(message)
		}
		if strings.Contains(message, "empty") {
			return komitake.Kart{}, huma.Error400BadRequest(message)
		}
		return komitake.Kart{}, huma.Error404NotFound(message)
	}
	for _, kart := range karts {
		if kart.Ident == match.Ident {
			return kart, nil
		}
	}
	return komitake.Kart{}, huma.Error404NotFound(fmt.Sprintf("no kart matches %q", selector))
}

func writeResolveError(writer http.ResponseWriter, err error) {
	message := err.Error()
	statusCode := http.StatusNotFound
	if statusError, ok := err.(huma.StatusError); ok {
		statusCode = statusError.GetStatus()
		message = statusError.Error()
	} else if strings.Contains(message, "ambiguous") {
		statusCode = http.StatusConflict
	} else if strings.Contains(message, "empty") {
		statusCode = http.StatusBadRequest
	}
	http.Error(writer, message, statusCode)
}
