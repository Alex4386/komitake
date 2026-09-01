package rcd

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/Alex4386/komitake/internal/logging"
)

const headerSize = 16

type Message struct {
	Service    uint16
	Command    uint16
	Status     uint32
	IsResponse bool
	Data       []byte
}

// MaxPayloadSize bounds the declared payload length of a single message. The
// value is enforced on decode so an attacker-supplied length cannot drive a
// huge allocation or an out-of-range slice.
const MaxPayloadSize = 0x1000

// ErrPayloadTooLarge reports a declared payload length beyond MaxPayloadSize.
var ErrPayloadTooLarge = errors.New("rcd: payload length exceeds maximum")

// MessageSize reports the total wire size of the message that starts at buf. It
// returns ErrUnexpectedEOF when buf is too short to contain a header, and
// ErrPayloadTooLarge when the declared length is out of range.
func MessageSize(buf []byte) (int, error) {
	if len(buf) < headerSize {
		return 0, io.ErrUnexpectedEOF
	}
	// Compare as uint32 before converting to int: on 32-bit builds
	// headerSize + int(dataSize) can overflow to a small or negative value,
	// which would defeat the length check in DecodeMessage.
	dataSize := binary.BigEndian.Uint32(buf[4:8])
	if dataSize > MaxPayloadSize {
		return 0, fmt.Errorf("%w: %d", ErrPayloadTooLarge, dataSize)
	}
	return headerSize + int(dataSize), nil
}

func DecodeMessage(buf []byte) (Message, error) {
	size, err := MessageSize(buf)
	if err != nil {
		return Message{}, err
	}
	if len(buf) < size {
		return Message{}, io.ErrUnexpectedEOF
	}

	flags := buf[12]
	msg := Message{
		Service:    binary.BigEndian.Uint16(buf[0:2]),
		Command:    binary.BigEndian.Uint16(buf[2:4]),
		Status:     binary.BigEndian.Uint32(buf[8:12]),
		IsResponse: flags&1 == 1,
		Data:       append([]byte(nil), buf[headerSize:size]...),
	}
	return msg, nil
}

func EncodeMessage(msg Message) []byte {
	buf := make([]byte, headerSize+len(msg.Data))
	binary.BigEndian.PutUint16(buf[0:2], msg.Service)
	binary.BigEndian.PutUint16(buf[2:4], msg.Command)
	binary.BigEndian.PutUint32(buf[4:8], uint32(len(msg.Data)))
	binary.BigEndian.PutUint32(buf[8:12], msg.Status)
	if msg.IsResponse {
		buf[12] = 1
	}
	copy(buf[headerSize:], msg.Data)
	return buf
}

type Conn struct {
	net.Conn
	reader *bufio.Reader
	mu     sync.Mutex
}

func NewConn(conn net.Conn) *Conn {
	return &Conn{Conn: conn, reader: bufio.NewReader(conn)}
}

func (c *Conn) ReadMessage() (Message, error) {
	// Read the header into a buffer sized for the whole message so the payload
	// can be appended without a second allocation and copy.
	buf := make([]byte, headerSize, headerSize+256)
	if _, err := io.ReadFull(c.reader, buf); err != nil {
		return Message{}, err
	}

	size, err := MessageSize(buf)
	if err != nil {
		return Message{}, err
	}

	if size > headerSize {
		buf = append(buf, make([]byte, size-headerSize)...)
		if _, err := io.ReadFull(c.reader, buf[headerSize:]); err != nil {
			return Message{}, err
		}
	}

	return DecodeMessage(buf)
}

func (c *Conn) WriteMessage(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.Write(EncodeMessage(msg))
	return err
}

func (c *Conn) Host() string {
	if c == nil || c.Conn == nil || c.RemoteAddr() == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(c.RemoteAddr().String())
	if err != nil {
		return c.RemoteAddr().String()
	}
	return host
}

type Error struct {
	Code uint32
}

func (e *Error) Error() string {
	if name := ErrorName(e.Code); name != "" {
		return fmt.Sprintf("rcd error %s (0x%x)", name, e.Code)
	}
	return fmt.Sprintf("rcd error 0x%x", e.Code)
}

