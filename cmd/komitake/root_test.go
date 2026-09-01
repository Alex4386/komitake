package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"google.golang.org/grpc"
)

type stubAdminClient struct{}
type noopCloser struct{}

func (noopCloser) Close() error { return nil }

func (stubAdminClient) GetState(context.Context, *adminv1.GetStateRequest, ...grpc.CallOption) (*adminv1.GetStateResponse, error) {
	return &adminv1.GetStateResponse{State: adminv1.State_STATE_RUNNING}, nil
}

func (stubAdminClient) SetState(context.Context, *adminv1.SetStateRequest, ...grpc.CallOption) (*adminv1.SetStateResponse, error) {
	return &adminv1.SetStateResponse{State: adminv1.State_STATE_PAIRING, Pairing: &adminv1.PairingRecord{
		State:   adminv1.State_STATE_PAIRING,
		SeedHex: strings.Repeat("ab", 16),
		Ssid:    "Ptest",
	}}, nil
}

func (stubAdminClient) GetPairingInfo(context.Context, *adminv1.GetPairingInfoRequest, ...grpc.CallOption) (*adminv1.GetPairingInfoResponse, error) {
	return &adminv1.GetPairingInfoResponse{Pairing: &adminv1.PairingRecord{
		State:   adminv1.State_STATE_PAIRING,
		SeedHex: strings.Repeat("ab", 16),
		Ssid:    "Ptest",
	}}, nil
}

func (stubAdminClient) ListDevices(context.Context, *adminv1.ListDevicesRequest, ...grpc.CallOption) (*adminv1.ListDevicesResponse, error) {
	return &adminv1.ListDevicesResponse{Devices: []*adminv1.DeviceSummary{{Ident: "abc123", Serial: "XKW123", Kind: "Fuji"}}}, nil
}

func (stubAdminClient) WaitForDevice(context.Context, *adminv1.WaitForDeviceRequest, ...grpc.CallOption) (*adminv1.WaitForDeviceResponse, error) {
	return &adminv1.WaitForDeviceResponse{Device: &adminv1.DeviceSummary{Ident: "abc123", Serial: "XKW123", Kind: "Fuji"}}, nil
}

func (stubAdminClient) GetDeviceParam(context.Context, *adminv1.GetDeviceParamRequest, ...grpc.CallOption) (*adminv1.GetDeviceParamResponse, error) {
	return &adminv1.GetDeviceParamResponse{
		Device: &adminv1.DeviceSummary{Ident: "abc123", Serial: "XKW123", Kind: "Fuji"},
		Value:  []byte("x"),
	}, nil
}

func (stubAdminClient) GetProductCode(context.Context, *adminv1.GetProductCodeRequest, ...grpc.CallOption) (*adminv1.GetProductCodeResponse, error) {
	return &adminv1.GetProductCodeResponse{
		Device:      &adminv1.DeviceSummary{Ident: "abc123", Serial: "XKW123", Kind: "Fuji"},
		ProductCode: &adminv1.ProductCode{Serial: "XKW123"},
	}, nil
}

func (stubAdminClient) SetDrive(
	context.Context,
	*adminv1.SetDriveRequest,
	...grpc.CallOption,
) (*adminv1.SetDriveResponse, error) {
	return &adminv1.SetDriveResponse{}, nil
}

func (stubAdminClient) GetDrive(
	context.Context,
	*adminv1.GetDriveRequest,
	...grpc.CallOption,
) (*adminv1.GetDriveResponse, error) {
	return &adminv1.GetDriveResponse{}, nil
}

func (stubAdminClient) SetDriveMode(
	context.Context,
	*adminv1.SetDriveModeRequest,
	...grpc.CallOption,
) (*adminv1.SetDriveModeResponse, error) {
	return &adminv1.SetDriveModeResponse{}, nil
}

func (stubAdminClient) ShutdownKart(
	context.Context,
	*adminv1.ShutdownKartRequest,
	...grpc.CallOption,
) (*adminv1.ShutdownKartResponse, error) {
	return &adminv1.ShutdownKartResponse{}, nil
}

func (stubAdminClient) StreamVideo(
	context.Context,
	*adminv1.StreamVideoRequest,
	...grpc.CallOption,
) (grpc.ServerStreamingClient[adminv1.StreamVideoResponse], error) {
	return nil, nil
}

// pairingAdminClient reports pairing mode with a populated record, so status
// output covering the pairing fields can be exercised.
type pairingAdminClient struct{ stubAdminClient }

func (pairingAdminClient) GetState(context.Context, *adminv1.GetStateRequest, ...grpc.CallOption) (*adminv1.GetStateResponse, error) {
	return &adminv1.GetStateResponse{
		State: adminv1.State_STATE_PAIRING,
		Pairing: &adminv1.PairingRecord{
			State:   adminv1.State_STATE_PAIRING,
			SeedHex: strings.Repeat("ab", 16),
			Ssid:    "Ptest",
			Channel: 6,
		},
	}, nil
}

func TestDevicesCommand(t *testing.T) {
	adminDial = func(context.Context, endpoint) (adminv1.AdminServiceClient, adminCloser, error) {
		return stubAdminClient{}, noopCloser{}, nil
	}
	t.Cleanup(func() { adminDial = dialAdmin })

	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"devices"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "XKW123") {
		t.Fatalf("output = %q", got)
	}
	if strings.Contains(got, "abc123") {
		t.Fatalf("ident should be hidden without -l: %q", got)
	}

	cmd = New()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"devices", "-l"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(-l) error = %v", err)
	}
	if !strings.Contains(out.String(), "abc123") {
		t.Fatalf("-l output = %q", out.String())
	}
}

func TestStatusCommandPrintsModeName(t *testing.T) {
	adminDial = func(context.Context, endpoint) (adminv1.AdminServiceClient, adminCloser, error) {
		return stubAdminClient{}, noopCloser{}, nil
	}
	t.Cleanup(func() { adminDial = dialAdmin })

	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(out.String()) != "normal" {
		t.Fatalf("output = %q", out.String())
	}
}

// The pairing seed is equivalent to the network key, so it must stay hidden
// unless explicitly requested.
func TestStatusCommandHidesSeedByDefault(t *testing.T) {
	adminDial = func(context.Context, endpoint) (adminv1.AdminServiceClient, adminCloser, error) {
		return pairingAdminClient{}, noopCloser{}, nil
	}
	t.Cleanup(func() { adminDial = dialAdmin })

	seed := strings.Repeat("ab", 16)

	cmd := New()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"status"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.Contains(out.String(), seed) {
		t.Fatalf("seed leaked without --show-secrets: %q", out.String())
	}

	cmd = New()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"status", "--show-secrets"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), seed) {
		t.Fatalf("--show-secrets did not print the seed: %q", out.String())
	}
}
