package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	VideoHwaccelAuto   = "auto"
	VideoHwaccelVAAPI  = "vaapi"
	VideoHwaccelNVENC  = "nvenc"
	VideoHwaccelQSV    = "qsv"
	VideoHwaccelCustom = "custom"
	VideoHwaccelNone   = "none"

	VideoFFmpegProfileRealtime = "realtime"
)

// VideoFFmpegArgsFile holds optional ffmpeg argument overrides split around the
// pipe input. With built-in hwaccel profiles these append after the matching
// built-in section and override duplicate flags via ffmpeg last-wins semantics.
type VideoFFmpegArgsFile struct {
	Input  []string `json:"input,omitempty"`
	Output []string `json:"output,omitempty"`
}

// VideoFile configures live camera transcoding in the daemon.
type VideoFile struct {
	// Hwaccel selects the encoder backend. "auto" discovers an H.264 hardware
	// encoder at daemon startup; "custom" uses only ffmpeg_args; "none" uses
	// software encoding (libx264).
	Hwaccel string `json:"hwaccel,omitempty"`
	// FFmpegPath overrides the ffmpeg binary searched on PATH.
	FFmpegPath string `json:"ffmpeg_path,omitempty"`
	// FFmpegProfile selects optional ffmpeg tuning presets (e.g. realtime).
	FFmpegProfile string `json:"ffmpeg_profile,omitempty"`
	FFmpegArgs VideoFFmpegArgsFile `json:"ffmpeg_args,omitempty"`
}

func (video VideoFile) NormalizedHwaccel() string {
	value := strings.ToLower(strings.TrimSpace(video.Hwaccel))
	if value == "" {
		return VideoHwaccelAuto
	}
	return value
}

func (video VideoFile) ResolvedFFmpegPath() (string, error) {
	if path := strings.TrimSpace(video.FFmpegPath); path != "" {
		return path, nil
	}
	return exec.LookPath("ffmpeg")
}

func (video VideoFile) NormalizedFFmpegProfile() string {
	return strings.ToLower(strings.TrimSpace(video.FFmpegProfile))
}

func (video VideoFile) HasFFmpegArgs() bool {
	return len(video.FFmpegArgs.Input) > 0 || len(video.FFmpegArgs.Output) > 0
}

func ValidateVideoFFmpegProfile(raw string) error {
	switch raw {
	case "", VideoFFmpegProfileRealtime:
		return nil
	default:
		return fmt.Errorf("video.ffmpeg_profile %q: want realtime or empty", raw)
	}
}

func ValidateVideoHwaccel(raw string) error {
	switch raw {
	case VideoHwaccelAuto, VideoHwaccelVAAPI, VideoHwaccelNVENC, VideoHwaccelQSV, VideoHwaccelCustom, VideoHwaccelNone:
		return nil
	default:
		return fmt.Errorf("video.hwaccel %q: want auto, vaapi, nvenc, qsv, custom, or none (software)", raw)
	}
}

func ValidateVideo(video VideoFile) error {
	hwaccel := video.NormalizedHwaccel()
	if err := ValidateVideoHwaccel(hwaccel); err != nil {
		return err
	}
	if err := ValidateVideoFFmpegProfile(video.NormalizedFFmpegProfile()); err != nil {
		return err
	}
	if path := strings.TrimSpace(video.FFmpegPath); path != "" {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("video.ffmpeg_path %q: %w", path, err)
		}
	}
	if hwaccel == VideoHwaccelCustom {
		if len(video.FFmpegArgs.Input) == 0 || len(video.FFmpegArgs.Output) == 0 {
			return fmt.Errorf("video.hwaccel custom: video.ffmpeg_args.input and video.ffmpeg_args.output are required")
		}
		if !ffmpegArgsContain(video.FFmpegArgs.Input, "pipe:0") {
			return fmt.Errorf("video.ffmpeg_args.input must include pipe:0")
		}
		if !ffmpegArgsContain(video.FFmpegArgs.Output, "pipe:1") {
			return fmt.Errorf("video.ffmpeg_args.output must include pipe:1")
		}
	}
	return nil
}

func ffmpegArgsContain(arguments []string, wanted string) bool {
	for _, argument := range arguments {
		if argument == wanted {
			return true
		}
	}
	return false
}
