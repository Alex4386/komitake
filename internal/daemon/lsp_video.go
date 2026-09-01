package daemon

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	niffinDatagramSize  = 1416
	niffinHeaderSize    = 8
	moLivePacketSize    = 1400
	moLiveTrailerSize   = 8
	l2HeaderSize        = 14
	framVideoHeaderSize = 32
)

var errUnsupportedParity = errors.New("EC-C1 parity packet requires a parity-bearing fixture")

type niffinPacket struct {
	sequence    uint32
	generation  uint8
	sourceCount uint8
	index       uint8
	parity      bool
	packet      []byte
}

func parseNiffinPacket(datagram []byte) (niffinPacket, error) {
	if len(datagram) != niffinDatagramSize {
		return niffinPacket{}, fmt.Errorf("EC-C1 datagram length %d, want %d", len(datagram), niffinDatagramSize)
	}
	if (uint16(datagram[0])<<4)|uint16(datagram[1]>>4) != 0xECC || datagram[1]&0x0F != 1 {
		return niffinPacket{}, errors.New("invalid EC-C1 magic or version")
	}
	packet := niffinPacket{
		sequence:   uint32(datagram[2]&0x7F)<<16 | uint32(datagram[3])<<8 | uint32(datagram[4]),
		generation: datagram[5], sourceCount: datagram[6], index: datagram[7],
		parity: datagram[2]&0x80 != 0,
		packet: append([]byte(nil), datagram[niffinHeaderSize:niffinHeaderSize+moLivePacketSize]...),
	}
	if packet.sourceCount == 0 {
		return niffinPacket{}, errors.New("EC-C1 source count is zero")
	}
	if packet.parity {
		return packet, errUnsupportedParity
	}
	if packet.index >= packet.sourceCount {
		return niffinPacket{}, errors.New("EC-C1 source index outside generation")
	}
	return packet, nil
}

type videoAssembler struct {
	deviceID          string
	synchronized      bool
	packetSize        int
	serialInitialized bool
	serial            uint16
	activeFrame       *activeVideoFrame
	sequence          uint64
}

type activeVideoFrame struct {
	total    int
	metadata [3]uint64
	data     []byte
}

func newVideoAssembler(deviceID string) *videoAssembler { return &videoAssembler{deviceID: deviceID} }

func (assembler *videoAssembler) discardActiveFrame() {
	assembler.activeFrame = nil
}

func (assembler *videoAssembler) reset() {
	assembler.synchronized = false
	assembler.packetSize = 0
	assembler.serialInitialized = false
	assembler.activeFrame = nil
}

func (assembler *videoAssembler) consume(packet []byte) ([]VideoFrame, error) {
	if len(packet) != moLivePacketSize {
		assembler.reset()
		return nil, fmt.Errorf("MoLive packet length %d", len(packet))
	}
	cursor := 0
	if !assembler.synchronized {
		if len(packet) < l2HeaderSize || packet[0] != 'L' || packet[1] != '2' {
			return nil, errors.New("waiting for L2 synchronization")
		}
		words := [4]uint16{}
		for index := range words {
			words[index] = binary.BigEndian.Uint16(packet[4+index*2 : 6+index*2])
		}
		checksum := (words[0] & 0x7FFF) ^ words[1] ^ words[2] ^ words[3] ^ 0xAAAA
		if checksum != binary.BigEndian.Uint16(packet[2:4]) {
			return nil, errors.New("invalid L2 checksum")
		}
		assembler.packetSize = int(binary.BigEndian.Uint16(packet[12:14])) + 1
		if assembler.packetSize < 16 || assembler.packetSize > len(packet) {
			return nil, fmt.Errorf("unsupported L2 packet size %d", assembler.packetSize)
		}
		assembler.synchronized = true
		cursor = l2HeaderSize
		for {
			tag, next, err := readBase128(packet[:assembler.packetSize], cursor)
			if err != nil {
				assembler.reset()
				return nil, err
			}
			length, payloadStart, err := readBase128(packet[:assembler.packetSize], next)
			if err != nil {
				assembler.reset()
				return nil, err
			}
			payloadEnd := payloadStart + int(length)
			if payloadEnd > assembler.packetSize {
				assembler.reset()
				return nil, errors.New("descriptor exceeds L2 packet")
			}
			cursor = payloadEnd
			if tag == 0 {
				break
			}
			if tag != 1 {
				assembler.reset()
				return nil, fmt.Errorf("unsupported captured descriptor tag %d", tag)
			}
			if length != 12 || packet[payloadStart] != 0 {
				assembler.reset()
				return nil, errors.New("unsupported video descriptor")
			}
		}
		if cursor >= assembler.packetSize {
			assembler.reset()
			return nil, errors.New("missing L2 section control")
		}
		control := packet[cursor]
		cursor++
		if control&2 != 0 {
			if cursor+2 > assembler.packetSize {
				assembler.reset()
				return nil, errors.New("truncated L2 serial")
			}
			serial := binary.BigEndian.Uint16(packet[cursor : cursor+2])
			cursor += 2
			if assembler.serialInitialized && serial != assembler.serial+1 {
				assembler.activeFrame = nil
			}
			assembler.serial = serial
			assembler.serialInitialized = true
		}
	} else {
		cursor = 3
	}
	return assembler.consumeMediaEntries(packet[:assembler.packetSize], cursor)
}