const (
	ErrCodeMissizedPayload             = 0x708e8
	ErrCodeBadRequest                  = 0x710e8
	ErrCodeHandshakeVersionMismatch    = 0x800e8
	ErrCodeHandshakeSequence           = 0x810e8
	ErrCodeHandshakeBadVersions        = 0x820e8
	ErrCodeHandshakeHash               = 0x830e8
	ErrCodeHandshakeUnrecognizedDevice = 0x850e8

	// Fuji control-service codes, documented by switchbrew. These are produced
	// by the kart rather than this daemon, and are named here so they appear
	// readably in logs instead of as bare hex.
	ErrCodeParamUnrecognized = 0x1060e8
	ErrCodeParamState        = 0x1040e8
)

// errorNames maps status codes to symbolic names for log output.
var errorNames = map[uint32]string{
	ErrCodeMissizedPayload:             "MISSIZED_PAYLOAD",
	ErrCodeBadRequest:                  "BAD_REQUEST",
	ErrCodeHandshakeVersionMismatch:    "HANDSHAKE_VERSION_MISMATCH",
	ErrCodeHandshakeSequence:           "HANDSHAKE_SEQUENCE",
	ErrCodeHandshakeBadVersions:        "HANDSHAKE_BAD_VERSIONS",
	ErrCodeHandshakeHash:               "HANDSHAKE_HASH",
	ErrCodeHandshakeUnrecognizedDevice: "HANDSHAKE_UNRECOGNIZED_DEVICE",
	ErrCodeParamUnrecognized:           "PARAM_UNRECOGNIZED",
	ErrCodeParamState:                  "PARAM_STATE",
}

// ErrorName returns the symbolic name for a status code, or "" if unknown.
func ErrorName(code uint32) string {
	return errorNames[code]
}

func newError(code uint32) error {
	return &Error{Code: code}
}

type Client struct {
	conn *Conn
	mu   sync.Mutex

	// ExchangeTimeout overrides the package default when non-zero.
	ExchangeTimeout time.Duration

	logger *logging.Logger

	// sensitiveServices lists service IDs whose payloads must never be dumped,
	// because they carry key material on the wire. Trace output falls back to a
	// length and fingerprint for these.
	sensitiveServices map[uint16]bool
}

// MarkSensitiveService suppresses payload dumps for a service ID. The Fuji
// pairing service carries the raw 32-byte PSK in its request, so tracing it
// verbatim would write the link key to the log.
func (c *Client) MarkSensitiveService(service uint16) {
	if c.sensitiveServices == nil {
		c.sensitiveServices = map[uint16]bool{}
	}
	c.sensitiveServices[service] = true
}

// payloadAttr renders a payload for trace output, redacting services marked
// sensitive.
func (c *Client) payloadAttr(service uint16, data []byte) slog.Attr {
	if c.sensitiveServices[service] {
		return logging.Secret("payload", data)
	}
	return logging.Dump("payload", data)
}

// log returns the client's logger, falling back to the process default so a
// zero-value Client is still usable. It must not lazily assign: Invoke can run
// concurrently with SetLogger from another goroutine.
func (c *Client) log() *logging.Logger {
	if c.logger == nil {
		return logging.New(nil).With("component", "rcd-client")
	}
	return c.logger
}

// SetLogger attaches a component logger.
func (c *Client) SetLogger(l *logging.Logger) {
	c.logger = l
}

func (c *Client) exchangeTimeout() time.Duration {
	if c.ExchangeTimeout > 0 {
		return c.ExchangeTimeout
	}
	return ExchangeTimeout
}

func DialClient(ctx context.Context, network string, address string) (*Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	return &Client{conn: NewConn(conn)}, nil
}

func NewClientFromConn(conn net.Conn) *Client {
	return &Client{conn: NewConn(conn)}
}

func (c *Client) Close() error {
	return c.conn.Close()
}

// ExchangeTimeout bounds a single request/response exchange when the caller's
// context carries no deadline, so Invoke cannot block forever.
const ExchangeTimeout = 5 * time.Second

