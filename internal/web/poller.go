package web

import (
	"context"
	"reflect"
	"time"

	"github.com/Alex4386/komitake/pkg/komitake"
)

// Poller watches the daemon and pushes status/device diffs over the Hub.
type Poller struct {
	client Client
	hub    *Hub
	every  time.Duration
}

func NewPoller(client Client, hub *Hub) *Poller {
	return &Poller{client: client, hub: hub, every: 500 * time.Millisecond}
}

func (p *Poller) Run(ctx context.Context) {
	ticker := time.NewTicker(p.every)
	defer ticker.Stop()

	var (
		lastStatus *komitake.Status
		lastKarts  []komitake.Kart
	)

	push := func() {
		st, err := p.client.Status(ctx)
		if err != nil {
			p.hub.BroadcastJSON(map[string]any{"type": "error", "message": err.Error()})
			return
		}
		karts, err := p.client.Karts(ctx)
		if err != nil {
			p.hub.BroadcastJSON(map[string]any{"type": "error", "message": err.Error()})
			return
		}

		if !statusEqual(lastStatus, st) {
			p.hub.BroadcastJSON(map[string]any{"type": "status", "status": statusToDTO(st)})
			lastStatus = st
		}
		if !kartsEqual(lastKarts, karts) {
			devices := make([]kartDTO, 0, len(karts))
			for _, k := range karts {
				devices = append(devices, kartToDTO(k))
			}
			p.hub.BroadcastJSON(map[string]any{"type": "devices", "devices": devices})
			lastKarts = karts
		}
	}

	push()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			push()
		}
	}
}

func statusEqual(a, b *komitake.Status) bool {
	return reflect.DeepEqual(a, b)
}

func kartsEqual(a, b []komitake.Kart) bool {
	return reflect.DeepEqual(a, b)
}

func snapshotPayload(client Client) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	st, err := client.Status(ctx)
	if err != nil {
		return nil, err
	}
	karts, err := client.Karts(ctx)
	if err != nil {
		return nil, err
	}
	devices := make([]kartDTO, 0, len(karts))
	driveStates := make([]DriveState, 0, len(karts))
	for _, kart := range karts {
		devices = append(devices, kartToDTO(kart))
		if drive, driveErr := getDriveViaClient(ctx, client, kart.Ident); driveErr == nil {
			driveStates = append(driveStates, drive)
		}
	}
	return map[string]any{
		"type":    "snapshot",
		"status":  statusToDTO(st),
		"devices": devices,
		"drive":   driveStates,
	}, nil
}
