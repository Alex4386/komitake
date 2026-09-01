package web

import (
	"context"
	"math"
	"time"

	"github.com/Alex4386/komitake/pkg/komitake"
)

type DriveInput struct {
	Steer    float64 `json:"steer"`
	Throttle float64 `json:"throttle"`
	Brake    float64 `json:"brake"`
}

type DriveState struct {
	DeviceID  string    `json:"device_id"`
	Steer     float64   `json:"steer"`
	Throttle  float64   `json:"throttle"`
	Brake     float64   `json:"brake"`
	Applied   bool      `json:"applied"`
	Reason    string    `json:"reason,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

func clamp(value, minimum, maximum float64) float64 {
	return math.Min(maximum, math.Max(minimum, value))
}

func driveFromSDK(state *komitake.DriveState) DriveState {
	if state == nil {
		return DriveState{}
	}
	return DriveState{
		DeviceID:  state.DeviceID,
		Steer:     state.Steer,
		Throttle:  state.Throttle,
		Brake:     state.Brake,
		Applied:   state.Applied,
		Reason:    state.Reason,
		UpdatedAt: state.UpdatedAt,
	}
}

func setDriveViaClient(ctx context.Context, client Client, deviceID string, input DriveInput) (DriveState, error) {
	input.Steer = clamp(input.Steer, -1, 1)
	input.Throttle = clamp(input.Throttle, -1, 1)
	input.Brake = clamp(input.Brake, 0, 1)
	state, err := client.SetDrive(ctx, deviceID, input.Steer, input.Throttle, input.Brake)
	if err != nil {
		return DriveState{DeviceID: deviceID, Steer: input.Steer, Throttle: input.Throttle, Brake: input.Brake, Reason: err.Error()}, err
	}
	return driveFromSDK(state), nil
}

func getDriveViaClient(ctx context.Context, client Client, deviceID string) (DriveState, error) {
	state, err := client.GetDrive(ctx, deviceID)
	if err != nil {
		return DriveState{DeviceID: deviceID, Reason: err.Error()}, err
	}
	out := driveFromSDK(state)
	if out.DeviceID == "" {
		out.DeviceID = deviceID
	}
	return out, nil
}