func (c *Client) Invoke(ctx context.Context, service uint16, command uint16, data []byte) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// A stalled control channel makes the kart reset its network connection, so
	// always bound the exchange even when the caller's context has no deadline.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.exchangeTimeout())
	}
	if err := c.conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	defer func() { _ = c.conn.SetDeadline(timeZero) }()

	// Closing the conn is the only way to interrupt a blocking read on cancel.
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		select {
		case <-ctx.Done():
			_ = c.conn.Close()
		case <-stopWatch:
		}
	}()

	log := c.log()
	started := time.Now()

	req := Message{
		Service: service,
		Command: command,
		Data:    append([]byte(nil), data...),
	}

	if log.TraceEnabled() {
		log.Trace("invoke request",
			"service", hexU16(service), "command", command,
			"peer", c.conn.Host(),
			logging.Size("payload_bytes", req.Data),
			c.payloadAttr(service, req.Data))
	}

	if err := c.conn.WriteMessage(req); err != nil {
		log.Debug("invoke write failed",
			"service", hexU16(service), "command", command, "error", err)
		return nil, err
	}
	resp, err := c.conn.ReadMessage()
	if err != nil {
		log.Debug("invoke read failed",
			"service", hexU16(service), "command", command,
			"elapsed", time.Since(started), "error", err)
		return nil, err
	}

	if log.TraceEnabled() {
		log.Trace("invoke response",
			"service", hexU16(resp.Service), "command", resp.Command,
			"status", hexU32(resp.Status), "elapsed", time.Since(started),
			logging.Size("payload_bytes", resp.Data),
			c.payloadAttr(resp.Service, resp.Data))
	}

	if !resp.IsResponse || resp.Service != service || resp.Command != command {
		log.Warn("mismatched rcd response",
			"want_service", hexU16(service), "want_command", command,
			"got_service", hexU16(resp.Service), "got_command", resp.Command,
			"is_response", resp.IsResponse)
		return nil, errors.New("invalid rcd response")
	}
	if resp.Status != 0 {
		log.Debug("rcd error status",
			"service", hexU16(service), "command", command,
			"status", hexU32(resp.Status), "name", ErrorName(resp.Status))
		return nil, &Error{Code: resp.Status}
	}
	return resp.Data, nil
}

func hexU16(v uint16) string { return fmt.Sprintf("%#06x", v) }
func hexU32(v uint32) string { return fmt.Sprintf("%#x", v) }

var timeZero = func() time.Time { return time.Time{} }()

type Service interface {
	ServiceID() uint16
	Handle(ctx context.Context, channel *Conn, msg Message) ([]byte, error)
	Close() error
}

// IdleTimeout bounds a connection until it establishes a session, so a silent
// peer cannot pin a goroutine forever. It is dropped once established: the kart
// resets its whole network connection if an open RCD channel is lost, so an
// established channel must be allowed to sit idle.
const IdleTimeout = 30 * time.Second

// establisher lets a service signal that its session is up and the idle
// deadline no longer applies. HandshakeService implements it.
type establisher interface {
	Established() bool
}

type Server struct {
	conn     *Conn
	services map[uint16]Service

	// IdleTimeout overrides the package default when non-zero.
	IdleTimeout time.Duration

	logger            *logging.Logger
	sensitiveServices map[uint16]bool
}

// SetLogger attaches a component logger.
func (s *Server) SetLogger(l *logging.Logger) {
	s.logger = l
	for _, service := range s.services {
		if ls, ok := service.(interface{ SetLogger(*logging.Logger) }); ok {
			ls.SetLogger(l)
		}
	}
}

// sensitiveResponse reports whether a response payload carries key material and
// must not be dumped. Handshake command 3 returns the derived secret key.
func sensitiveResponse(service uint16, command uint16) bool {
	return service == handshakeServiceID && command == 3
}

// MarkSensitiveService suppresses payload dumps for requests to a service ID.
func (s *Server) MarkSensitiveService(service uint16) {
	if s.sensitiveServices == nil {
		s.sensitiveServices = map[uint16]bool{}
	}
	s.sensitiveServices[service] = true
}

func (s *Server) sensitiveRequest(service uint16) bool {
	return s.sensitiveServices[service]
}

func (s *Server) log() *logging.Logger {
	if s.logger == nil {
		return logging.New(nil).With("component", "rcd-server")
	}
	return s.logger
}

