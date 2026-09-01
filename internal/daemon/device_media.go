package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
)

const initialLVNIRecordSize = 24

var initialLVNIRecord = [initialLVNIRecordSize]byte{'L', 'V', 'N', 'I'}

type deviceMedia struct {
	ident               string
	deviceAddress       string
	slot                int
	telemetryPort       int
	lspControlPort      int
	lspVideoPort        int
	telemetryConnection net.PacketConn
	controlListener     net.Listener
	videoConnection     net.PacketConn
	cancel              context.CancelFunc
	closeOnce           sync.Once
	transcoder          videoEncoder
}

func (manager *Manager) openDeviceMedia(parent context.Context, ident, deviceAddress string) (*deviceMedia, error) {
	var lastError error
	for slot := 0; slot < maximumMediaSlots; slot++ {
		media, err := manager.reserveDeviceMedia(parent, ident, deviceAddress, slot)
		if err == nil {
			transcoder, transcoderErr := manager.newTranscoder(parent, ident, manager.video, manager.logger)
			if transcoderErr != nil {
				media.closeSockets()
				lastError = transcoderErr
				continue
			}
			media.transcoder = transcoder
			manager.serveDeviceMedia(parent, media)
			return media, nil
		}
		lastError = err
	}
	return nil, fmt.Errorf("reserve Fuji media ports: %w", lastError)
}

func (manager *Manager) reserveDeviceMedia(parent context.Context, ident, deviceAddress string, slot int) (*deviceMedia, error) {
	telemetryPort := fujiTelemetryBasePort + slot
	controlPort := fujiLSPControlBasePort + slot
	videoPort := fujiLSPVideoBasePort + slot
	telemetry, err := manager.listenPacket("udp", net.JoinHostPort(manager.cfg.Address, strconv.Itoa(telemetryPort)))
	if err != nil {
		return nil, err
	}
	control, err := manager.listen("tcp", net.JoinHostPort(manager.cfg.Address, strconv.Itoa(controlPort)))
	if err != nil {
		telemetry.Close()
		return nil, err
	}
	video, err := manager.listenPacket("udp", net.JoinHostPort(manager.cfg.Address, strconv.Itoa(videoPort)))
	if err != nil {
		control.Close()
		telemetry.Close()
		return nil, err
	}
	return &deviceMedia{ident: ident, deviceAddress: hostWithoutPort(deviceAddress), slot: slot,
		telemetryPort: telemetryPort, lspControlPort: controlPort, lspVideoPort: videoPort,
		telemetryConnection: telemetry, controlListener: control, videoConnection: video}, nil
}

func (manager *Manager) serveDeviceMedia(parent context.Context, media *deviceMedia) {
	ctx, cancel := context.WithCancel(parent)
	media.cancel = cancel
	manager.backgroundWG.Add(3)
	go func() { defer manager.backgroundWG.Done(); manager.serveMediaTelemetry(ctx, media) }()
	go func() { defer manager.backgroundWG.Done(); manager.serveLSPControl(ctx, media) }()
	go func() { defer manager.backgroundWG.Done(); manager.serveLSPVideo(ctx, media) }()
}

func (manager *Manager) serveMediaTelemetry(ctx context.Context, media *deviceMedia) {
	buffer := make([]byte, 512)
	for {
		count, source, err := media.telemetryConnection.ReadFrom(buffer)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		if hostWithoutPort(source.String()) == media.deviceAddress {
			manager.applyTelemetryPacket(media.ident, buffer[:count])
		}
	}
}

func (manager *Manager) serveLSPControl(ctx context.Context, media *deviceMedia) {
	for {
		connection, err := media.controlListener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		if hostWithoutPort(connection.RemoteAddr().String()) != media.deviceAddress {
			connection.Close()
			continue
		}
		if _, err := connection.Write(initialLVNIRecord[:]); err != nil {
			connection.Close()
			continue
		}
		manager.logger.Info("LSP control session established", "ident", media.ident, "remote", connection.RemoteAddr())
		manager.holdLSPControlConnection(ctx, connection)
	}
}

func (manager *Manager) holdLSPControlConnection(ctx context.Context, connection net.Conn) {
	defer connection.Close()
	done := make(chan struct{})
	go func() { var one [1]byte; _, _ = connection.Read(one[:]); close(done) }()
	select {
	case <-ctx.Done():
	case <-done:
	}
}

func (manager *Manager) serveLSPVideo(ctx context.Context, media *deviceMedia) {
	buffer := make([]byte, niffinDatagramSize)
	assembler := newVideoAssembler(media.ident)
	var generation uint8
	var sourceCount uint8
	var sourcePackets map[uint8][]byte
	haveGeneration := false
	var previousSequence uint32
	haveSequence := false
	for {
		count, source, err := media.videoConnection.ReadFrom(buffer)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		if hostWithoutPort(source.String()) != media.deviceAddress {
			continue
		}
		packet, err := parseNiffinPacket(buffer[:count])
		if errors.Is(err, errUnsupportedParity) {
			continue
		}
		if err != nil {
			assembler.reset()
			haveGeneration = false
			continue
		}
		if haveSequence && packet.sequence != (previousSequence+1)&0x7FFFFF {
			assembler.discardActiveFrame()
			haveGeneration = false
		}
		previousSequence = packet.sequence
		haveSequence = true
		if !haveGeneration || packet.generation != generation {
			if haveGeneration && len(sourcePackets) != int(sourceCount) {
				assembler.discardActiveFrame()
			}
			generation = packet.generation
			sourceCount = packet.sourceCount
			sourcePackets = make(map[uint8][]byte, sourceCount)
			haveGeneration = true
		}
		if packet.sourceCount != sourceCount {
			assembler.discardActiveFrame()
			haveGeneration = false
			continue
		}
		sourcePackets[packet.index] = packet.packet
		if len(sourcePackets) != int(sourceCount) {
			continue
		}
		for sourceIndex := uint8(0); sourceIndex < sourceCount; sourceIndex++ {
			frames, parseErr := assembler.consume(sourcePackets[sourceIndex])
			if parseErr != nil {
				assembler.discardActiveFrame()
				break
			}
			for _, frame := range frames {
				if writeErr := media.transcoder.writeFrame(frame.Data); writeErr != nil {
					manager.logger.Error("video transcoder input failed", "ident", media.ident, "error", writeErr)
					manager.video.publishDiscontinuity(media.ident)
					return
				}
			}
		}
		haveGeneration = false
	}
}

func (media *deviceMedia) closeSockets() {
	if media.telemetryConnection != nil {
		_ = media.telemetryConnection.Close()
	}
	if media.controlListener != nil {
		_ = media.controlListener.Close()
	}
	if media.videoConnection != nil {
		_ = media.videoConnection.Close()
	}
}

func (media *deviceMedia) close() {
	media.closeOnce.Do(func() {
		if media.transcoder != nil {
			media.transcoder.close()
		}
		if media.cancel != nil {
			media.cancel()
		}
		media.closeSockets()
	})
}
