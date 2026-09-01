package ipc

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Alex4386/komitake/internal/config"
	"github.com/Alex4386/komitake/internal/daemon"
	"github.com/Alex4386/komitake/internal/rcd"
	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func TestAdminServiceOverSocket(t *testing.T) {
	t.Parallel()

	lis := bufconn.Listen(1024 * 1024)
	defer lis.Close()

	manager := daemon.NewManager(config.Runtime{
		ServerInfo: rcd.ServerInfo{
			Name:      "Komitake",
			Ident:     make([]byte, 16),
			MasterKey: make([]byte, 64),
			Versions:  []uint8{2, 1},
		},
	})
	srv := grpc.NewServer()
	adminv1.RegisterAdminServiceServer(srv, NewDaemonService(manager))
	defer srv.Stop()
	go srv.Serve(lis)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient() error = %v", err)
	}
	defer conn.Close()

	client := adminv1.NewAdminServiceClient(conn)
	state, err := client.GetState(context.Background(), &adminv1.GetStateRequest{})
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if state.State != adminv1.State_STATE_DOWN {
		t.Fatalf("state = %v, want STATE_DOWN", state.State)
	}
}

func TestStateConversionsRoundTrip(t *testing.T) {
	t.Parallel()

	for _, st := range []daemon.State{daemon.StateDown, daemon.StateRunning, daemon.StatePairing} {
		got, err := stateFromProto(stateToProto(st))
		if err != nil {
			t.Fatalf("round trip %q error = %v", st, err)
		}
		if got != st {
			t.Fatalf("round trip %q = %q", st, got)
		}
	}

	if _, err := stateFromProto(adminv1.State_STATE_UNSPECIFIED); err == nil {
		t.Fatal("expected STATE_UNSPECIFIED to be rejected")
	}
}

func TestDriveProtoConversion(t *testing.T) {
	t.Parallel()
	updatedAt := time.Unix(123, 456).UTC()
	protoState := driveToProto(daemon.DriveState{
		DeviceID: "aabb", Steer: 0.5, Throttle: -0.25, Brake: 1, Applied: true, UpdatedAt: updatedAt,
	})
	if protoState.GetDeviceId() != "aabb" || !protoState.GetApplied() || protoState.GetSteer() != 0.5 {
		t.Fatalf("drive proto = %+v", protoState)
	}
	if protoState.GetUpdatedAt() != updatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("updated_at = %q", protoState.GetUpdatedAt())
	}
}

func TestStreamVideoOverSocket(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	defer listener.Close()
	manager := daemon.NewManager(config.Runtime{AutoStart: true, Address: "127.0.0.1"})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	manager.DeviceConnected(&rcd.Device{Name: "Fuji", Ident: []byte{0xaa, 0xbb}})
	server := grpc.NewServer()
	adminv1.RegisterAdminServiceServer(server, NewDaemonService(manager))
	defer server.Stop()
	go server.Serve(listener)
	connection, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := adminv1.NewAdminServiceClient(connection).StreamVideo(ctx, &adminv1.StreamVideoRequest{Selector: "aabb"})
	if err != nil {
		t.Fatal(err)
	}
	manager.PublishVideoFrame(daemon.VideoFrame{DeviceID: "aabb", Sequence: 7, KeyFrame: true, Data: []byte{0, 0, 0, 1, 0x65}})
	frame, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if frame.GetDeviceId() != "aabb" || frame.GetSequence() != 7 || !frame.GetKeyFrame() || len(frame.GetAnnexB()) != 5 {
		t.Fatalf("frame = %+v", frame)
	}
}