// established reports whether any registered service has an active session.
func (s *Server) established() bool {
	for _, service := range s.services {
		if e, ok := service.(establisher); ok && e.Established() {
			return true
		}
	}
	return false
}

func NewServer(conn net.Conn, services ...Service) *Server {
	table := make(map[uint16]Service, len(services))
	for _, service := range services {
		table[service.ServiceID()] = service
	}
	return &Server{
		conn:     NewConn(conn),
		services: table,
	}
}

func (s *Server) idleTimeout() time.Duration {
	if s.IdleTimeout > 0 {
		return s.IdleTimeout
	}
	return IdleTimeout
}

func (s *Server) Serve(ctx context.Context) error {
	log := s.log().With("peer", s.conn.Host())
	started := time.Now()
	var messages int

	log.Debug("serving rcd connection", "services", s.serviceIDs())

	defer func() {
		for _, service := range s.services {
			_ = service.Close()
		}
		_ = s.conn.Close()
		log.Debug("rcd connection closed",
			"messages", messages, "duration", time.Since(started))
	}()

	// Closing the conn is the only way to interrupt a blocking read on cancel.
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		select {
		case <-ctx.Done():
			_ = s.conn.Close()
		case <-stopWatch:
		}
	}()

	timeout := s.idleTimeout()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Only bound reads until a session is established; see IdleTimeout.
		deadline := time.Time{}
		if !s.established() {
			deadline = time.Now().Add(timeout)
		}
		if err := s.conn.SetReadDeadline(deadline); err != nil {
			return err
		}

		msg, err := s.conn.ReadMessage()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				log.Debug("serve canceled", "error", ctxErr)
				return ctxErr
			}
			if errors.Is(err, io.EOF) {
				log.Debug("peer closed the connection", "messages", messages)
				return nil
			}
			log.Debug("read failed", "messages", messages, "error", err)
			return err
		}
		messages++

		if log.TraceEnabled() {
			// Requests to a sensitive service are redacted as well: the Fuji
			// pairing request carries a raw PSK, and this server type is generic.
			payload := logging.Dump("payload", msg.Data)
			if s.sensitiveRequest(msg.Service) {
				payload = logging.Secret("payload", msg.Data)
			}
			log.Trace("received message",
				"service", hexU16(msg.Service), "command", msg.Command,
				"is_response", msg.IsResponse, "seq", messages,
				logging.Size("payload_bytes", msg.Data), payload)
		}

		response := Message{
			Service:    msg.Service,
			Command:    msg.Command,
			IsResponse: true,
		}

		if err := s.conn.SetWriteDeadline(time.Now().Add(timeout)); err != nil {
			return err
		}

		service := s.services[msg.Service]
		if msg.IsResponse || service == nil {
			log.Warn("rejecting unroutable message",
				"service", hexU16(msg.Service), "command", msg.Command,
				"is_response", msg.IsResponse, "known_services", s.serviceIDs())
			response.Status = ErrCodeBadRequest
			if err := s.conn.WriteMessage(response); err != nil {
				return err
			}
			continue
		}

		handleStarted := time.Now()
		data, err := service.Handle(ctx, s.conn, msg)
		if err != nil {
			var rcdErr *Error
			if errors.As(err, &rcdErr) {
				log.Debug("service returned a protocol error",
					"service", hexU16(msg.Service), "command", msg.Command,
					"status", hexU32(rcdErr.Code), "name", ErrorName(rcdErr.Code))
				response.Status = rcdErr.Code
			} else {
				log.Error("service returned an unexpected error",
					"service", hexU16(msg.Service), "command", msg.Command, "error", err)
				// Report a protocol error rather than dropping the connection
				// with no reply, which left the peer waiting for its own
				// timeout.
				response.Status = ErrCodeBadRequest
			}
		} else {
			response.Data = data
		}

		if log.TraceEnabled() {
			payload := logging.Dump("payload", response.Data)
			if sensitiveResponse(response.Service, response.Command) {
				payload = logging.Secret("payload", response.Data)
			}
			log.Trace("sending response",
				"service", hexU16(response.Service), "command", response.Command,
				"status", hexU32(response.Status), "handler_elapsed", time.Since(handleStarted),
				logging.Size("payload_bytes", response.Data), payload)
		}

		if err := s.conn.WriteMessage(response); err != nil {
			log.Debug("write failed", "error", err)
			return err
		}
	}
}

