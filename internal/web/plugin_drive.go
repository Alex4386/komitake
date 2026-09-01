package web

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type drivePlugin struct{ client Client }

func (plugin *drivePlugin) Prefix() string { return "" }

func (plugin *drivePlugin) Register(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "get-kart-drive", Method: http.MethodGet, Path: "/by-id/{id}/drive", Summary: "Get last drive command"}, func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*struct{ Body DriveState }, error) {
		kart, err := resolveKart(ctx, plugin.client, input.ID)
		if err != nil {
			return nil, err
		}
		state, err := getDriveViaClient(ctx, plugin.client, kart.Ident)
		if err != nil {
			return nil, daemonError(err)
		}
		return &struct{ Body DriveState }{Body: state}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "set-kart-drive", Method: http.MethodPost, Path: "/by-id/{id}/drive", Summary: "Set drive command"}, func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body DriveInput
	}) (*struct{ Body DriveState }, error) {
		kart, err := resolveKart(ctx, plugin.client, input.ID)
		if err != nil {
			return nil, err
		}
		state, err := setDriveViaClient(ctx, plugin.client, kart.Ident, input.Body)
		if err != nil {
			return nil, daemonError(err)
		}
		return &struct{ Body DriveState }{Body: state}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "set-kart-drive-mode", Method: http.MethodPut, Path: "/by-id/{id}/drive-mode", Summary: "Enable or disable drive mode"}, func(ctx context.Context, input *struct {
		ID   string `path:"id"`
		Body struct {
			Enabled bool `json:"enabled"`
		}
	}) (*struct{ Body kartDTO }, error) {
		kart, err := resolveKart(ctx, plugin.client, input.ID)
		if err != nil {
			return nil, err
		}
		updated, err := plugin.client.SetDriveMode(ctx, kart.Ident, input.Body.Enabled)
		if err != nil {
			return nil, daemonError(err)
		}
		return &struct{ Body kartDTO }{Body: kartToDTO(*updated)}, nil
	})
	huma.Register(api, huma.Operation{OperationID: "shutdown-kart", Method: http.MethodPost, Path: "/by-id/{id}/shutdown", Summary: "Power off a connected kart"}, func(ctx context.Context, input *struct {
		ID string `path:"id"`
	}) (*struct{ Body kartDTO }, error) {
		kart, err := resolveKart(ctx, plugin.client, input.ID)
		if err != nil {
			return nil, err
		}
		updated, err := plugin.client.ShutdownKart(ctx, kart.Ident)
		if err != nil {
			return nil, daemonError(err)
		}
		return &struct{ Body kartDTO }{Body: kartToDTO(*updated)}, nil
	})
}
