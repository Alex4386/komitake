package web

import (
	"context"
	"net/http"

	"github.com/Alex4386/komitake/internal/config"
	"github.com/danielgtaylor/huma/v2"
)

type settingsPlugin struct {
	configPath string
}

func (plugin *settingsPlugin) Prefix() string { return "/settings" }

func (plugin *settingsPlugin) Register(api huma.API) {
	huma.Get(api, "", func(context.Context, *struct{}) (*struct {
		Body config.ServiceSettings
	}, error) {
		settings, err := config.ReadServiceSettings(plugin.configPath)
		if err != nil {
			return nil, huma.Error500InternalServerError("read config settings", err)
		}
		return &struct{ Body config.ServiceSettings }{Body: settings}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "update-service-settings",
		Method:      http.MethodPut,
		Path:        "",
		Summary:     "Update daemon service settings",
	}, func(ctx context.Context, input *struct {
		Body struct {
			Web      config.WebFile            `json:"web"`
			Socket   config.SocketFile         `json:"socket"`
			Video    config.VideoFile          `json:"video"`
			WebRTC   *config.WebRTCFile        `json:"webrtc,omitempty"`
			General  *config.GeneralSettings   `json:"general,omitempty"`
			Wireless *config.WirelessSettings  `json:"wireless,omitempty"`
		}
	}) (*struct{ Body config.ServiceSettings }, error) {
		webrtc := config.WebRTCFile{}
		if input.Body.WebRTC != nil {
			webrtc = *input.Body.WebRTC
		}
		settings, err := config.WriteServiceSettings(plugin.configPath, input.Body.Web, input.Body.Socket, input.Body.Video, webrtc, input.Body.General, input.Body.Wireless)
		if err != nil {
			return nil, huma.Error400BadRequest("invalid config settings", err)
		}
		return &struct{ Body config.ServiceSettings }{Body: settings}, nil
	})
}