// serviceIDs lists the registered service IDs for diagnostics.
func (s *Server) serviceIDs() []string {
	out := make([]string, 0, len(s.services))
	for id := range s.services {
		out = append(out, hexU16(id))
	}
	sort.Strings(out)
	return out
}

type Device struct {
	Name      string
	Ident     []byte
	PairingID []byte
	SecretKey []byte
	Address   string
	Version   uint8
	channel   *Conn
}

// Close is nil-receiver safe so callers can invoke it unconditionally during
// cleanup without first checking the pointer.
func (d *Device) Close() error {
	if d == nil || d.channel == nil {
		return nil
	}
	return d.channel.Close()
}

type ServerInfo struct {
	Name      string
	Ident     []byte
	MasterKey []byte
	Versions  []uint8
}

func (s ServerInfo) Validate() error {
	if len(s.Ident) != 16 {
		return errors.New("rcd ident must be 16 bytes")
	}
	if len(s.MasterKey) == 0 {
		return errors.New("rcd master key must not be empty")
	}
	if len([]byte(s.Name)) >= 16 {
		return errors.New("rcd name must be < 16 bytes")
	}
	if len(s.Versions) == 0 {
		return errors.New("rcd versions must not be empty")
	}
	return nil
}

func (s ServerInfo) PairingKeys(deviceIdent []byte, deviceName string) (pairingID []byte, secretKey []byte) {
	deviceID := append(append([]byte(nil), deviceIdent...), []byte(deviceName)...)
	pairing := sha256.Sum256(append(append(append([]byte(nil), s.MasterKey...), deviceID...), s.MasterKey...))
	secret := sha512.Sum512(append(append(append([]byte(nil), s.MasterKey...), pairing[:]...), s.MasterKey...))
	return pairing[:], secret[:]
}

type DeviceHandler interface {
	DeviceConnected(*Device)
	DeviceDisconnected(*Device)
}

type HandshakeService struct {
	serverInfo ServerInfo
	handler    DeviceHandler
	pairing    bool
	transcript transcript
	device     *Device
	nextCmd    uint16
	done       bool
	logger     *logging.Logger
}

// SetLogger attaches a component logger.
func (s *HandshakeService) SetLogger(l *logging.Logger) {
	s.logger = l
}

func (s *HandshakeService) log() *logging.Logger {
	if s.logger == nil {
		return logging.New(nil).With("component", "rcd-handshake", "pairing", s.pairing)
	}
	return s.logger
}

func NewHandshakeService(serverInfo ServerInfo, handler DeviceHandler, pairing bool) *HandshakeService {
	return &HandshakeService{
		serverInfo: serverInfo,
		handler:    handler,
		pairing:    pairing,
		transcript: newTranscript(),
		nextCmd:    1,
	}
}

// handshakeServiceID is the RCD handshake service, per switchbrew.
const handshakeServiceID = 0x0001

func (s *HandshakeService) ServiceID() uint16 {
	return handshakeServiceID
}

func (s *HandshakeService) Handle(ctx context.Context, channel *Conn, msg Message) ([]byte, error) {
	var (
		resp []byte
		err  error
	)

	log := s.log()
	log.Debug("handshake step",
		"command", msg.Command, "name", handshakeCommandName(msg.Command),
		"expecting", s.nextCmd, logging.Size("payload_bytes", msg.Data))

	switch msg.Command {
	case 1:
		resp, err = s.beginHandshake(channel, msg.Data)
	case 2:
		resp, err = s.negotiateVersion(msg.Data)
	case 3:
		resp, err = s.getSecretKey(msg.Data)
	case 4:
		resp, err = s.finalize(msg.Data)
	default:
		log.Warn("unknown handshake command", "command", msg.Command)
		err = newError(ErrCodeBadRequest)
	}
	if err != nil {
		var rcdErr *Error
		code := uint32(0)
		if errors.As(err, &rcdErr) {
			code = rcdErr.Code
		}
		log.Info("handshake step failed",
			"command", msg.Command, "name", handshakeCommandName(msg.Command),
			"status", hexU32(code), "status_name", ErrorName(code), "error", err)
		return nil, err
	}

	s.transcript.Append(msg.Data, resp)
	if log.TraceEnabled() {
		// The transcript digest is not secret: every input to it crosses the
		// wire in both directions. Logging it is what makes a mismatched-hash
		// failure diagnosable.
		log.Trace("handshake step ok",
			"command", msg.Command, "next", s.nextCmd,
			logging.Size("response_bytes", resp),
			logging.Dump("transcript_digest", s.transcript.FlushedDigest()))
	}
	_ = ctx
	return resp, nil
}

