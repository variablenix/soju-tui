package main

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// IRCMessage is the small subset of IRCv3's wire representation needed by the
// client. Keeping this parser here avoids coupling the runtime binary to an IRC
// framework while still handling message tags, trailing parameters, and CAP.
type IRCMessage struct {
	Tags    map[string]string
	Prefix  string
	Command string
	Params  []string
}

func parseIRCLine(line string) (IRCMessage, error) {
	line = strings.TrimSuffix(line, "\r\n")
	line = strings.TrimSuffix(line, "\n")
	if line == "" {
		return IRCMessage{}, errors.New("empty IRC line")
	}

	var msg IRCMessage
	if line[0] == '@' {
		space := strings.IndexByte(line, ' ')
		if space < 0 {
			return IRCMessage{}, errors.New("IRC tags without a message")
		}
		msg.Tags = parseTags(line[1:space])
		line = strings.TrimLeft(line[space+1:], " ")
	}
	if strings.HasPrefix(line, ":") {
		space := strings.IndexByte(line, ' ')
		if space < 0 {
			return IRCMessage{}, errors.New("IRC prefix without a command")
		}
		msg.Prefix = line[1:space]
		line = strings.TrimLeft(line[space+1:], " ")
	}
	if line == "" {
		return IRCMessage{}, errors.New("IRC message without a command")
	}

	parts := strings.SplitN(line, " ", 2)
	msg.Command = strings.ToUpper(parts[0])
	if len(parts) == 1 {
		return msg, nil
	}
	params := parts[1]
	for params != "" {
		params = strings.TrimLeft(params, " ")
		if params == "" {
			break
		}
		if params[0] == ':' {
			msg.Params = append(msg.Params, params[1:])
			break
		}
		space := strings.IndexByte(params, ' ')
		if space < 0 {
			msg.Params = append(msg.Params, params)
			break
		}
		msg.Params = append(msg.Params, params[:space])
		params = params[space+1:]
	}
	return msg, nil
}

func parseTags(raw string) map[string]string {
	tags := make(map[string]string)
	for _, token := range strings.Split(raw, ";") {
		if token == "" {
			continue
		}
		name, value, hasValue := strings.Cut(token, "=")
		if hasValue {
			tags[name] = unescapeTag(value)
		} else {
			tags[name] = ""
		}
	}
	return tags
}

func unescapeTag(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 >= len(value) {
			b.WriteByte(value[i])
			continue
		}
		i++
		switch value[i] {
		case ':':
			b.WriteByte(';')
		case 's':
			b.WriteByte(' ')
		case 'r':
			b.WriteByte('\r')
		case 'n':
			b.WriteByte('\n')
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte(value[i])
		}
	}
	return b.String()
}

func escapeTag(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, ";", `\:`)
	value = strings.ReplaceAll(value, " ", `\s`)
	value = strings.ReplaceAll(value, "\r", `\r`)
	return strings.ReplaceAll(value, "\n", `\n`)
}

func (m IRCMessage) String() string {
	var b strings.Builder
	if len(m.Tags) != 0 {
		keys := make([]string, 0, len(m.Tags))
		for key := range m.Tags {
			keys = append(keys, key)
		}
		// Stable tag ordering makes raw logging and tests predictable.
		for i := 0; i < len(keys); i++ {
			for j := i + 1; j < len(keys); j++ {
				if keys[j] < keys[i] {
					keys[i], keys[j] = keys[j], keys[i]
				}
			}
		}
		b.WriteByte('@')
		for i, key := range keys {
			if i > 0 {
				b.WriteByte(';')
			}
			b.WriteString(key)
			if m.Tags[key] != "" {
				b.WriteByte('=')
				b.WriteString(escapeTag(m.Tags[key]))
			}
		}
		b.WriteByte(' ')
	}
	if m.Prefix != "" {
		b.WriteByte(':')
		b.WriteString(m.Prefix)
		b.WriteByte(' ')
	}
	b.WriteString(m.Command)
	for i, param := range m.Params {
		b.WriteByte(' ')
		if i == len(m.Params)-1 && (strings.ContainsAny(param, " :") || param == "") {
			b.WriteByte(':')
		}
		b.WriteString(param)
	}
	return b.String()
}

