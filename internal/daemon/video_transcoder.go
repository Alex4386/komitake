package daemon

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"

	"github.com/Alex4386/komitake/internal/config"
)

const (
	ffmpegVideoFrameRate = 25
	ffmpegVideoGOPFrames = 10
)

var ffmpegVideoCommand = exec.CommandContext

type videoEncoder interface {
	writeFrame([]byte) error
	close()
}

type disabledVideoEncoder struct{}

func (disabledVideoEncoder) writeFrame([]byte) error { return nil }
func (disabledVideoEncoder) close()                  {}

type videoTranscoder struct {
	deviceID  string
	stdin     io.WriteCloser
	command   *exec.Cmd
	cancel    context.CancelFunc
	exited    chan struct{}
	exitMu    sync.Mutex
	exitErr   error
	writeMu   sync.Mutex
	closeOnce sync.Once
}

func startVideoTranscoder(parent context.Context, deviceID string, hub *videoHub, logger *slog.Logger, profile VideoProfile) (*videoTranscoder, error) {
	if profile.FFmpegPath == "" {
		return nil, fmt.Errorf("video transcoder: ffmpeg path is unset")
	}
	ctx, cancel := context.WithCancel(parent)
	arguments := ffmpegVideoArguments(profile)
	logArgs := []any{
		"ident", deviceID,
		"binary", profile.FFmpegPath,
		"hwaccel", profile.Backend,
		"frame_rate", ffmpegVideoFrameRate,
		"gop_frames", ffmpegVideoGOPFrames,
		"software_fallback", false,
	}
	if profileName := profile.Video.NormalizedFFmpegProfile(); profileName != "" {
		logArgs = append(logArgs, "ffmpeg_profile", profileName)
	}
	if profile.Backend == config.VideoHwaccelCustom {
		logArgs = append(logArgs, "custom_ffmpeg_args", true)
	} else {
		logArgs = append(logArgs,
			"decoder", profile.Backend,
			"encoder", profile.Encoder,
			"render_node", profile.RenderNode,
			"low_power", profile.Backend == config.VideoHwaccelVAAPI,
		)
	}
	logger.Info("starting video transcoder", logArgs...)
	command := ffmpegVideoCommand(ctx, profile.FFmpegPath, arguments...)
	stdin, err := command.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open ffmpeg stdin: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open ffmpeg stdout: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("open ffmpeg stderr: %w", err)
	}
	if err := command.Start(); err != nil {
		cancel()
		logger.Error("video transcoder failed to start", "ident", deviceID, "encoder", profile.Encoder, "error", err)
		return nil, fmt.Errorf("start ffmpeg hardware video transcoder: %w", err)
	}
	logger.Info("video transcoder process started", "ident", deviceID, "pid", command.Process.Pid, "encoder", profile.Encoder)
	transcoder := &videoTranscoder{deviceID: deviceID, stdin: stdin, command: command, cancel: cancel, exited: make(chan struct{})}
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.Contains(line, "Using VAAPI"), strings.Contains(line, "Input surface format"), strings.Contains(line, "entrypoint"):
				logger.Info("video transcoder hardware", "ident", deviceID, "encoder", profile.Encoder, "ffmpeg", line)
			case strings.Contains(strings.ToLower(line), "error"), strings.Contains(strings.ToLower(line), "failed"):
				logger.Warn("video transcoder diagnostic", "ident", deviceID, "encoder", profile.Encoder, "ffmpeg", line)
			default:
				logger.Debug("video transcoder", "ident", deviceID, "ffmpeg", line)
			}
		}
	}()
	go func() {
		var sequence uint64
		readErr := readFFmpegAccessUnits(stdout, func(accessUnit []byte) error {
			sequence++
			if sequence == 1 {
				logger.Info("video transcoder ready", "ident", deviceID, "pid", command.Process.Pid, "encoder", profile.Encoder, "first_frame_bytes", len(accessUnit), "key_frame", containsNALType(accessUnit, 5))
			}
			hub.publish(VideoFrame{DeviceID: deviceID, Sequence: sequence, KeyFrame: containsNALType(accessUnit, 5), Data: accessUnit})
			return nil
		})
		waitErr := command.Wait()
		transcoder.exitMu.Lock()
		if readErr != nil && !errors.Is(readErr, context.Canceled) {
			transcoder.exitErr = readErr
		} else {
			transcoder.exitErr = waitErr
		}
		transcoder.exitMu.Unlock()
		if transcoder.exitErr != nil && ctx.Err() == nil {
			logger.Error("video transcoder exited unexpectedly", "ident", deviceID, "pid", command.Process.Pid, "encoder", profile.Encoder, "error", transcoder.exitErr)
		} else {
			logger.Info("video transcoder stopped", "ident", deviceID, "pid", command.Process.Pid, "encoder", profile.Encoder)
		}
		close(transcoder.exited)
	}()
	return transcoder, nil
}

