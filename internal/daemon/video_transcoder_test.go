package daemon

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Alex4386/komitake/internal/config"
)

func TestFFmpegVideoArgumentsStayHardwareOnlyAndPeriodicIDR(t *testing.T) {
	profile := VideoProfile{
		Backend:    config.VideoHwaccelVAAPI,
		Encoder:    "h264_vaapi",
		RenderNode: "/dev/dri/renderD128",
		FFmpegPath: "ffmpeg",
	}
	arguments := ffmpegVideoArguments(profile)
	wanted := [][2]string{{"-loglevel", "verbose"}, {"-hwaccel", "vaapi"}, {"-hwaccel_output_format", "vaapi"}, {"-c:v", "h264_vaapi"}, {"-low_power", "1"}, {"-async_depth", "1"}, {"-g", "10"}, {"-bf", "0"}, {"-flush_packets", "1"}}
	for _, pair := range wanted {
		if !containsArgumentPair(arguments, pair[0], pair[1]) {
			t.Fatalf("arguments lack %q %q: %v", pair[0], pair[1], arguments)
		}
	}
	for _, argument := range arguments {
		if argument == "libx264" {
			t.Fatalf("software encoder in arguments: %v", arguments)
		}
	}
}

func TestReadFFmpegAccessUnitsSplitsAUDDelimitedFrames(t *testing.T) {
	stream := append([]byte{}, fakeFFmpegAccessUnit(true)...)
	stream = append(stream, fakeFFmpegAccessUnit(false)...)
	stream = append(stream, fakeFFmpegAccessUnit(false)...)
	frames := make([][]byte, 0, 3)
	idrs := 0
	err := readFFmpegAccessUnits(bytes.NewReader(stream), func(frame []byte) error {
		frames = append(frames, append([]byte(nil), frame...))
		if containsNALType(frame, 5) {
			idrs++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 || idrs != 1 {
		t.Fatalf("frames=%d idrs=%d", len(frames), idrs)
	}
	for _, frame := range frames {
		if findAUDStart(frame, 0) != 0 {
			t.Fatal("frame does not begin with FFmpeg AUD")
		}
	}
}

func fakeFFmpegAccessUnit(keyFrame bool) []byte {
	nalType := byte(1)
	if keyFrame {
		nalType = 5
	}
	return []byte{0, 0, 0, 1, 9, 0x10, 0, 0, 0, 1, 0x60 | nalType, 0x80}
}

func containsArgumentPair(arguments []string, key, value string) bool {
	for index := 0; index+1 < len(arguments); index++ {
		if arguments[index] == key && arguments[index+1] == value {
			return true
		}
	}
	return false
}

func TestPersistentTranscoderFeedsLateSubscriberFromPeriodicIDRGOP(t *testing.T) {
	originalEncoders := listFFmpegEncoders
	originalRenderNodes := listVARenderNodes
	listFFmpegEncoders = func(string) (map[string]bool, error) { return map[string]bool{"h264_vaapi": true}, nil }
	listVARenderNodes = func() ([]string, error) { return []string{"/dev/dri/renderD128"}, nil }
	t.Cleanup(func() {
		listFFmpegEncoders = originalEncoders
		listVARenderNodes = originalRenderNodes
	})

	original := ffmpegVideoCommand
	ffmpegVideoCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", `cat >/dev/null; printf '\000\000\000\001\011\020\000\000\000\001\145\200\000\000\000\001\011\020\000\000\000\001\141\200'`)
	}
	t.Cleanup(func() { ffmpegVideoCommand = original })
	hub := newVideoHub()
	profile := VideoProfile{
		Backend:    config.VideoHwaccelVAAPI,
		Encoder:    "h264_vaapi",
		RenderNode: "/dev/dri/renderD128",
		FFmpegPath: "ffmpeg",
	}
	transcoder, err := startVideoTranscoder(context.Background(), "kart", hub, slog.New(slog.NewTextHandler(io.Discard, nil)), profile)
	if err != nil {
		t.Fatal(err)
	}
	if err := transcoder.writeFrame([]byte{1}); err != nil {
		t.Fatal(err)
	}
	_ = transcoder.stdin.Close()
	select {
	case <-transcoder.exited:
	case <-time.After(time.Second):
		t.Fatal("transcoder did not finish")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	frames := hub.subscribe(ctx, "kart", false)
	select {
	case frame := <-frames:
		if !frame.KeyFrame {
			t.Fatalf("late subscriber first frame=%+v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("late subscriber did not receive cached IDR")
	}
}

func TestTranscoderStartupLogIdentifiesVAAPIAndNoSoftwareFallback(t *testing.T) {
	original := ffmpegVideoCommand
	ffmpegVideoCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "sh", "-c", "cat >/dev/null")
	}
	t.Cleanup(func() { ffmpegVideoCommand = original })
	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelInfo}))
	profile := VideoProfile{
		Backend:    config.VideoHwaccelVAAPI,
		Encoder:    "h264_vaapi",
		RenderNode: "/dev/dri/renderD128",
		FFmpegPath: "ffmpeg",
	}
	transcoder, err := startVideoTranscoder(context.Background(), "kart", newVideoHub(), logger, profile)
	if err != nil {
		t.Fatal(err)
	}
	transcoder.close()
	logText := output.String()
	for _, wanted := range []string{"starting video transcoder", "encoder=h264_vaapi", "hwaccel=vaapi", "software_fallback=false"} {
		if !strings.Contains(logText, wanted) {
			t.Fatalf("log missing %q: %s", wanted, logText)
		}
	}
}
