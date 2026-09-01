package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateVideoHwaccel(t *testing.T) {
	for _, value := range []string{"auto", "vaapi", "nvenc", "qsv", "custom", "none", "AUTO", " VAAPI "} {
		if err := ValidateVideoHwaccel(VideoFile{Hwaccel: value}.NormalizedHwaccel()); err != nil {
			t.Fatalf("ValidateVideoHwaccel(%q) = %v", value, err)
		}
	}
	if err := ValidateVideoHwaccel("software"); err == nil {
		t.Fatal("expected invalid hwaccel error")
	}
}

func TestVideoFileDefaultsToAuto(t *testing.T) {
	if got := (VideoFile{}).NormalizedHwaccel(); got != VideoHwaccelAuto {
		t.Fatalf("NormalizedHwaccel() = %q, want auto", got)
	}
}

func TestValidateVideoFFmpegProfile(t *testing.T) {
	if err := ValidateVideoFFmpegProfile("realtime"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateVideoFFmpegProfile(""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateVideoFFmpegProfile("cinematic"); err == nil {
		t.Fatal("expected invalid profile error")
	}
}

func TestValidateVideoCustomRequiresPipeEndpoints(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte{0x7f, 'E', 'L', 'F'}, 0o755); err != nil {
		t.Fatal(err)
	}
	err := ValidateVideo(VideoFile{
		Hwaccel:    "custom",
		FFmpegPath: path,
		FFmpegArgs: VideoFFmpegArgsFile{
			Input:  []string{"-f", "h264", "-i", "pipe:0"},
			Output: []string{"-c:v", "libx264", "-f", "h264", "pipe:1"},
		},
	})
	if err != nil {
		t.Fatalf("ValidateVideo() = %v", err)
	}

	if err := ValidateVideo(VideoFile{
		Hwaccel:    "custom",
		FFmpegPath: path,
		FFmpegArgs: VideoFFmpegArgsFile{
			Input:  []string{"-f", "h264", "-i", "/dev/null"},
			Output: []string{"-c:v", "libx264", "-f", "h264", "pipe:1"},
		},
	}); err == nil {
		t.Fatal("expected missing pipe:0 error")
	}
}

func TestValidateVideoNoneSkipsFFmpegPath(t *testing.T) {
	if err := ValidateVideo(VideoFile{Hwaccel: "none"}); err != nil {
		t.Fatalf("ValidateVideo() = %v", err)
	}
}

func TestValidateVideoFFmpegPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte{0x7f, 'E', 'L', 'F'}, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateVideo(VideoFile{FFmpegPath: path}); err != nil {
		t.Fatalf("ValidateVideo() = %v", err)
	}
	if err := ValidateVideo(VideoFile{FFmpegPath: path + "-missing"}); err == nil {
		t.Fatal("expected missing ffmpeg path error")
	}
}
