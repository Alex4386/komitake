package web

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Alex4386/komitake/pkg/komitake"
	"github.com/coder/websocket"
	"github.com/pion/webrtc/v4"
)

type testTelemetryPayload struct {
	Battery        *int     `json:"battery"`
	CableConnected *bool    `json:"cable_connected"`
	DriveArmed     bool     `json:"drive_armed"`
	AccelMPS2      *vec3DTO `json:"accel_mps2"`
}

type fakeClient struct {
	karts              []komitake.Kart
	pairing            *komitake.Pairing
	drive              komitake.DriveState
	driveModeEnabled   bool
	driveDisarmed      bool
	video              []komitake.VideoFrame
	videoStreamOptions chan komitake.VideoStreamOptions
}

func (f *fakeClient) Status(context.Context) (*komitake.Status, error) {
	return &komitake.Status{Mode: komitake.ModeNormal, Wireless: &komitake.Wireless{SSID: "Gtest"}}, nil
}
func (f *fakeClient) Karts(context.Context) ([]komitake.Kart, error)          { return f.karts, nil }
func (f *fakeClient) StartPairing(context.Context) (*komitake.Pairing, error) { return f.pairing, nil }
func (f *fakeClient) StopPairing(context.Context) error                       { return nil }
func (f *fakeClient) AwaitPairing(context.Context) error                      { return nil }
func (f *fakeClient) SetDrive(_ context.Context, selector string, steer, throttle, brake float64) (*komitake.DriveState, error) {
	f.drive = komitake.DriveState{
		DeviceID: selector, Steer: steer, Throttle: throttle, Brake: brake,
		Applied: !f.driveDisarmed, UpdatedAt: time.Now().UTC(),
	}
	if f.driveDisarmed {
		f.drive.Reason = "Fuji drive state is not active"
	}
	return &f.drive, nil
}
func (f *fakeClient) GetDrive(_ context.Context, selector string) (*komitake.DriveState, error) {
	state := f.drive
	if state.DeviceID == "" {
		state.DeviceID = selector
	}
	if f.driveDisarmed {
		state.Applied = false
		state.Reason = "Fuji drive state is not active"
	}
	return &state, nil
}
func (f *fakeClient) SetDriveMode(_ context.Context, selector string, enabled bool) (*komitake.Kart, error) {
	f.driveModeEnabled = enabled
	f.driveDisarmed = !enabled
	if !enabled {
		f.drive.Steer = 0
		f.drive.Throttle = 0
		f.drive.Brake = 0
		f.drive.Applied = false
		f.drive.Reason = "Fuji drive state is not active"
		f.drive.UpdatedAt = time.Now().UTC()
	}
	if f.drive.DeviceID == "" {
		f.drive.DeviceID = selector
	}
	for index := range f.karts {
		if f.karts[index].Ident == selector {
			f.karts[index].DriveArmed = enabled
			return &f.karts[index], nil
		}
	}
	return &komitake.Kart{Ident: selector, DriveArmed: enabled}, nil
}

func (f *fakeClient) ShutdownKart(_ context.Context, selector string) (*komitake.Kart, error) {
	for index := range f.karts {
		if f.karts[index].Ident == selector {
			kart := f.karts[index]
			f.karts = append(f.karts[:index], f.karts[index+1:]...)
			f.driveDisarmed = true
			f.driveModeEnabled = false
			return &kart, nil
		}
	}
	return &komitake.Kart{Ident: selector}, nil
}

func (f *fakeClient) StreamVideo(ctx context.Context, selector string) (komitake.VideoReceiver, error) {
	return f.StreamVideoWithOptions(ctx, selector, komitake.VideoStreamOptions{})
}

func (f *fakeClient) StreamVideoWithOptions(ctx context.Context, _ string, options komitake.VideoStreamOptions) (komitake.VideoReceiver, error) {
	if f.videoStreamOptions != nil {
		f.videoStreamOptions <- options
	}
	return &fakeVideoReceiver{ctx: ctx, frames: append([]komitake.VideoFrame(nil), f.video...)}, nil
}

type fakeVideoReceiver struct {
	ctx    context.Context
	frames []komitake.VideoFrame
}

func (receiver *fakeVideoReceiver) Recv() (komitake.VideoFrame, error) {
	if len(receiver.frames) > 0 {
		frame := receiver.frames[0]
		receiver.frames = receiver.frames[1:]
		return frame, nil
	}
	<-receiver.ctx.Done()
	return komitake.VideoFrame{}, io.EOF
}

