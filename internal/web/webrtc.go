package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Alex4386/komitake/internal/config"
	"github.com/Alex4386/komitake/internal/logging"
	"github.com/Alex4386/komitake/pkg/komitake"

	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

const webRTCFrameDuration = 40 * time.Millisecond

type webRTCOffer struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

type iceServerDTO struct {
	URLs []string `json:"urls"`
}

type webRTCConfigDTO struct {
	ICEServers []iceServerDTO `json:"ice_servers"`
}

func registerWebRTC(mux *http.ServeMux, client Client, configPath string) {
	mux.HandleFunc("POST /v1/karts/by-id/{id}/webrtc", func(writer http.ResponseWriter, request *http.Request) {
		serveWebRTCOffer(writer, request, client, configPath, request.PathValue("id"))
	})
	mux.HandleFunc("GET /v1/webrtc/config", func(writer http.ResponseWriter, request *http.Request) {
		serveWebRTCConfig(writer, configPath)
	})
}

// webRTCConfiguration reads the configured STUN servers and turns them into a
// pion Configuration. Missing or unreadable config yields an empty (host-only)
// configuration so same-segment viewing keeps working.
func webRTCConfiguration(configPath string) webrtc.Configuration {
	servers := webRTCSTUNServers(configPath)
	if len(servers) == 0 {
		return webrtc.Configuration{}
	}
	return webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{URLs: servers}},
	}
}

func webRTCSTUNServers(configPath string) []string {
	settings, err := config.ReadServiceSettings(configPath)
	if err != nil {
		// Fall back to the built-in default so NAT traversal still works when
		// config is unreadable.
		return config.WebRTCFile{}.ResolvedSTUNServers()
	}
	return settings.WebRTC.ResolvedSTUNServers()
}

func serveWebRTCConfig(writer http.ResponseWriter, configPath string) {
	servers := webRTCSTUNServers(configPath)
	out := webRTCConfigDTO{ICEServers: []iceServerDTO{}}
	if len(servers) > 0 {
		out.ICEServers = append(out.ICEServers, iceServerDTO{URLs: servers})
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(out)
}

func serveWebRTCOffer(writer http.ResponseWriter, request *http.Request, client Client, configPath, selector string) {
	kart, err := resolveKart(request.Context(), client, selector)
	if err != nil {
		writeResolveError(writer, err)
		return
	}
	var offer webRTCOffer
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1<<20))
	if err := decoder.Decode(&offer); err != nil || offer.Type != "offer" || offer.SDP == "" {
		http.Error(writer, "invalid WebRTC offer", http.StatusBadRequest)
		return
	}
	api, err := newWebRTCAPI()
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	peer, err := api.NewPeerConnection(webRTCConfiguration(configPath))
	if err != nil {
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264},
		"video", kart.Ident,
	)
	if err != nil {
		peer.Close()
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	sender, err := peer.AddTrack(track)
	if err != nil {
		peer.Close()
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	go drainWebRTCRTCP(sender)
	log := logging.New(nil).With("component", "webrtc", "kart", kart.Ident)
	peer.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		log.Debug("ice connection state", "state", state.String())
	})
	peer.OnICEGatheringStateChange(func(state webrtc.ICEGatheringState) {
		log.Debug("ice gathering state", "state", state.String())
	})
	if err := peer.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer.SDP}); err != nil {
		peer.Close()
		http.Error(writer, "invalid WebRTC SDP offer", http.StatusBadRequest)
		return
	}
	answer, err := peer.CreateAnswer(nil)
	if err != nil {
		peer.Close()
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	gatheringComplete := webrtc.GatheringCompletePromise(peer)
	if err := peer.SetLocalDescription(answer); err != nil {
		peer.Close()
		http.Error(writer, err.Error(), http.StatusInternalServerError)
		return
	}
	select {
	case <-gatheringComplete:
	case <-request.Context().Done():
		peer.Close()
		return
	case <-time.After(10 * time.Second):
		peer.Close()
		http.Error(writer, "WebRTC ICE gathering timed out", http.StatusGatewayTimeout)
		return
	}
	localDescription := peer.LocalDescription()
	if localDescription == nil {
		peer.Close()
		http.Error(writer, "WebRTC answer unavailable", http.StatusInternalServerError)
		return
	}
	log.Debug("negotiated webrtc session",
		"offer_candidates", countSDPCandidates(offer.SDP),
		"answer_candidates", countSDPCandidates(localDescription.SDP))
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(webRTCOffer{Type: localDescription.Type.String(), SDP: localDescription.SDP}); err != nil {
		peer.Close()
		return
	}
	go streamWebRTCVideo(context.Background(), peer, track, client, kart.Ident, log)
}

func newWebRTCAPI() (*webrtc.API, error) {
	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		return nil, err
	}
	registry := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, registry); err != nil {
		return nil, err
	}
	return webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(registry),
	), nil
}

func streamWebRTCVideo(parent context.Context, peer *webrtc.PeerConnection, track *webrtc.TrackLocalStaticSample, client Client, selector string, log *logging.Logger) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	defer peer.Close()
	connected := make(chan struct{})
	var connectedOnce sync.Once
	peer.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Debug("peer connection state", "state", state.String())
		switch state {
		case webrtc.PeerConnectionStateConnected:
			connectedOnce.Do(func() { close(connected) })
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed, webrtc.PeerConnectionStateDisconnected:
			cancel()
		}
	})
	if peer.ConnectionState() == webrtc.PeerConnectionStateConnected {
		connectedOnce.Do(func() { close(connected) })
	}
	select {
	case <-connected:
	case <-ctx.Done():
		return
	case <-time.After(15 * time.Second):
		return
	}
	// FreshKeyFrame:false replays the hub's stored GOP (which begins at the last
	// keyframe) before live frames. This primes the decoder immediately on
	// connect so the first visible frame is the last real image rather than
	// black while waiting for the kart to emit a new IDR.
	stream, err := client.StreamVideoWithOptions(ctx, selector, komitake.VideoStreamOptions{FreshKeyFrame: false})
	if err != nil {
		return
	}
	for {
		frame, receiveErr := stream.Recv()
		if receiveErr != nil {
			return
		}
		if frame.Discontinuity || len(frame.AnnexB) == 0 {
			continue
		}
		if err := track.WriteSample(media.Sample{Data: frame.AnnexB, Duration: webRTCFrameDuration}); err != nil {
			return
		}
	}
}

// countSDPCandidates counts a=candidate lines in an SDP, a cheap way to confirm
// whether each side actually exchanged ICE candidates during signaling.
func countSDPCandidates(sdp string) int {
	count := 0
	for _, line := range strings.Split(sdp, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "a=candidate:") {
			count++
		}
	}
	return count
}

func drainWebRTCRTCP(sender *webrtc.RTPSender) {
	for {
		if _, _, err := sender.ReadRTCP(); err != nil {
			if !errors.Is(err, io.EOF) {
			}
			return
		}
	}
}