func readBase128(data []byte, cursor int) (uint32, int, error) {
	var value uint32
	for count := 0; count < 4; count++ {
		if cursor >= len(data) {
			return 0, cursor, errors.New("truncated base-128 integer")
		}
		current := data[cursor]
		cursor++
		if count == 3 {
			return value<<7 | uint32(current), cursor, nil
		}
		value = value<<7 | uint32(current&0x7F)
		if current&0x80 == 0 {
			return value, cursor, nil
		}
	}
	return value, cursor, nil
}

func (assembler *videoAssembler) consumeMediaEntries(packet []byte, cursor int) ([]VideoFrame, error) {
	frames := make([]VideoFrame, 0, 1)
	for cursor < len(packet) {
		if packet[cursor] == 0 {
			return frames, nil
		}
		complete, payloadStart, payloadLength, err := parseCapturedStreamZeroHeader(packet, cursor)
		if err != nil {
			assembler.activeFrame = nil
			return frames, err
		}
		payloadEnd := payloadStart + payloadLength
		if payloadEnd > len(packet) {
			assembler.activeFrame = nil
			return frames, errors.New("media payload exceeds packet")
		}
		payload := packet[payloadStart:payloadEnd]
		cursor = payloadEnd
		if assembler.activeFrame == nil {
			if complete || len(payload) != framVideoHeaderSize || string(payload[:4]) != "FRAM" {
				continue
			}
			total := int(binary.BigEndian.Uint32(payload[4:8]))
			if total < framVideoHeaderSize {
				return frames, errors.New("invalid FRAM total")
			}
			assembler.activeFrame = &activeVideoFrame{total: total, metadata: [3]uint64{
				binary.BigEndian.Uint64(payload[8:16]), binary.BigEndian.Uint64(payload[16:24]), binary.BigEndian.Uint64(payload[24:32]),
			}, data: append([]byte(nil), payload...)}
			continue
		}
		assembler.activeFrame.data = append(assembler.activeFrame.data, payload...)
		if !complete {
			continue
		}
		frame := assembler.activeFrame
		assembler.activeFrame = nil
		if len(frame.data) != frame.total {
			continue
		}
		encoded := append([]byte(nil), frame.data[framVideoHeaderSize:]...)
		assembler.sequence++
		frames = append(frames, VideoFrame{DeviceID: assembler.deviceID, Sequence: assembler.sequence,
			Metadata0: frame.metadata[0], Metadata1: frame.metadata[1], Metadata2: frame.metadata[2],
			KeyFrame: containsNALType(encoded, 5), Data: encoded})
	}
	return frames, nil
}

func parseCapturedStreamZeroHeader(packet []byte, cursor int) (bool, int, int, error) {
	if cursor >= len(packet) {
		return false, cursor, 0, errors.New("missing media header")
	}
	headerSize := 0
	complete := false
	switch packet[cursor] & 0xE0 {
	case 0x80:
		headerSize = 2
	case 0xA0:
		headerSize = 6
		complete = true
	default:
		return false, cursor, 0, fmt.Errorf("unsupported media header %02x", packet[cursor])
	}
	if cursor+headerSize > len(packet) {
		return false, cursor, 0, errors.New("truncated media header")
	}
	header := packet[cursor : cursor+headerSize]
	payloadLength := int(binary.BigEndian.Uint16(header[headerSize-2:])&0x1FFF) + 1
	return complete, cursor + headerSize, payloadLength, nil
}

func containsNALType(payload []byte, target byte) bool {
	for index := 0; index+4 < len(payload); index++ {
		header := -1
		if payload[index] == 0 && payload[index+1] == 0 && payload[index+2] == 1 {
			header = index + 3
		}
		if index+4 < len(payload) && payload[index] == 0 && payload[index+1] == 0 && payload[index+2] == 0 && payload[index+3] == 1 {
			header = index + 4
		}
		if header >= 0 && payload[header]&0x1F == target {
			return true
		}
	}
	return false
}
