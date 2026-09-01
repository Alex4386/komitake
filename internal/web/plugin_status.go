package web

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

// statusPlugin exposes daemon status under /status.
type statusPlugin struct {
	client Client
}

func (p *statusPlugin) Prefix() string { return "/status" }

func (p *statusPlugin) Register(api huma.API) {
	huma.Get(api, "", func(ctx context.Context, _ *struct{}) (*struct {
		Body statusDTO
	}, error) {
		st, err := p.client.Status(ctx)
		if err != nil {
			return nil, daemonError(err)
		}
		return &struct{ Body statusDTO }{Body: statusToDTO(st)}, nil
	})
}