func handshakeCommandName(command uint16) string {
	switch command {
	case 1:
		return "begin"
	case 2:
		return "negotiate_version"
	case 3:
		return "get_secret_key"
	case 4:
		return "finalize"
	default:
		return "unknown"
	}
}

// Established reports whether the handshake completed. Once it has, the channel
// must stay open indefinitely: the kart treats a lost RCD connection as a signal
// to reset its whole network connection.
func (s *HandshakeService) Established() bool {
	return s.done
}

func (s *HandshakeService) Close() error {
	if s.done && s.handler != nil && s.device != nil {
		s.handler.DeviceDisconnected(s.device)
	}
	return nil
}

func (s *HandshakeService) beginHandshake(channel *Conn, data []byte) ([]byte, error) {
	if len(data) != 0x50 {
		return nil, newError(ErrCodeMissizedPayload)
	}
	if s.nextCmd != 1 {
		return nil, newError(ErrCodeHandshakeSequence)
	}
	if data[0] != 1 {
		return nil, newError(ErrCodeHandshakeVersionMismatch)
	}

	deviceNameBytes := data[0x10:0x20]
	deviceIdent := append([]byte(nil), data[0x20:0x30]...)
	if idx := bytesIndexByte(deviceNameBytes, 0); idx >= 0 {
		deviceNameBytes = deviceNameBytes[:idx]
	}
	deviceName := string(deviceNameBytes)
	pairingID, secretKey := s.serverInfo.PairingKeys(deviceIdent, deviceName)

	s.device = &Device{
		Name:      deviceName,
		Ident:     deviceIdent,
		PairingID: pairingID,
		SecretKey: secretKey,
		Address:   hostOf(channel),
		channel:   channel,
	}
	s.nextCmd = 2

	// The ident is a device identifier rather than a secret, so it is logged in
	// full; the pairing ID and secret key are derived from the master key and
	// are only fingerprinted.
	s.log().Info("device began handshake",
		"name", deviceName, "address", hostOf(channel),
		logging.Dump("ident", deviceIdent),
		logging.Secret("pairing_id", pairingID),
		logging.Secret("secret_key", secretKey))

	resp := padTo([]byte{1}, 0x10)
	resp = append(resp, padTo([]byte(s.serverInfo.Name), 0x10)...)
	resp = append(resp, s.serverInfo.Ident...)
	random := make([]byte, 0x20)
	if _, err := rand.Read(random); err != nil {
		// A silent all-zero nonce would be worse than failing the handshake.
		s.log().Error("failed to read handshake nonce entropy", "error", err)
		return nil, newError(ErrCodeBadRequest)
	}
	resp = append(resp, random...)
	return resp, nil
}

