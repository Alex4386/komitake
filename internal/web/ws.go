package web

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"time"

	"github.com/Alex4386/komitake/pkg/komitake"

	"github.com/coder/websocket"
)

func registerRealtime(mux *http.ServeMux, client Client, hub *Hub) {
	mux.HandleFunc("GET /v1/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWS(w, r, client, hub)
	})
	mux.HandleFunc("GET /v1/karts/by-id/{id}/ws", func(w http.ResponseWriter, r *http.Request) {
		serveDeviceWS(w, r, client, r.PathValue("id"))
	})
}

func serveWS(w http.ResponseWriter, r *http.Request, client Client, hub *Hub) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // local operator UI; same-origin in production embed
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	ctx := r.Context()
	sub := hub.Subscribe()
	defer hub.Unsubscribe(sub)

	if snap, err := snapshotPayload(client); err == nil {
		if b, err := json.Marshal(snap); err == nil {
			_ = conn.Write(ctx, websocket.MessageText, b)
		}
	}

	errCh := make(chan error, 1)
	go func() {
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				errCh <- err
				return
			}
			handleWSClientMessage(ctx, data, client, hub)
		}
	}()

	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errCh:
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		case msg, ok := <-sub:
			if !ok {
				return
			}
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(wctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

type deviceWSMessage struct {
	messageType websocket.MessageType
	data        []byte
}

func serveDeviceWS(w http.ResponseWriter, request *http.Request, client Client, selector string) {
	kart, err := resolveKart(request.Context(), client, selector)
	if err != nil {
		writeResolveError(w, err)
		return
	}
	connection, err := websocket.Accept(w, request, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	outbound := make(chan deviceWSMessage, 64)
	writeErrors := make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case message := <-outbound:
				writeContext, writeCancel := context.WithTimeout(ctx, 5*time.Second)
				writeErr := connection.Write(writeContext, message.messageType, message.data)
				writeCancel()
				if writeErr != nil {
					writeErrors <- writeErr
					return
				}
			}
		}
	}()
	sendJSON := func(value any) bool {
		data, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return false
		}
		select {
		case outbound <- deviceWSMessage{messageType: websocket.MessageText, data: data}:
			return true
		case <-ctx.Done():
			return false
		}
	}
	if drive, driveErr := getDriveViaClient(ctx, client, kart.Ident); driveErr == nil {
		_ = sendJSON(map[string]any{"type": "drive", "drive": drive})
	}
	_ = sendJSON(map[string]any{"type": "telemetry", "telemetry": telemetryFromKart(kart)})

	readErrors := make(chan error, 1)
	go func() {
		for {
			_, data, readErr := connection.Read(ctx)
			if readErr != nil {
				readErrors <- readErr
				return
			}
			var message wsClientMsg
			if json.Unmarshal(data, &message) != nil {
				continue
			}
			switch message.Type {
			case "drive":
				state, setErr := setDriveViaClient(ctx, client, kart.Ident, DriveInput{
					Steer: message.Steer, Throttle: message.Throttle, Brake: message.Brake,
				})
				if setErr != nil {
					state.Reason = setErr.Error()
				}
				if !sendJSON(map[string]any{"type": "drive", "drive": state}) {
					return
				}
			case "drive-mode":
				updated, setErr := client.SetDriveMode(ctx, kart.Ident, message.Enabled)
				if setErr != nil {
					_ = sendJSON(map[string]any{"type": "error", "message": setErr.Error()})
					continue
				}
				if !sendJSON(map[string]any{"type": "telemetry", "telemetry": telemetryFromKart(*updated)}) {
					return
				}
				if drive, driveErr := getDriveViaClient(ctx, client, kart.Ident); driveErr == nil {
					if !sendJSON(map[string]any{"type": "drive", "drive": drive}) {
						return
					}
				}
			}
		}
	}()

	videoErrors := make(chan error)
	if request.URL.Query().Get("video") != "0" {
		go func() {
			stream, streamErr := client.StreamVideo(ctx, kart.Ident)
			if streamErr != nil {
				videoErrors <- streamErr
				return
			}
			for {
				frame, receiveErr := stream.Recv()
				if receiveErr != nil {
					videoErrors <- receiveErr
					return
				}
				select {
				case outbound <- deviceWSMessage{messageType: websocket.MessageBinary, data: encodeVideoFrame(frame)}:
				case <-ctx.Done():
					return
				}
			}
		}()

	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastTelemetry map[string]any
	var lastDrive DriveState
	haveLastDrive := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-readErrors:
			return
		case <-writeErrors:
			return
		case videoErr := <-videoErrors:
			if videoErr != nil && ctx.Err() == nil {
				_ = sendJSON(map[string]any{"type": "video-error", "message": videoErr.Error()})
			}
			return
		case <-ticker.C:
			karts, listErr := client.Karts(ctx)
			if listErr != nil {
				continue
			}
			found := false
			for _, current := range karts {
				if current.Ident != kart.Ident {
					continue
				}
				found = true
				payload := telemetryFromKart(current)
				if !reflect.DeepEqual(lastTelemetry, payload) {
					if !sendJSON(map[string]any{"type": "telemetry", "telemetry": payload}) {
						return
					}
					lastTelemetry = payload
				}
				if drive, driveErr := getDriveViaClient(ctx, client, kart.Ident); driveErr == nil {
					if !haveLastDrive || !reflect.DeepEqual(lastDrive, drive) {
						if !sendJSON(map[string]any{"type": "drive", "drive": drive}) {
							return
						}
						lastDrive = drive
						haveLastDrive = true
					}
				}
			}
			if !found {
				_ = sendJSON(map[string]any{"type": "error", "message": "device disconnected"})
				return
			}
		}
	}
}

