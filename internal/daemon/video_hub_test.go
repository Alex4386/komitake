package daemon

import (
	"context"
	"testing"
	"time"
)

func TestVideoHubSubscriberReceivesKeyframeBacklogAndLiveFrames(t *testing.T) {
	hub := newVideoHub()
	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 1, KeyFrame: true, Data: []byte{1}})
	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 2, Data: []byte{2}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	frames := hub.subscribe(ctx, "kart", false)
	for expected := uint64(1); expected <= 3; expected++ {
		if expected == 3 {
			hub.publish(VideoFrame{DeviceID: "kart", Sequence: 3, Data: []byte{3}})
		}
		select {
		case frame := <-frames:
			if frame.Sequence != expected {
				t.Fatalf("sequence = %d, want %d", frame.Sequence, expected)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for frame")
		}
	}
}

func TestVideoHubWaitsForKeyframeAfterReset(t *testing.T) {
	hub := newVideoHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	frames := hub.subscribe(ctx, "kart", false)
	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 1, Data: []byte{1}})
	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 2, KeyFrame: true, Data: []byte{2}})
	select {
	case frame := <-frames:
		if frame.Sequence != 2 {
			t.Fatalf("sequence = %d", frame.Sequence)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for keyframe")
	}
}

func TestVideoHubWaitingSubscriberReceivesNewKeyframeAfterDiscontinuity(t *testing.T) {
	hub := newVideoHub()
	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 1, KeyFrame: true, Data: []byte{1}})
	hub.publishDiscontinuity("kart")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	frames := hub.subscribe(ctx, "kart", false)
	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 2, Data: []byte{2}})
	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 3, KeyFrame: true, Data: []byte{3}})
	select {
	case frame := <-frames:
		if frame.Sequence != 3 || !frame.KeyFrame {
			t.Fatalf("frame = %+v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not resume on new keyframe")
	}
}

func TestVideoHubFreshSubscriberSkipsCachedGOP(t *testing.T) {
	hub := newVideoHub()
	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 1, KeyFrame: true, Data: []byte{1}})
	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 2, Data: []byte{2}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	frames := hub.subscribe(ctx, "kart", true)

	select {
	case frame := <-frames:
		t.Fatalf("fresh subscriber received cached frame: %+v", frame)
	case <-time.After(20 * time.Millisecond):
	}

	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 3, Data: []byte{3}})
	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 4, KeyFrame: true, Data: []byte{4}})
	select {
	case frame := <-frames:
		if frame.Sequence != 4 || !frame.KeyFrame {
			t.Fatalf("fresh subscriber first frame = %+v", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("fresh subscriber did not receive next keyframe")
	}
}

func TestVideoHubOverflowDropsStaleFramesAndResumesAtKeyframe(t *testing.T) {
	hub := newVideoHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	frames := hub.subscribe(ctx, "kart", true)

	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 1, KeyFrame: true, Data: []byte{1}})
	for sequence := uint64(2); sequence <= videoSubscriberBuffer+2; sequence++ {
		hub.publish(VideoFrame{DeviceID: "kart", Sequence: sequence, Data: []byte{byte(sequence)}})
	}

	select {
	case frame := <-frames:
		if !frame.Discontinuity {
			t.Fatalf("overflow first frame = %+v, want discontinuity", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive overflow discontinuity")
	}

	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 20, Data: []byte{20}})
	hub.publish(VideoFrame{DeviceID: "kart", Sequence: 21, KeyFrame: true, Data: []byte{21}})
	select {
	case frame := <-frames:
		if frame.Sequence != 21 || !frame.KeyFrame {
			t.Fatalf("post-overflow frame = %+v, want keyframe 21", frame)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not resume at next keyframe")
	}
}