func (s *HandshakeService) negotiateVersion(data []byte) ([]byte, error) {
	if len(data) < 0x21 {
		return nil, newError(ErrCodeMissizedPayload)
	}
	if s.nextCmd != 2 {
		return nil, newError(ErrCodeHandshakeSequence)
	}

	numVersions := int(data[0x20])
	versions := data[0x21:]
	if len(versions) != numVersions {
		return nil, newError(ErrCodeMissizedPayload)
	}

	var selected uint8
	found := false
	for _, supported := range s.serverInfo.Versions {
		for _, offered := range versions {
			if supported == offered {
				selected = supported
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		s.log().Warn("no common protocol version",
			"offered", versions, "supported", s.serverInfo.Versions)
		return nil, newError(ErrCodeHandshakeBadVersions)
	}

	pairingID := data[:0x20]
	recognized := bytesEqual(pairingID, s.device.PairingID)
	if recognized {
		s.nextCmd = 4
	} else if !s.pairing {
		s.log().Info("rejecting unrecognized device in normal mode",
			"name", s.device.Name, "address", s.device.Address,
			logging.Secret("offered_pairing_id", pairingID),
			logging.Secret("expected_pairing_id", s.device.PairingID))
		return nil, newError(ErrCodeHandshakeUnrecognizedDevice)
	} else {
		s.nextCmd = 3
	}

	s.device.Version = selected

	s.log().Debug("negotiated version",
		"selected", selected, "offered", versions, "supported", s.serverInfo.Versions,
		"recognized", recognized, "next_command", s.nextCmd)

	resp := append([]byte(nil), s.device.PairingID...)
	resp = append(resp, padTo([]byte{selected}, 0x10)...)
	return resp, nil
}

func (s *HandshakeService) getSecretKey(data []byte) ([]byte, error) {
	if len(data) != 0x20 {
		return nil, newError(ErrCodeMissizedPayload)
	}
	if s.nextCmd != 3 {
		return nil, newError(ErrCodeHandshakeSequence)
	}
	s.nextCmd = 4

	// Worth a log line at Info: this hands the derived secret key to whoever
	// asked, gated only on pairing mode and sequence state.
	s.log().Info("releasing secret key to device in pairing mode",
		"name", s.device.Name, "address", s.device.Address,
		logging.Secret("secret_key", s.device.SecretKey))

	return append([]byte(nil), s.device.SecretKey...), nil
}

func (s *HandshakeService) finalize(data []byte) ([]byte, error) {
	if len(data) != 0x20 {
		return nil, newError(ErrCodeMissizedPayload)
	}
	if s.nextCmd != 4 {
		return nil, newError(ErrCodeHandshakeSequence)
	}
	expected := s.transcript.FlushedDigest()
	if !bytesEqual(expected, data) {
		// Both values are transcript digests over data the peer already has, so
		// logging them is safe and is the only way to debug a mismatch.
		s.log().Warn("handshake transcript mismatch",
			"name", s.device.Name, "address", s.device.Address,
			logging.Dump("expected", expected), logging.Dump("received", data))
		return nil, newError(ErrCodeHandshakeHash)
	}

	s.nextCmd = 0
	s.done = true
	s.log().Info("handshake complete",
		"name", s.device.Name, "address", s.device.Address, "version", s.device.Version)
	if s.handler != nil && s.device != nil {
		go s.handler.DeviceConnected(s.device)
	}
	return s.transcript.Finalize(data), nil
}

type transcript struct {
	hash hashState
	buf  []byte
}

type hashState struct {
	h hash.Hash
}

func newTranscript() transcript {
	return transcript{hash: hashState{h: sha256.New()}}
}

func (t *transcript) Append(request []byte, response []byte) {
	t.buf = append(t.buf, request...)
	t.buf = append(t.buf, response...)
	if len(t.buf) >= 64 {
		n := len(t.buf) &^ 63
		_, _ = t.hash.h.Write(t.buf[:n])
		t.buf = append([]byte(nil), t.buf[n:]...)
	}
}

func (t *transcript) FlushedDigest() []byte {
	return t.hash.h.Sum(nil)
}

func (t *transcript) Finalize(clientDigest []byte) []byte {
	if len(t.buf) > 0 {
		_, _ = t.hash.h.Write(t.buf)
	}
	_, _ = t.hash.h.Write(clientDigest)
	t.buf = nil
	return t.hash.h.Sum(nil)
}

func padTo(data []byte, size int) []byte {
	if len(data) >= size {
		return append([]byte(nil), data...)
	}
	out := make([]byte, size)
	copy(out, data)
	return out
}

func hostOf(conn *Conn) string {
	if conn == nil {
		return ""
	}
	return conn.Host()
}

func bytesIndexByte(data []byte, target byte) int {
	for i, b := range data {
		if b == target {
			return i
		}
	}
	return -1
}

func bytesEqual(a []byte, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
