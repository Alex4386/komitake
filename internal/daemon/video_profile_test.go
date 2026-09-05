package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Alex4386/komitake/internal/config"
)

func testFFmpegPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte{0x7f, 'E', 'L', 'F'}, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func stubVideoDiscovery(encoders map[string]bool, renderNodes []string) func() {
	originalEncoders := listFFmpegEncoders
	originalRenderNodes := listVARenderNodes
	listFFmpegEncoders = func(string) (map[string]bool, error) { return encoders, nil }
	listVARenderNodes = func() ([]string, error) { return renderNodes, nil }
	return func() {
		listFFmpegEncoders = originalEncoders
		listVARenderNodes = originalRenderNodes
	}
}

func TestResolveVideoProfileAutoPrefersVAAPI(t *testing.T) {
	defer stubVideoDiscovery(map[string]bool{"h264_vaapi": true, "h264_nvenc": true}, []string{"/dev/dri/renderD128"})()

	profile, err := ResolveVideoProfile(config.VideoFile{Hwaccel: "auto", FFmpegPath: testFFmpegPath(t)})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Backend != config.VideoHwaccelVAAPI || profile.Encoder != "h264_vaapi" || profile.RenderNode != "/dev/dri/renderD128" {
		t.Fatalf("profile = %+v", profile)
	}
}

func TestResolveVideoProfileAutoFallsBackToNVENC(t *testing.T) {
	defer stubVideoDiscovery(map[string]bool{"h264_nvenc": true}, nil)()

	profile, err := ResolveVideoProfile(config.VideoFile{Hwaccel: "auto", FFmpegPath: testFFmpegPath(t)})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Backend != config.VideoHwaccelNVENC || profile.Encoder != "h264_nvenc" {
		t.Fatalf("profile = %+v", profile)
	}
}

func TestResolveVideoProfileExplicitVAAPIRequiresRenderNode(t *testing.T) {
	defer stubVideoDiscovery(map[string]bool{"h264_vaapi": true}, nil)()

	if _, err := ResolveVideoProfile(config.VideoFile{Hwaccel: "vaapi", FFmpegPath: testFFmpegPath(t)}); err == nil {
		t.Fatal("expected missing render node error")
	}
}

func TestFFmpegVideoArgumentsRealtimeProfile(t *testing.T) {
	profile := VideoProfile{
		Backend:    config.VideoHwaccelVAAPI,
		Encoder:    "h264_vaapi",
		RenderNode: "/dev/dri/renderD128",
		Video: config.VideoFile{
			Hwaccel:       "vaapi",
			FFmpegProfile: "realtime",
		},
	}
	arguments := ffmpegVideoArguments(profile)
	for _, pair := range [][2]string{
		{"-fflags", "nobuffer"},
		{"-flags", "low_delay"},
		{"-low_delay", "1"},
		{"-low_power", "1"},
	} {
		if !containsArgumentPair(arguments, pair[0], pair[1]) {
			t.Fatalf("arguments lack %q %q: %v", pair[0], pair[1], arguments)
		}
	}
}

func TestFFmpegVideoArgumentsProfileOverriddenByCustomArgs(t *testing.T) {
	profile := VideoProfile{
		Backend:    config.VideoHwaccelNVENC,
		Encoder:    "h264_nvenc",
		Video: config.VideoFile{
			Hwaccel:       "nvenc",
			FFmpegProfile: "realtime",
			FFmpegArgs: config.VideoFFmpegArgsFile{
				Output: []string{"-preset", "p4"},
			},
		},
	}
	arguments := ffmpegVideoArguments(profile)
	if lastArgumentValue(arguments, "-preset") != "p4" {
		t.Fatalf("ffmpeg_args should override profile preset: %v", arguments)
	}
}

func TestFFmpegVideoArgumentsOverrideBuiltInHwaccel(t *testing.T) {
	profile := VideoProfile{
		Backend:    config.VideoHwaccelVAAPI,
		Encoder:    "h264_vaapi",
		RenderNode: "/dev/dri/renderD128",
		FFmpegPath: "ffmpeg",
		Video: config.VideoFile{
			Hwaccel: "vaapi",
			FFmpegArgs: config.VideoFFmpegArgsFile{
				Input:  []string{"-fflags", "nobuffer"},
				Output: []string{"-qp", "20"},
			},
		},
	}
	arguments := ffmpegVideoArguments(profile)
	for _, pair := range [][2]string{
		{"-init_hw_device", "vaapi=va:/dev/dri/renderD128"},
		{"-hwaccel", "vaapi"},
		{"-fflags", "nobuffer"},
		{"-i", "pipe:0"},
		{"-c:v", "h264_vaapi"},
		{"-low_power", "1"},
		{"-bsf:v", "h264_mp4toannexb,h264_metadata=aud=insert"},
		{"-qp", "20"},
	} {
		if !containsArgumentPair(arguments, pair[0], pair[1]) {
			t.Fatalf("arguments lack %q %q: %v", pair[0], pair[1], arguments)
		}
	}
	if indexPair(arguments, "-qp", "24") >= indexPair(arguments, "-qp", "20") {
		t.Fatalf("override qp should follow built-in qp: %v", arguments)
	}
}

func TestResolveVideoProfileCustomUsesConfiguredArgs(t *testing.T) {
	profile, err := ResolveVideoProfile(config.VideoFile{
		Hwaccel:    "custom",
		FFmpegPath: testFFmpegPath(t),
		FFmpegArgs: config.VideoFFmpegArgsFile{
			Input:  []string{"-hwaccel", "auto", "-f", "h264", "-i", "pipe:0", "-an"},
			Output: []string{"-c:v", "h264_nvenc", "-f", "h264", "pipe:1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Backend != config.VideoHwaccelCustom {
		t.Fatalf("backend = %q", profile.Backend)
	}
	arguments := ffmpegVideoArguments(profile)
	if !containsArgumentPair(arguments, "-hwaccel", "auto") || !containsArgumentPair(arguments, "-c:v", "h264_nvenc") {
		t.Fatalf("arguments = %v", arguments)
	}
	for _, argument := range arguments {
		if argument == "h264_vaapi" {
			t.Fatalf("built-in vaapi encoder leaked into custom args: %v", arguments)
		}
	}
}

func TestResolveVideoProfileNone(t *testing.T) {
	defer stubVideoDiscovery(map[string]bool{"libx264": true}, nil)()
	ffmpegPath := testFFmpegPath(t)

	profile, err := ResolveVideoProfile(config.VideoFile{Hwaccel: "none", FFmpegPath: ffmpegPath})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Backend != config.VideoHwaccelNone || profile.Encoder != "libx264" || profile.FFmpegPath != ffmpegPath {
		t.Fatalf("profile = %+v", profile)
	}
}

func TestFFmpegVideoArgumentsNoneUsesLibx264(t *testing.T) {
	profile := VideoProfile{
		Backend:    config.VideoHwaccelNone,
		Encoder:    "libx264",
		FFmpegPath: "ffmpeg",
	}
	arguments := ffmpegVideoArguments(profile)
	for _, pair := range [][2]string{
		{"-c:v", "libx264"},
		{"-preset", "veryfast"},
		{"-tune", "zerolatency"},
		{"-crf", "23"},
	} {
		if !containsArgumentPair(arguments, pair[0], pair[1]) {
			t.Fatalf("arguments lack %q %q: %v", pair[0], pair[1], arguments)
		}
	}
}