func newServer(c Client) *httptest.Server {
	mux := http.NewServeMux()
	RegisterAPI(mux, c)
	return httptest.NewServer(mux)
}

func TestKartsEndpoint(t *testing.T) {
	sig := -47
	cableConnected := false
	c := &fakeClient{karts: []komitake.Kart{{Kind: "Fuji", Ident: "abc", Serial: "XKW123456789", Address: "10.0.0.2", MACAddress: "00:11:22:33:44:55", SignalDBM: &sig, CableConnected: &cableConnected}}}
	srv := newServer(c)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/karts")
	if err != nil {
		t.Fatalf("GET /v1/karts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body struct {
		Karts []kartDTO `json:"karts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Karts) != 1 || body.Karts[0].Ident != "abc" || body.Karts[0].Serial != "XKW123456789" || body.Karts[0].SignalDBM == nil || body.Karts[0].CableConnected == nil || *body.Karts[0].CableConnected {
		t.Fatalf("unexpected karts: %+v", body.Karts)
	}
}

func TestDriveEndpoint(t *testing.T) {
	c := &fakeClient{karts: []komitake.Kart{{Kind: "Fuji", Ident: "abc", Serial: "XKW1"}}}
	srv := newServer(c)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/karts/by-id/abc/drive", "application/json",
		strings.NewReader(`{"steer":0.5,"throttle":0.25,"brake":0}`))
	if err != nil {
		t.Fatalf("POST drive: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body DriveState
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.DeviceID != "abc" || body.Steer != 0.5 || body.Throttle != 0.25 || !body.Applied {
		t.Fatalf("unexpected drive state: %+v", body)
	}
}

func TestDriveModeEndpoint(t *testing.T) {
	t.Parallel()
	client := &fakeClient{karts: []komitake.Kart{{Kind: "Fuji", Ident: "abc"}}}
	server := newServer(client)
	defer server.Close()

	request, err := http.NewRequest(http.MethodPut, server.URL+"/v1/karts/by-id/abc/drive-mode", strings.NewReader(`{"enabled":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var body kartDTO
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !client.driveModeEnabled || !body.DriveArmed {
		t.Fatalf("client enabled=%v body=%+v", client.driveModeEnabled, body)
	}
}

func TestShutdownEndpoint(t *testing.T) {
	t.Parallel()
	client := &fakeClient{karts: []komitake.Kart{{Kind: "Fuji", Ident: "abc", Serial: "XKW1"}}}
	server := newServer(client)
	defer server.Close()

	response, err := http.Post(server.URL+"/v1/karts/by-id/abc/shutdown", "application/json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var body kartDTO
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Ident != "abc" || body.Serial != "XKW1" {
		t.Fatalf("body = %+v", body)
	}
	if len(client.karts) != 0 {
		t.Fatalf("karts = %+v, want removed", client.karts)
	}
}

func TestKartsBySerialRedirect(t *testing.T) {
	c := &fakeClient{karts: []komitake.Kart{{Kind: "Fuji", Ident: "aabbccddeeff00112233445566778899", Serial: "XKW123456789", MACAddress: "e8:a0:cd:13:7f:ac"}}}
	srv := newServer(c)
	defer srv.Close()

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(srv.URL + "/v1/karts/by-serial/XKW123")
	if err != nil {
		t.Fatalf("GET by-serial: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	want := "/v1/karts/by-id/aabbccddeeff00112233445566778899"
	if loc != want {
		t.Fatalf("Location = %q, want %q", loc, want)
	}

	resp2, err := client.Get(srv.URL + "/v1/karts/by-serial/XKW123/extra?x=1")
	if err != nil {
		t.Fatalf("GET by-serial rest: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d", resp2.StatusCode)
	}
	if got := resp2.Header.Get("Location"); got != want+"/extra?x=1" {
		t.Fatalf("Location = %q", got)
	}

	resp3, err := http.Get(srv.URL + want)
	if err != nil {
		t.Fatalf("GET by-id: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("by-id status = %d", resp3.StatusCode)
	}
}

func TestPairEndpointReturnsQRPayload(t *testing.T) {
	c := &fakeClient{pairing: &komitake.Pairing{
		SSID: "Ptest", Channel: 6,
		Seed: "11111111111111111111111111111111", // 16 bytes hex
	}}
	srv := newServer(c)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/v1/karts/pair", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST /v1/karts/pair: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var body pairingDTO
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.SSID != "Ptest" || len(body.QRPayload) != 0x3e {
		t.Fatalf("unexpected pairing dto: ssid=%q payload=%d", body.SSID, len(body.QRPayload))
	}
}

// The plugin tree must compose into the expected nested paths.
func TestPluginTreeRoutes(t *testing.T) {
	sig := -40
	c := &fakeClient{
		karts:   []komitake.Kart{{Kind: "Fuji", Ident: "abc", SignalDBM: &sig}},
		pairing: &komitake.Pairing{SSID: "Ptest", Channel: 6, Seed: "11111111111111111111111111111111"},
	}
	srv := newServer(c)
	defer srv.Close()

	cases := []struct {
		method, path string
		body         string
		want         int
	}{
		{http.MethodGet, "/v1/status", "", http.StatusOK},
		{http.MethodGet, "/v1/karts", "", http.StatusOK},
		{http.MethodGet, "/v1/karts/by-id/abc", "", http.StatusOK},
		{http.MethodGet, "/v1/karts/by-id/missing", "", http.StatusNotFound},
		{http.MethodGet, "/v1/karts/abc", "", http.StatusNotFound},
		{http.MethodGet, "/v1/devices", "", http.StatusNotFound},
		{http.MethodGet, "/v1/devices/by-id/abc", "", http.StatusNotFound},
		{http.MethodPost, "/v1/devices/by-id/abc/drive", "{}", http.StatusNotFound},
		{http.MethodPost, "/v1/karts/pair", "{}", http.StatusAccepted},
		{http.MethodPost, "/v1/karts/pair/stop", "", http.StatusNoContent},
	}
	for _, tc := range cases {
		req, err := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader(tc.body))
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("%s %s = %d, want %d", tc.method, tc.path, resp.StatusCode, tc.want)
		}
	}
}

// The composed OpenAPI document must advertise the nested paths.
func TestOpenAPIPaths(t *testing.T) {
	srv := newServer(&fakeClient{})
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/openapi.json")
	if err != nil {
		t.Fatalf("GET /openapi.json: %v", err)
	}
	defer resp.Body.Close()
	var doc struct {
		Paths map[string]any `json:"paths"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, want := range []string{
		"/v1/status",
		"/v1/karts",
		"/v1/karts/by-id/{id}",
		"/v1/karts/by-serial/{serial}",
		"/v1/karts/by-id/{id}/drive",
		"/v1/karts/by-id/{id}/drive-mode",
		"/v1/karts/by-id/{id}/shutdown",
		"/v1/karts/pair",
		"/v1/karts/pair/stop",
		"/v1/settings",
	} {
		if _, ok := doc.Paths[want]; !ok {
			t.Errorf("openapi missing path %q; have %v", want, keys(doc.Paths))
		}
	}
	for advertisedPath := range doc.Paths {
		if strings.HasPrefix(advertisedPath, "/v1/devices") {
			t.Errorf("openapi still advertises legacy path %q", advertisedPath)
		}
	}
	if _, ok := doc.Paths["/v1/karts/{ident}"]; ok {
		t.Error("openapi still advertises legacy flat kart path")
	}
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestSettingsEndpointsPreserveConfig(t *testing.T) {
	t.Parallel()
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "secret": "keep-me",
  "wireless": {"interface": "wlan0", "address": "192.168.1.1/24"}
}
`), 0o640); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterAPI(mux, &fakeClient{}, Options{ConfigPath: configPath})
	server := httptest.NewServer(mux)
	defer server.Close()

	response, err := http.Get(server.URL + "/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET status = %d", response.StatusCode)
	}

	request, err := http.NewRequest(http.MethodPut, server.URL+"/v1/settings", strings.NewReader(`{
  "web": {"bind": "0.0.0.0:8080"},
  "socket": {"bind": "unix:/tmp/k.sock", "chmod": "0770"},
  "video": {"hwaccel": "auto"}
}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	updateResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer updateResponse.Body.Close()
	if updateResponse.StatusCode != http.StatusOK {
		t.Fatalf("PUT status = %d", updateResponse.StatusCode)
	}
	body, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, wanted := range []string{`"secret": "keep-me"`, `"bind": "0.0.0.0:8080"`, `"chmod": "0770"`} {
		if !strings.Contains(string(body), wanted) {
			t.Fatalf("missing %s in config:\n%s", wanted, body)
		}
	}
}

func TestWebSocketDriveMessageAppliesAndBroadcasts(t *testing.T) {
	t.Parallel()
	client := &fakeClient{}
	hub := NewHub()
	subscriber := hub.Subscribe()
	defer hub.Unsubscribe(subscriber)
	handleWSClientMessage(context.Background(), []byte(`{
  "type": "drive",
  "device_id": "aabb",
  "steer": 0.25,
  "throttle": 0.5,
  "brake": 0
}`), client, hub)
	select {
	case payload := <-subscriber:
		var message struct {
			Type  string     `json:"type"`
			Drive DriveState `json:"drive"`
		}
		if err := json.Unmarshal(payload, &message); err != nil {
			t.Fatal(err)
		}
		if message.Type != "drive" || !message.Drive.Applied || message.Drive.DeviceID != "aabb" {
			t.Fatalf("message = %+v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("no drive broadcast")
	}
}

func TestDeviceWebSocketStreamsTelemetryAndDrive(t *testing.T) {
	t.Parallel()
	battery := 3
	cableConnected := true
	client := &fakeClient{karts: []komitake.Kart{{
		Kind: "Fuji", Ident: "aabb", Battery: &battery, CableConnected: &cableConnected, DriveArmed: true,
		AccelMPS2:   &komitake.Vec3{X: 1, Y: 2, Z: 3},
		GyroDPS:     &komitake.Vec3{X: 4, Y: 5, Z: 6},
		Orientation: &komitake.Quat{R: 1},
	}}}
	mux := http.NewServeMux()
	hub := NewHub()
	registerRealtime(mux, client, hub)
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + server.URL[len("http"):] + "/v1/karts/by-id/aabb/ws"
	connection, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	var telemetry testTelemetryPayload
	for telemetry.Battery == nil {
		_, payload, readErr := connection.Read(ctx)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var message struct {
			Type      string               `json:"type"`
			Telemetry testTelemetryPayload `json:"telemetry"`
		}
		if json.Unmarshal(payload, &message) == nil && message.Type == "telemetry" {
			telemetry = message.Telemetry
		}
	}
	if *telemetry.Battery != 3 || telemetry.CableConnected == nil || !*telemetry.CableConnected || telemetry.AccelMPS2 == nil || !telemetry.DriveArmed {
		t.Fatalf("telemetry = %+v", telemetry)
	}
	if err := connection.Write(ctx, websocket.MessageText, []byte(`{"type":"drive","steer":0.5,"throttle":1}`)); err != nil {
		t.Fatal(err)
	}
	for {
		_, payload, readErr := connection.Read(ctx)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var message struct {
			Type  string     `json:"type"`
			Drive DriveState `json:"drive"`
		}
		if json.Unmarshal(payload, &message) == nil && message.Type == "drive" {
			if !message.Drive.Applied || message.Drive.Throttle != 1 {
				t.Fatalf("drive = %+v", message.Drive)
			}
			break
		}
	}
	if err := connection.Write(ctx, websocket.MessageText, []byte(`{"type":"drive-mode","enabled":false}`)); err != nil {
		t.Fatal(err)
	}
	disarmed := false
	driveCleared := false
	for !disarmed || !driveCleared {
		_, payload, readErr := connection.Read(ctx)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var message struct {
			Type      string               `json:"type"`
			Telemetry testTelemetryPayload `json:"telemetry"`
			Drive     DriveState           `json:"drive"`
		}
		if json.Unmarshal(payload, &message) != nil {
			continue
		}
		switch message.Type {
		case "telemetry":
			if message.Telemetry.DriveArmed {
				t.Fatalf("telemetry = %+v", message.Telemetry)
			}
			disarmed = true
		case "drive":
			if message.Drive.Applied || message.Drive.Steer != 0 || message.Drive.Throttle != 0 {
				t.Fatalf("drive = %+v", message.Drive)
			}
			driveCleared = true
		}
	}
	if client.driveModeEnabled {
		t.Fatal("drive mode remained enabled")
	}
}

func TestWebRTCOfferStreamsSharedH264Track(t *testing.T) {
	t.Parallel()
	client := &fakeClient{
		karts:              []komitake.Kart{{Kind: "Fuji", Ident: "aabb"}},
		video:              []komitake.VideoFrame{{DeviceID: "aabb", Sequence: 1, KeyFrame: true, AnnexB: []byte{0, 0, 0, 1, 0x67, 0x64, 0, 0x20, 0, 0, 0, 1, 0x68, 0xee, 0, 0, 0, 1, 0x65, 0x80}}},
		videoStreamOptions: make(chan komitake.VideoStreamOptions, 1),
	}
	mux := http.NewServeMux()
	registerWebRTC(mux, client)
	server := httptest.NewServer(mux)
	defer server.Close()
	browser, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	_, err = browser.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{Direction: webrtc.RTPTransceiverDirectionRecvonly})
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan struct{}, 1)
	browser.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		go func() {
			if _, _, readErr := track.ReadRTP(); readErr == nil {
				received <- struct{}{}
			}
		}()
	})
	offer, err := browser.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	gather := webrtc.GatheringCompletePromise(browser)
	if err = browser.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	<-gather
	body, _ := json.Marshal(webRTCOffer{Type: "offer", SDP: browser.LocalDescription().SDP})
	response, err := http.Post(server.URL+"/v1/karts/by-id/aabb/webrtc", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("status=%d body=%s", response.StatusCode, payload)
	}
	var answer webRTCOffer
	if err = json.NewDecoder(response.Body).Decode(&answer); err != nil {
		t.Fatal(err)
	}
	if err = browser.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer.SDP}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("no WebRTC H264 RTP received")
	}
	select {
	case options := <-client.videoStreamOptions:
		if !options.FreshKeyFrame {
			t.Fatal("WebRTC stream did not request a fresh keyframe")
		}
	default:
		t.Fatal("WebRTC stream options were not recorded")
	}
}

func TestDeviceWebSocketStreamsBinaryVideoEnvelope(t *testing.T) {
	t.Parallel()
	client := &fakeClient{karts: []komitake.Kart{{Kind: "Fuji", Ident: "aabb"}}, video: []komitake.VideoFrame{{DeviceID: "aabb", Sequence: 42, KeyFrame: true, AnnexB: []byte{0, 0, 0, 1, 0x65}}}}
	mux := http.NewServeMux()
	registerRealtime(mux, client, NewHub())
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, "ws"+server.URL[len("http"):]+"/v1/karts/by-id/aabb/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	for {
		messageType, payload, readErr := connection.Read(ctx)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if messageType != websocket.MessageBinary {
			continue
		}
		if len(payload) != videoEnvelopeHeaderSize+5 || string(payload[:4]) != "KTV1" {
			t.Fatalf("payload=%x", payload)
		}
		if payload[4]&1 == 0 || binary.BigEndian.Uint64(payload[8:16]) != 42 {
			t.Fatalf("header=%x", payload[:16])
		}
		return
	}
}

func TestWebRTCOfferRejectsInvalidSDP(t *testing.T) {
	t.Parallel()
	client := &fakeClient{karts: []komitake.Kart{{Kind: "Fuji", Ident: "aabb"}}}
	mux := http.NewServeMux()
	registerWebRTC(mux, client)
	server := httptest.NewServer(mux)
	defer server.Close()
	response, err := http.Post(server.URL+"/v1/karts/by-id/aabb/webrtc", "application/json", strings.NewReader(`{"type":"offer","sdp":"bad"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", response.StatusCode)
	}
}

func TestDeviceWebSocketCanDisableBinaryVideo(t *testing.T) {
	t.Parallel()
	client := &fakeClient{karts: []komitake.Kart{{Kind: "Fuji", Ident: "aabb"}}, video: []komitake.VideoFrame{{DeviceID: "aabb", Sequence: 1, KeyFrame: true, AnnexB: []byte{1}}}}
	mux := http.NewServeMux()
	registerRealtime(mux, client, NewHub())
	server := httptest.NewServer(mux)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, "ws"+server.URL[len("http"):]+"/v1/karts/by-id/aabb/ws?video=0", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "")
	for count := 0; count < 2; count++ {
		messageType, _, readErr := connection.Read(ctx)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if messageType == websocket.MessageBinary {
			t.Fatal("control-only WebSocket received video")
		}
	}
}