func command(command string, params ...string) IRCMessage {
	return IRCMessage{Command: strings.ToUpper(command), Params: params}
}

type SessionConfig struct {
	Address         string
	TLS             bool
	TLSServerName   string
	InsecureSkipTLS bool
	Username        string
	Password        string
	Nick            string
	Realname        string
	ClientName      string
	BindNetworkID   string
	Label           string
}

type SessionEvent struct {
	Session *IRCSession
	Message *IRCMessage
	Err     error
}

type IRCSession struct {
	cfg       SessionConfig
	conn      net.Conn
	writeMu   sync.Mutex
	readLines chan string
	closed    chan struct{}
	closeOnce sync.Once

	Capabilities map[string]string
	EnabledCaps  map[string]bool
}

func connectSession(cfg SessionConfig) (*IRCSession, error) {
	dialer := net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var err error
	if cfg.TLS {
		serverName := cfg.TLSServerName
		if serverName == "" {
			serverName = hostFromAddress(cfg.Address)
		}
		conn, err = tls.DialWithDialer(&dialer, "tcp", cfg.Address, &tls.Config{
			ServerName:         serverName,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: cfg.InsecureSkipTLS, // explicitly controlled by CLI
		})
	} else {
		conn, err = dialer.Dial("tcp", cfg.Address)
	}
	if err != nil {
		return nil, err
	}

	s := &IRCSession{
		cfg:          cfg,
		conn:         conn,
		readLines:    make(chan string, 8),
		closed:       make(chan struct{}),
		Capabilities: make(map[string]string),
		EnabledCaps:  make(map[string]bool),
	}
	if err := s.register(); err != nil {
		conn.Close()
		return nil, err
	}
	return s, nil
}

func hostFromAddress(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}
	return strings.Trim(address, "[]")
}

func (s *IRCSession) register() error {
	reader := bufio.NewReaderSize(s.conn, 128*1024)
	if err := s.send(command("CAP", "LS", "302")); err != nil {
		return err
	}
	if err := s.send(command("NICK", s.cfg.Nick)); err != nil {
		return err
	}
	username := s.cfg.Username
	if s.cfg.ClientName != "" && !strings.Contains(username, "@") {
		username += "@" + s.cfg.ClientName
	}
	if err := s.send(command("USER", username, "0", "*", s.cfg.Realname)); err != nil {
		return err
	}

	saslStarted := false
	registered := false
	capEnded := false
	for !registered {
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("registration read: %w", err)
		}
		msg, err := parseIRCLine(line)
		if err != nil {
			continue
		}
		switch msg.Command {
		case "PING":
			if len(msg.Params) > 0 {
				_ = s.send(command("PONG", msg.Params[len(msg.Params)-1]))
			}
		case "CAP":
			if len(msg.Params) < 2 {
				continue
			}
			subcommand := strings.ToUpper(msg.Params[1])
			caps := strings.Fields(msg.Params[len(msg.Params)-1])
			switch subcommand {
			case "LS":
				for _, cap := range caps {
					name, value, _ := strings.Cut(cap, "=")
					s.Capabilities[name] = value
				}
				if !supportsPlainSASL(s.Capabilities["sasl"]) {
					return errors.New("soju server does not advertise SASL PLAIN")
				}
				requested := s.requestedCapabilities()
				if len(requested) == 0 {
					return errors.New("soju server advertised no usable IRCv3 capabilities")
				}
				if err := s.send(command("CAP", "REQ", strings.Join(requested, " "))); err != nil {
					return err
				}
			case "ACK":
				for _, cap := range caps {
					name := strings.TrimPrefix(cap, "-")
					if strings.HasPrefix(cap, "-") {
						delete(s.EnabledCaps, name)
					} else {
						s.EnabledCaps[name] = true
					}
				}
				if s.EnabledCaps["sasl"] && !saslStarted {
					saslStarted = true
					if err := s.send(command("AUTHENTICATE", "PLAIN")); err != nil {
						return err
					}
				}
			case "NAK":
				return errors.New("soju server rejected the requested IRCv3 capabilities")
			}
		case "AUTHENTICATE":
			if len(msg.Params) == 0 {
				continue
			}
			if msg.Params[0] == "+" {
				payload := "\x00" + username + "\x00" + s.cfg.Password
				encoded := base64.StdEncoding.EncodeToString([]byte(payload))
				if err := s.send(command("AUTHENTICATE", encoded)); err != nil {
					return err
				}
			}
		case "903":
			if !capEnded {
				if s.cfg.BindNetworkID != "" {
					if err := s.send(command("BOUNCER", "BIND", s.cfg.BindNetworkID)); err != nil {
						return err
					}
				}
				if err := s.send(command("CAP", "END")); err != nil {
					return err
				}
				capEnded = true
			}
		case "904", "905", "906", "907":
			return fmt.Errorf("SASL authentication failed (%s): %s", msg.Command, strings.Join(msg.Params, " "))
		case "001":
			registered = true
		default:
			if msg.Command == "FAIL" && len(msg.Params) > 1 {
				return fmt.Errorf("soju registration failed: %s", strings.Join(msg.Params, " "))
			}
		}
	}
	// A few IRC servers omit 903 and simply allow CAP END after AUTHENTICATE.
	if !capEnded {
		if s.cfg.BindNetworkID != "" {
			if err := s.send(command("BOUNCER", "BIND", s.cfg.BindNetworkID)); err != nil {
				return err
			}
		}
		if err := s.send(command("CAP", "END")); err != nil {
			return err
		}
	}
	return nil
}

