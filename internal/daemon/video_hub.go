package daemon

import (
	"context"
	"sync"
)

const (
	maximumVideoBacklogBytes = 128 << 20
	videoSubscriberBuffer    = 4
)

// VideoFrame is one complete Annex-B H.264 frame recovered from a Fuji FRAM record.
type VideoFrame struct {
	DeviceID      string
	Sequence      uint64
	Metadata0     uint64
	Metadata1     uint64
	Metadata2     uint64
	KeyFrame      bool
	Discontinuity bool
	Data          []byte
}

type videoHub struct {
	mu               sync.Mutex
	streams          map[string]*videoStreamState
	nextSubscriberID uint64
}

type videoStreamState struct {
	backlog      []VideoFrame
	backlogBytes int
	hasKeyFrame  bool
	subscribers  map[uint64]*videoSubscriber
}

type videoSubscriber struct {
	frames chan VideoFrame
	ready  bool
}

func newVideoHub() *videoHub {
	return &videoHub{streams: make(map[string]*videoStreamState)}
}

func (hub *videoHub) publish(frame VideoFrame) {
	frame.Data = append([]byte(nil), frame.Data...)
	hub.mu.Lock()
	defer hub.mu.Unlock()
	stream := hub.streams[frame.DeviceID]
	if stream == nil {
		stream = &videoStreamState{subscribers: make(map[uint64]*videoSubscriber)}
		hub.streams[frame.DeviceID] = stream
	}
	if frame.KeyFrame {
		stream.backlog = nil
		stream.backlogBytes = 0
		stream.hasKeyFrame = true
		for _, subscriber := range stream.subscribers {
			subscriber.ready = true
		}
	}
	if stream.hasKeyFrame {
		stream.backlog = append(stream.backlog, frame)
		stream.backlogBytes += len(frame.Data)
		if stream.backlogBytes > maximumVideoBacklogBytes {
			stream.backlog = nil
			stream.backlogBytes = 0
			stream.hasKeyFrame = false
			for _, subscriber := range stream.subscribers {
				subscriber.ready = false
			}
		}
	}
	for _, subscriber := range stream.subscribers {
		if !subscriber.ready {
			continue
		}
		select {
		case subscriber.frames <- frame:
		default:
			drainVideoFrames(subscriber.frames)
			subscriber.ready = false
			subscriber.frames <- VideoFrame{DeviceID: frame.DeviceID, Discontinuity: true}
		}
	}
}

func (hub *videoHub) unsubscribe(deviceID string, subscriberID uint64) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	stream := hub.streams[deviceID]
	if stream == nil {
		return
	}
	if subscriber, ok := stream.subscribers[subscriberID]; ok {
		close(subscriber.frames)
		delete(stream.subscribers, subscriberID)
	}
}

func (hub *videoHub) publishDiscontinuity(deviceID string) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	stream := hub.streams[deviceID]
	if stream == nil {
		return
	}
	stream.backlog = nil
	stream.backlogBytes = 0
	stream.hasKeyFrame = false
	frame := VideoFrame{DeviceID: deviceID, Discontinuity: true}
	for subscriberID, subscriber := range stream.subscribers {
		subscriber.ready = false
		select {
		case subscriber.frames <- frame:
		default:
			close(subscriber.frames)
			delete(stream.subscribers, subscriberID)
		}
	}
}

func (hub *videoHub) reset(deviceID string) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	stream := hub.streams[deviceID]
	if stream == nil {
		return
	}
	stream.backlog = nil
	stream.backlogBytes = 0
	stream.hasKeyFrame = false
}

func (hub *videoHub) remove(deviceID string) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	stream := hub.streams[deviceID]
	if stream == nil {
		return
	}
	for subscriberID, subscriber := range stream.subscribers {
		close(subscriber.frames)
		delete(stream.subscribers, subscriberID)
	}
	delete(hub.streams, deviceID)
}

func (hub *videoHub) subscribe(ctx context.Context, deviceID string, freshKeyFrame bool) <-chan VideoFrame {
	hub.mu.Lock()
	stream := hub.streams[deviceID]
	if stream == nil {
		stream = &videoStreamState{subscribers: make(map[uint64]*videoSubscriber)}
		hub.streams[deviceID] = stream
	}
	var snapshot []VideoFrame
	ready := stream.hasKeyFrame
	if freshKeyFrame {
		ready = false
	} else {
		snapshot = append([]VideoFrame(nil), stream.backlog...)
	}
	hub.nextSubscriberID++
	subscriberID := hub.nextSubscriberID
	liveFrames := make(chan VideoFrame, videoSubscriberBuffer)
	stream.subscribers[subscriberID] = &videoSubscriber{frames: liveFrames, ready: ready}
	hub.mu.Unlock()

	if freshKeyFrame {
		go func() {
			<-ctx.Done()
			hub.unsubscribe(deviceID, subscriberID)
		}()
		return liveFrames
	}

	frames := make(chan VideoFrame)
	go func() {
		defer close(frames)
		defer hub.unsubscribe(deviceID, subscriberID)
		for _, frame := range snapshot {
			select {
			case frames <- frame:
			case <-ctx.Done():
				return
			}
		}
		for {
			select {
			case frame, ok := <-liveFrames:
				if !ok {
					return
				}
				select {
				case frames <- frame:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	return frames
}

func drainVideoFrames(frames chan VideoFrame) {
	for {
		select {
		case <-frames:
		default:
			return
		}
	}
}