func telemetryFromKart(kart komitake.Kart) map[string]any {
	out := map[string]any{"device_id": kart.Ident, "battery": kart.Battery, "cable_connected": kart.CableConnected, "drive_armed": kart.DriveArmed, "imu_timer_us": kart.IMUTimerUs}
	if kart.AccelMPS2 != nil {
		out["accel_mps2"] = map[string]float64{"x": kart.AccelMPS2.X, "y": kart.AccelMPS2.Y, "z": kart.AccelMPS2.Z}
	}
	if kart.GyroDPS != nil {
		out["gyro_dps"] = map[string]float64{"x": kart.GyroDPS.X, "y": kart.GyroDPS.Y, "z": kart.GyroDPS.Z}
	}
	if kart.Orientation != nil {
		out["orientation"] = map[string]float64{"i": kart.Orientation.I, "j": kart.Orientation.J, "k": kart.Orientation.K, "r": kart.Orientation.R}
	}
	return out
}

type wsClientMsg struct {
	Type     string  `json:"type"`
	DeviceID string  `json:"device_id"`
	Steer    float64 `json:"steer"`
	Throttle float64 `json:"throttle"`
	Brake    float64 `json:"brake"`
	Enabled  bool    `json:"enabled"`
}

func handleWSClientMessage(ctx context.Context, data []byte, client Client, hub *Hub) {
	var message wsClientMsg
	if err := json.Unmarshal(data, &message); err != nil {
		return
	}
	switch message.Type {
	case "drive":
		if message.DeviceID == "" {
			return
		}
		driveContext, cancel := context.WithTimeout(ctx, 2*time.Second)
		state, err := setDriveViaClient(driveContext, client, message.DeviceID, DriveInput{
			Steer: message.Steer, Throttle: message.Throttle, Brake: message.Brake,
		})
		cancel()
		if err != nil {
			state.Reason = err.Error()
		}
		hub.BroadcastJSON(map[string]any{"type": "drive", "drive": state})
	case "ping":
	}
}