func supportsPlainSASL(value string) bool {
	for _, mechanism := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(mechanism), "plain") {
			return true
		}
	}
	return false
}

func (s *IRCSession) requestedCapabilities() []string {
	// soju makes these capabilities available downstream; unsupported items are
	// safely omitted by intersecting with the server's CAP LS response.
	wanted := []string{
		"sasl",
		"batch",
		"cap-notify",
		"echo-message",
		"message-tags",
		"server-time",
		"multi-prefix",
		"extended-join",
		"away-notify",
		"account-notify",
		"draft/chathistory",
		"draft/read-marker",
		"soju.im/bouncer-networks",
		"soju.im/bouncer-networks-notify",
	}
	available := make(map[string]bool, len(s.Capabilities))
	for name := range s.Capabilities {
		available[name] = true
	}
	requested := make([]string, 0, len(wanted))
	for _, name := range wanted {
		if available[name] {
			requested = append(requested, name)
		}
	}
	return requested
}

func (s *IRCSession) send(msg IRCMessage) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := io.WriteString(s.conn, msg.String()+"\r\n"); err != nil {
		return err
	}
	return nil
}

func (s *IRCSession) Start(events chan<- SessionEvent) {
	go func() {
		reader := bufio.NewReaderSize(s.conn, 128*1024)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) {
					events <- SessionEvent{Session: s, Err: err}
				} else {
					events <- SessionEvent{Session: s, Err: io.EOF}
				}
				s.close()
				return
			}
			msg, err := parseIRCLine(line)
			if err != nil {
				continue
			}
			if msg.Command == "PING" && len(msg.Params) > 0 {
				_ = s.send(command("PONG", msg.Params[len(msg.Params)-1]))
				continue
			}
			events <- SessionEvent{Session: s, Message: &msg}
		}
	}()
}

func (s *IRCSession) Send(commandName string, params ...string) error {
	return s.send(command(commandName, params...))
}

func (s *IRCSession) SendMessage(msg IRCMessage) error {
	return s.send(msg)
}

func (s *IRCSession) Close() {
	s.close()
}

func (s *IRCSession) close() {
	s.closeOnce.Do(func() {
		close(s.closed)
		_ = s.conn.Close()
	})
}

func parseNetworkAttrs(raw string) map[string]string {
	attrs := make(map[string]string)
	for _, pair := range strings.Split(raw, ";") {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			continue
		}
		attrs[key] = unescapeTag(value)
	}
	return attrs
}

func parsePort(raw string, tlsEnabled bool) string {
	if raw != "" {
		if _, err := strconv.Atoi(raw); err == nil {
			return raw
		}
	}
	if tlsEnabled {
		return "6697"
	}
	return "6667"
}
