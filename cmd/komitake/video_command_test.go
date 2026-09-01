package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestVideoCommandWritesAnnexBFramesToPlayer(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "video.h264")
	adminDial = func(context.Context, endpoint) (adminv1.AdminServiceClient, adminCloser, error) {
		return videoAdminClient{frames: []*adminv1.StreamVideoResponse{{AnnexB: []byte{0, 0, 0, 1, 0x65, 0, 0, 0, 1, 9, 0x30}}, {AnnexB: []byte{0, 0, 1, 0x41}}}}, noopCloser{}, nil
	}
	videoPlayerCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `cat > "$VIDEO_TEST_OUTPUT"`)
	}
	t.Setenv("VIDEO_TEST_OUTPUT", outputPath)
	t.Cleanup(func() {
		adminDial = dialAdmin
		videoPlayerCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, name, args...)
		}
	})
	command := New()
	command.SetArgs([]string{"video", "XKW123", "--player", "fake-player"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{0, 0, 0, 1, 0x65, 0, 0, 1, 0x41}
	if string(data) != string(want) {
		t.Fatalf("player input = %x, want %x", data, want)
	}
}

type videoAdminClient struct {
	stubAdminClient
	frames   []*adminv1.StreamVideoResponse
	selector string
}

func (client videoAdminClient) StreamVideo(_ context.Context, request *adminv1.StreamVideoRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[adminv1.StreamVideoResponse], error) {
	client.selector = request.GetSelector()
	return &fakeGRPCVideoStream{frames: client.frames}, nil
}

type fakeGRPCVideoStream struct {
	frames []*adminv1.StreamVideoResponse
}

func (stream *fakeGRPCVideoStream) Recv() (*adminv1.StreamVideoResponse, error) {
	if len(stream.frames) == 0 {
		return nil, io.EOF
	}
	frame := stream.frames[0]
	stream.frames = stream.frames[1:]
	return frame, nil
}
func (*fakeGRPCVideoStream) Header() (metadata.MD, error) { return nil, nil }
func (*fakeGRPCVideoStream) Trailer() metadata.MD         { return nil }
func (*fakeGRPCVideoStream) CloseSend() error             { return nil }
func (*fakeGRPCVideoStream) Context() context.Context     { return context.Background() }
func (*fakeGRPCVideoStream) SendMsg(any) error            { return nil }
func (*fakeGRPCVideoStream) RecvMsg(any) error            { return io.EOF }
