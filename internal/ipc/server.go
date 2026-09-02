// Package ipc adapts the daemon.Manager to the generated protobuf gRPC
// AdminService served over the unix socket. The wire types live in
// proto/komitake/admin/v1 (package adminv1); this package owns the conversions
// between those and the daemon's domain types.
package ipc

import (
	"context"
	"fmt"
	"time"

	"github.com/Alex4386/komitake/internal/daemon"
	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
)

// DaemonService implements the generated AdminServiceServer.
type DaemonService struct {
	adminv1.UnimplementedAdminServiceServer
	manager *daemon.Manager
}

func NewDaemonService(manager *daemon.Manager) *DaemonService {
	return &DaemonService{manager: manager}
}

func (s *DaemonService) GetState(ctx context.Context, _ *adminv1.GetStateRequest) (*adminv1.GetStateResponse, error) {
	return &adminv1.GetStateResponse{
		State:    stateToProto(s.manager.CurrentState()),
		Pairing:  pairingToProto(s.manager.CurrentPairing()),
		Wireless: wirelessToProto(s.manager.CurrentWireless()),
	}, nil
}

func (s *DaemonService) SetState(ctx context.Context, req *adminv1.SetStateRequest) (*adminv1.SetStateResponse, error) {
	state, err := stateFromProto(req.GetState())
	if err != nil {
		return nil, err
	}
	if err := s.manager.SetState(ctx, state); err != nil {
		return nil, err
	}
	return &adminv1.SetStateResponse{
		State:    stateToProto(s.manager.CurrentState()),
		Pairing:  pairingToProto(s.manager.CurrentPairing()),
		Wireless: wirelessToProto(s.manager.CurrentWireless()),
	}, nil
}

func (s *DaemonService) GetPairingInfo(ctx context.Context, _ *adminv1.GetPairingInfoRequest) (*adminv1.GetPairingInfoResponse, error) {
	return &adminv1.GetPairingInfoResponse{Pairing: pairingToProto(s.manager.CurrentPairing())}, nil
}

func (s *DaemonService) ListDevices(ctx context.Context, _ *adminv1.ListDevicesRequest) (*adminv1.ListDevicesResponse, error) {
	devices := s.manager.ListDevices()
	out := make([]*adminv1.DeviceSummary, 0, len(devices))
	for _, d := range devices {
		out = append(out, deviceToProto(d))
	}
	return &adminv1.ListDevicesResponse{Devices: out}, nil
}

func (s *DaemonService) WaitForDevice(ctx context.Context, req *adminv1.WaitForDeviceRequest) (*adminv1.WaitForDeviceResponse, error) {
	device, err := s.manager.WaitForDevice(ctx, req.GetIdent())
	if err != nil {
		return nil, err
	}
	return &adminv1.WaitForDeviceResponse{Device: deviceToProto(device)}, nil
}

func (s *DaemonService) GetDeviceParam(ctx context.Context, req *adminv1.GetDeviceParamRequest) (*adminv1.GetDeviceParamResponse, error) {
	device, value, err := s.manager.GetDeviceParam(ctx, req.GetSelector(), req.GetName())
	if err != nil {
		return nil, err
	}
	return &adminv1.GetDeviceParamResponse{
		Device: deviceToProto(device),
		Value:  value,
	}, nil
}

func (s *DaemonService) GetProductCode(ctx context.Context, req *adminv1.GetProductCodeRequest) (*adminv1.GetProductCodeResponse, error) {
	device, pc, err := s.manager.GetProductCode(ctx, req.GetSelector())
	if err != nil {
		return nil, err
	}
	return &adminv1.GetProductCodeResponse{
		Device: deviceToProto(device),
		ProductCode: &adminv1.ProductCode{
			Unk1:      uint32(pc.Unk1),
			Character: uint32(pc.Character),
			Unk2:      uint32(pc.Unk2),
			Serial:    pc.Serial,
		},
	}, nil
}

func (service *DaemonService) SetDrive(
	ctx context.Context,
	request *adminv1.SetDriveRequest,
) (*adminv1.SetDriveResponse, error) {
	state, err := service.manager.SetDrive(
		request.GetSelector(),
		request.GetSteer(),
		request.GetThrottle(),
		request.GetBrake(),
	)
	if err != nil {
		return nil, err
	}
	return &adminv1.SetDriveResponse{Drive: driveToProto(state)}, nil
}

func (service *DaemonService) GetDrive(
	ctx context.Context,
	request *adminv1.GetDriveRequest,
) (*adminv1.GetDriveResponse, error) {
	state, err := service.manager.GetDrive(request.GetSelector())
	if err != nil {
		return nil, err
	}
	return &adminv1.GetDriveResponse{Drive: driveToProto(state)}, nil
}

func (service *DaemonService) SetDriveMode(
	ctx context.Context,
	request *adminv1.SetDriveModeRequest,
) (*adminv1.SetDriveModeResponse, error) {
	device, err := service.manager.SetDriveMode(ctx, request.GetSelector(), request.GetEnabled())
	if err != nil {
		return nil, err
	}
	return &adminv1.SetDriveModeResponse{Device: deviceToProto(device)}, nil
}

func (service *DaemonService) ShutdownKart(
	ctx context.Context,
	request *adminv1.ShutdownKartRequest,
) (*adminv1.ShutdownKartResponse, error) {
	device, err := service.manager.ShutdownKart(ctx, request.GetSelector())
	if err != nil {
		return nil, err
	}
	return &adminv1.ShutdownKartResponse{Device: deviceToProto(device)}, nil
}