func (transcoder *videoTranscoder) writeFrame(frame []byte) error {
	transcoder.writeMu.Lock()
	defer transcoder.writeMu.Unlock()
	select {
	case <-transcoder.exited:
		transcoder.exitMu.Lock()
		err := transcoder.exitErr
		transcoder.exitMu.Unlock()
		if err == nil {
			return io.ErrClosedPipe
		}
		return fmt.Errorf("video transcoder exited: %w", err)
	default:
	}
	payload := normalizeSourceAnnexB(frame)
	_, err := transcoder.stdin.Write(payload)
	return err
}

func normalizeSourceAnnexB(payload []byte) []byte {
	trailingAUD := []byte{0, 0, 0, 1, 9, 0x30}
	if len(payload) >= len(trailingAUD) && string(payload[len(payload)-len(trailingAUD):]) == string(trailingAUD) {
		return payload[:len(payload)-len(trailingAUD)]
	}
	return payload
}

func (transcoder *videoTranscoder) close() {
	transcoder.closeOnce.Do(func() {
		transcoder.writeMu.Lock()
		_ = transcoder.stdin.Close()
		transcoder.writeMu.Unlock()
		transcoder.cancel()
		if transcoder.command.Process != nil {
			_ = transcoder.command.Process.Kill()
		}
		<-transcoder.exited
	})
}

func readFFmpegAccessUnits(reader io.Reader, emit func([]byte) error) error {
	buffer := make([]byte, 0, 1<<20)
	chunk := make([]byte, 64<<10)
	for {
		count, err := reader.Read(chunk)
		if count > 0 {
			buffer = append(buffer, chunk[:count]...)
			for {
				first := findAUDStart(buffer, 0)
				if first < 0 {
					if len(buffer) > 8 {
						buffer = append([]byte(nil), buffer[len(buffer)-8:]...)
					}
					break
				}
				second := findAUDStart(buffer, first+5)
				if second < 0 {
					if first > 0 {
						buffer = append([]byte(nil), buffer[first:]...)
					}
					break
				}
				accessUnit := append([]byte(nil), buffer[first:second]...)
				if emitErr := emit(accessUnit); emitErr != nil {
					return emitErr
				}
				buffer = append([]byte(nil), buffer[second:]...)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if start := findAUDStart(buffer, 0); start >= 0 && len(buffer) > start {
					return emit(append([]byte(nil), buffer[start:]...))
				}
				return nil
			}
			return err
		}
	}
}

func findAUDStart(data []byte, from int) int {
	for index := from; index+5 <= len(data); index++ {
		if index+6 <= len(data) && data[index] == 0 && data[index+1] == 0 && data[index+2] == 0 && data[index+3] == 1 && data[index+4]&0x1F == 9 {
			return index
		}
		if data[index] == 0 && data[index+1] == 0 && data[index+2] == 1 && data[index+3]&0x1F == 9 {
			return index
		}
	}
	return -1
}
