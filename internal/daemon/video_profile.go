package daemon

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Alex4386/komitake/internal/config"
)

type videoBackend struct {
	name    string
	encoder string
}

var videoBackendOrder = []videoBackend{
	{name: config.VideoHwaccelVAAPI, encoder: "h264_vaapi"},
	{name: config.VideoHwaccelNVENC, encoder: "h264_nvenc"},
	{name: config.VideoHwaccelQSV, encoder: "h264_qsv"},
}

var (
	listVARenderNodes  = discoverVARenderNodes
	listFFmpegEncoders = func(ffmpegPath string) (map[string]bool, error) {
		return discoverFFmpegEncoders(ffmpegPath)
	}
)

type VideoProfile struct {
	Video      config.VideoFile
	Backend    string
	Encoder    string
	RenderNode string
	FFmpegPath string
}

func ResolveVideoProfile(video config.VideoFile) (VideoProfile, error) {
	if err := config.ValidateVideo(video); err != nil {
		return VideoProfile{}, err
	}
	hwaccel := video.NormalizedHwaccel()
	ffmpegPath, err := video.ResolvedFFmpegPath()
	if err != nil {
		return VideoProfile{}, fmt.Errorf("resolve ffmpeg path: %w", err)
	}

	if hwaccel == config.VideoHwaccelNone {
		encoders, encodersErr := listFFmpegEncoders(ffmpegPath)
		if encodersErr != nil {
			return VideoProfile{}, encodersErr
		}
		if !encoders["libx264"] {
			return VideoProfile{}, fmt.Errorf("video.hwaccel none: ffmpeg encoder libx264 is unavailable")
		}
		return VideoProfile{
			Video:      video,
			Backend:    config.VideoHwaccelNone,
			Encoder:    "libx264",
			FFmpegPath: ffmpegPath,
		}, nil
	}

	if hwaccel == config.VideoHwaccelCustom {
		return VideoProfile{
			Video:      video,
			Backend:    config.VideoHwaccelCustom,
			FFmpegPath: ffmpegPath,
		}, nil
	}

	encoders, err := listFFmpegEncoders(ffmpegPath)
	if err != nil {
		return VideoProfile{}, err
	}
	renderNodes, err := listVARenderNodes()
	if err != nil {
		return VideoProfile{}, err
	}

	if hwaccel == config.VideoHwaccelAuto {
		for _, backend := range videoBackendOrder {
			if !encoders[backend.encoder] {
				continue
			}
			switch backend.name {
			case config.VideoHwaccelVAAPI:
				if len(renderNodes) == 0 {
					continue
				}
				return VideoProfile{
					Video:      video,
					Backend:    backend.name,
					Encoder:    backend.encoder,
					RenderNode: renderNodes[0],
					FFmpegPath: ffmpegPath,
				}, nil
			case config.VideoHwaccelNVENC, config.VideoHwaccelQSV:
				return VideoProfile{
					Video:      video,
					Backend:    backend.name,
					Encoder:    backend.encoder,
					FFmpegPath: ffmpegPath,
				}, nil
			}
		}
		return VideoProfile{}, fmt.Errorf("video.hwaccel auto: no supported H.264 hardware encoder found (need h264_vaapi with /dev/dri/renderD*, h264_nvenc, or h264_qsv)")
	}

	backend := backendByName(hwaccel)
	if backend == nil {
		return VideoProfile{}, fmt.Errorf("video.hwaccel %q: unsupported backend", hwaccel)
	}
	if !encoders[backend.encoder] {
		return VideoProfile{}, fmt.Errorf("video.hwaccel %q: ffmpeg encoder %q is unavailable", hwaccel, backend.encoder)
	}
	if hwaccel == config.VideoHwaccelVAAPI {
		if len(renderNodes) == 0 {
			return VideoProfile{}, fmt.Errorf("video.hwaccel vaapi: no /dev/dri/renderD* device found")
		}
		return VideoProfile{
			Video:      video,
			Backend:    hwaccel,
			Encoder:    backend.encoder,
			RenderNode: renderNodes[0],
			FFmpegPath: ffmpegPath,
		}, nil
	}
	return VideoProfile{
		Video:      video,
		Backend:    hwaccel,
		Encoder:    backend.encoder,
		FFmpegPath: ffmpegPath,
	}, nil
}

func backendByName(name string) *videoBackend {
	for index := range videoBackendOrder {
		if videoBackendOrder[index].name == name {
			return &videoBackendOrder[index]
		}
	}
	return nil
}

func discoverFFmpegEncoders(ffmpegPath string) (map[string]bool, error) {
	command := exec.Command(ffmpegPath, "-hide_banner", "-encoders")
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("list ffmpeg encoders: %w", err)
	}
	encoders := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if !strings.ContainsAny(fields[0], "V") {
			continue
		}
		encoders[fields[1]] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read ffmpeg encoders: %w", err)
	}
	return encoders, nil
}

func discoverVARenderNodes() ([]string, error) {
	matches, err := filepath.Glob("/dev/dri/renderD*")
	if err != nil {
		return nil, fmt.Errorf("list VAAPI render nodes: %w", err)
	}
	nodes := make([]string, 0, len(matches))
	for _, node := range matches {
		file, openErr := os.Open(node)
		if openErr != nil {
			continue
		}
		_ = file.Close()
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	return nodes, nil
}
