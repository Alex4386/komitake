package rcd

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
)

func header(dataSize uint32) []byte {
	buf := make([]byte, headerSize)
	binary.BigEndian.PutUint32(buf[4:8], dataSize)
	return buf
}

// A declared length of 0xFFFFFFFF used to overflow headerSize+int(dataSize) on
// 32-bit builds, wrapping to a small value that defeated the length check and
// panicked on the payload slice.
func TestMessageSizeRejectsOverflowLength(t *testing.T) {
	for _, dataSize := range []uint32{
		MaxPayloadSize + 1,
		math.MaxUint32,
		math.MaxUint32 - headerSize + 1,
		0xFFFFFFF0,
	} {
		if _, err := MessageSize(header(dataSize)); !errors.Is(err, ErrPayloadTooLarge) {
			t.Fatalf("MessageSize(dataSize=%#x) error = %v, want ErrPayloadTooLarge", dataSize, err)
		}
	}
}

func TestMessageSizeShortBufferReportsEOF(t *testing.T) {
	// Previously this returned (headerSize, nil), confidently reporting a size
	// it could not know.
	if _, err := MessageSize(make([]byte, headerSize-1)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestMessageSizeAcceptsBoundary(t *testing.T) {
	got, err := MessageSize(header(MaxPayloadSize))
	if err != nil {
		t.Fatalf("MessageSize() error = %v", err)
	}
	if want := headerSize + MaxPayloadSize; got != want {
		t.Fatalf("size = %d, want %d", got, want)
	}
}

// DecodeMessage is exported and must not rely on callers pre-validating length.
func TestDecodeMessageRejectsOversizedLength(t *testing.T) {
	if _, err := DecodeMessage(header(math.MaxUint32)); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("error = %v, want ErrPayloadTooLarge", err)
	}
}

func TestDecodeMessageRejectsTruncatedPayload(t *testing.T) {
	// Header claims 32 payload bytes but only 8 follow.
	buf := append(header(32), make([]byte, 8)...)
	if _, err := DecodeMessage(buf); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestDecodeMessageDoesNotAliasInput(t *testing.T) {
	buf := append(header(4), 1, 2, 3, 4)
	msg, err := DecodeMessage(buf)
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}

	buf[headerSize] = 0xff
	if msg.Data[0] != 1 {
		t.Fatal("decoded payload aliases the input buffer")
	}
}

func TestDecodeMessageEmptyPayload(t *testing.T) {
	msg, err := DecodeMessage(header(0))
	if err != nil {
		t.Fatalf("DecodeMessage() error = %v", err)
	}
	if len(msg.Data) != 0 {
		t.Fatalf("Data = %v, want empty", msg.Data)
	}
}

// FuzzDecodeMessage checks that arbitrary bytes never panic the decoder.
func FuzzDecodeMessage(f *testing.F) {
	f.Add(header(0))
	f.Add(append(header(4), 1, 2, 3, 4))
	f.Add(header(math.MaxUint32))
	f.Add(make([]byte, 3))

	f.Fuzz(func(t *testing.T, data []byte) {
		msg, err := DecodeMessage(data)
		if err != nil {
			return
		}
		// A successful decode must be internally consistent.
		if len(msg.Data) > MaxPayloadSize {
			t.Fatalf("decoded payload of %d exceeds maximum", len(msg.Data))
		}
		if headerSize+len(msg.Data) > len(data) {
			t.Fatalf("decoded %d payload bytes from a %d byte buffer", len(msg.Data), len(data))
		}
	})
}
