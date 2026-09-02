package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// daemonPlugin exposes daemon lifecycle actions (reload, restart) under /daemon.
type daemonPlugin struct {
	client Client
}

func (plugin *daemonPlugin) Prefix() string { return "/daemon" }

func (plugin *daemonPlugin) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "reload-daemon",
		Method:      http.MethodPost,
		Path:        "/reload",
		Summary:     "Reload daemon configuration",
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Reloaded bool `json:"reloaded"`
		}
	}, error) {
		if err := plugin.client.ReloadDaemon(ctx); err != nil {
			return nil, daemonError(err)
		}
		out := &struct {
			Body struct {
				Reloaded bool `json:"reloaded"`
			}
		}{}
		out.Body.Reloaded = true
		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "restart-daemon",
		Method:      http.MethodPost,
		Path:        "/restart",
		Summary:     "Restart the daemon process",
	}, func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Restarting bool `json:"restarting"`
		}
	}, error) {
		if err := plugin.client.RestartDaemon(ctx); err != nil {
			return nil, daemonError(err)
		}
		out := &struct {
			Body struct {
				Restarting bool `json:"restarting"`
			}
		}{}
		out.Body.Restarting = true
		return out, nil
	})
}
