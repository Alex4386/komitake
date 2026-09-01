package web

import (
	"encoding/binary"

	"github.com/Alex4386/komitake/pkg/komitake"
)

const videoEnvelopeHeaderSize = 16

func encodeVideoFrame(frame komitake.VideoFrame) []byte {
	output := make([]byte, videoEnvelopeHeaderSize+len(frame.AnnexB))
	copy(output[0:4], "KTV1")
	if frame.KeyFrame {
		output[4] |= 1
	}
	if frame.Discontinuity {
		output[4] |= 2
	}
	binary.BigEndian.PutUint64(output[8:16], frame.Sequence)
	copy(output[videoEnvelopeHeaderSize:], frame.AnnexB)
	return output
}
