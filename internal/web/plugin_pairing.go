package web

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// pairingPlugin drives the pairing flow under /pair (mounted beneath /karts).
type pairingPlugin struct {
	client Client
}

func (p *pairingPlugin) Prefix() string { return "/pair" }

func (p *pairingPlugin) Register(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID:   "pair-kart",
		Method:        http.MethodPost,
		Path:          "",
		Summary:       "Start pairing and return the QR payload",
		DefaultStatus: http.StatusAccepted,
	}, func(ctx context.Context, in *struct {
		Body struct {
			WaitSeconds int `json:"wait_seconds,omitempty" doc:"If set, block up to this many seconds for the kart to join"`
		}
	}) (*struct{ Body pairingDTO }, error) {
		pr, err := p.client.StartPairing(ctx)
		if err != nil {
			return nil, daemonError(err)
		}
		dto, err := pairingToDTO(pr)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to build pairing payload", err)
		}

		if in.Body.WaitSeconds > 0 {
			waitCtx, cancel := context.WithTimeout(ctx, time.Duration(in.Body.WaitSeconds)*time.Second)
			defer cancel()
			if err := p.client.AwaitPairing(waitCtx); err != nil {
				return nil, huma.Error408RequestTimeout("kart did not pair in time", err)
			}
		}
		return &struct{ Body pairingDTO }{Body: dto}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID:   "stop-pairing",
		Method:        http.MethodPost,
		Path:          "/stop",
		Summary:       "Leave pairing mode",
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		if err := p.client.StopPairing(ctx); err != nil {
			return nil, daemonError(err)
		}
		return nil, nil
	})
}