func (service *DaemonService) StreamVideo(
	request *adminv1.StreamVideoRequest,
	stream adminv1.AdminService_StreamVideoServer,
) error {
	frames, _, err := service.manager.StreamVideoWithOptions(stream.Context(), request.GetSelector(), daemon.VideoStreamOptions{FreshKeyFrame: request.GetFreshKeyFrame()})
	if err != nil {
		return err
	}
	for frame := range frames {
		if err := stream.Send(&adminv1.StreamVideoResponse{
			DeviceId:      frame.DeviceID,
			Sequence:      frame.Sequence,
			Metadata_0:    frame.Metadata0,
			Metadata_1:    frame.Metadata1,
			Metadata_2:    frame.Metadata2,
			KeyFrame:      frame.KeyFrame,
			AnnexB:        append([]byte(nil), frame.Data...),
			Discontinuity: frame.Discontinuity,
		}); err != nil {
			return err
		}
	}
	return stream.Context().Err()
}

func (s *DaemonService) ReloadDaemon(ctx context.Context, _ *adminv1.ReloadDaemonRequest) (*adminv1.ReloadDaemonResponse, error) {
	state, err := s.manager.Reload(ctx)
	if err != nil {
		return nil, err
	}
	return &adminv1.ReloadDaemonResponse{State: stateToProto(state)}, nil
}

func (s *DaemonService) RestartDaemon(ctx context.Context, _ *adminv1.RestartDaemonRequest) (*adminv1.RestartDaemonResponse, error) {
	s.manager.RequestRestart()
	return &adminv1.RestartDaemonResponse{}, nil
}

func driveToProto(state daemon.DriveState) *adminv1.DriveState {
	updatedAt := ""
	if !state.UpdatedAt.IsZero() {
		updatedAt = state.UpdatedAt.Format(time.RFC3339Nano)
	}
	return &adminv1.DriveState{
		DeviceId:  state.DeviceID,
		Steer:     state.Steer,
		Throttle:  state.Throttle,
		Brake:     state.Brake,
		Applied:   state.Applied,
		UpdatedAt: updatedAt,
		Reason:    state.Reason,
	}
}

// stateToProto maps the daemon state string to the proto enum.
func stateToProto(state daemon.State) adminv1.State {
	switch state {
	case daemon.StateDown:
		return adminv1.State_STATE_DOWN
	case daemon.StateRunning:
		return adminv1.State_STATE_RUNNING
	case daemon.StatePairing:
		return adminv1.State_STATE_PAIRING
	default:
		return adminv1.State_STATE_UNSPECIFIED
	}
}

// stateFromProto maps the proto enum back to a daemon state, rejecting values
// the daemon cannot act on.
func stateFromProto(state adminv1.State) (daemon.State, error) {
	switch state {
	case adminv1.State_STATE_DOWN:
		return daemon.StateDown, nil
	case adminv1.State_STATE_RUNNING:
		return daemon.StateRunning, nil
	case adminv1.State_STATE_PAIRING:
		return daemon.StatePairing, nil
	default:
		return "", fmt.Errorf("unknown state %q", state)
	}
}

func pairingToProto(p *daemon.PairingRecord) *adminv1.PairingRecord {
	if p == nil {
		return nil
	}
	return &adminv1.PairingRecord{
		State:       stateToProto(p.State),
		SeedHex:     p.SeedHex,
		Ssid:        p.SSID,
		Channel:     uint32(p.Channel),
		GeneratedAt: p.GeneratedAt,
		FilePath:    p.FilePath,
	}
}

func wirelessToProto(w *daemon.WirelessInfo) *adminv1.WirelessInfo {
	if w == nil {
		return nil
	}
	return &adminv1.WirelessInfo{
		Interface:   w.Interface,
		Address:     w.Address,
		Subnet:      w.Subnet,
		Channel:     uint32(w.Channel),
		Ssid:        w.SSID,
		HostapdPath: w.HostapdPath,
	}
}

func deviceToProto(d daemon.DeviceSummary) *adminv1.DeviceSummary {
	out := &adminv1.DeviceSummary{
		Kind:       d.Kind,
		Ident:      d.Ident,
		Serial:     d.Serial,
		Address:    d.Address,
		MacAddress: d.MACAddress,
	}
	if d.SignalDBM != nil {
		v := int32(*d.SignalDBM)
		out.SignalDbm = &v
	}
	if d.Battery != nil {
		v := int32(*d.Battery)
		out.Battery = &v
	}
	if d.CableConnected != nil {
		v := *d.CableConnected
		out.CableConnected = &v
	}
	out.DriveArmed = d.DriveArmed
	if d.IMU != nil {
		out.AccelMps2 = &adminv1.Vec3{X: d.IMU.Accel.X, Y: d.IMU.Accel.Y, Z: d.IMU.Accel.Z}
		out.GyroDps = &adminv1.Vec3{X: d.IMU.Gyro.X, Y: d.IMU.Gyro.Y, Z: d.IMU.Gyro.Z}
		out.Orientation = &adminv1.Quat{I: d.IMU.Orientation.I, J: d.IMU.Orientation.J, K: d.IMU.Orientation.K, R: d.IMU.Orientation.R}
		timer := d.IMU.TimerUs
		out.ImuTimerUs = &timer
	}
	return out
}
