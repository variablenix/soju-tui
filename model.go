package main

import (
	"sort"
	"strings"
	"sync"
	"time"
)

type NetworkInfo struct {
	ID       string
	Name     string
	Host     string
	Port     string
	TLS      bool
	Nick     string
	Username string
	Realname string
	State    string
	Error    string
	Attrs    map[string]string
	Session  *IRCSession
}

func (n *NetworkInfo) displayName() string {
	if n.Name != "" {
		return n.Name
	}
	if n.Host != "" {
		return n.Host
	}
	return "network " + n.ID
}

type ChatLine struct {
	When  time.Time
	Kind  string
	From  string
	Text  string
	Style string
}

type Buffer struct {
	Key       string
	NetworkID string
	Target    string
	Title     string
	Kind      string
	Lines     []ChatLine
	Members   []string
	Unread    int
	AtBottom  bool
	// OldestMsgID is used as the cursor for draft/chathistory BEFORE requests.
	OldestMsgID string
}

type App struct {
	mu       sync.RWMutex
	cfg      AppConfig
	events   chan SessionEvent
	root     *IRCSession
	sessions map[*IRCSession]string
	networks map[string]*NetworkInfo
	order    []string
	buffers  map[string]*Buffer
	active   string

	input       []rune
	history     []string
	historyPos  int
	scroll      int
	status      string
	statusUntil time.Time
	quit        bool
	started     bool
	admin       AdminState
}

type AppConfig struct {
	Server          string
	TLS             bool
	TLSServerName   string
	InsecureSkipTLS bool
	Username        string
	Password        string
	Nick            string
	Realname        string
	ClientName      string
	NetworkFilter   string
}

func newApp(cfg AppConfig) *App {
	a := &App{
		cfg:      cfg,
		events:   make(chan SessionEvent, 128),
		sessions: make(map[*IRCSession]string),
		networks: make(map[string]*NetworkInfo),
		buffers:  make(map[string]*Buffer),
		status:   "connecting to soju...",
		admin:    newAdminState(),
	}
	a.ensureBuffer("root", "", "BouncerServ", "server")
	a.active = "root"
	return a
}

func (a *App) start() error {
	rootUsername := a.cfg.Username
	rootNick := a.cfg.Nick
	if rootNick == "" {
		rootNick = rootUsername
	}
	root, err := connectSession(SessionConfig{
		Address:         a.cfg.Server,
		TLS:             a.cfg.TLS,
		TLSServerName:   a.cfg.TLSServerName,
		InsecureSkipTLS: a.cfg.InsecureSkipTLS,
		Username:        rootUsername,
		Password:        a.cfg.Password,
		Nick:            rootNick,
		Realname:        a.cfg.Realname,
		ClientName:      a.cfg.ClientName + "-root",
		Label:           "root",
	})
	if err != nil {
		return err
	}
	a.root = root
	a.sessions[root] = "root"
	root.Start(a.events)
	a.started = true
	if strings.Contains(rootUsername, "/") {
		a.setStatus("connected (single-network login)", 0)
	} else {
		a.setStatus("connected; discovering soju networks...", 0)
		_ = root.Send("BOUNCER", "LISTNETWORKS")
	}
	return nil
}

func (a *App) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.quit = true
	for session := range a.sessions {
		session.Close()
	}
}

func (a *App) processEvent(event SessionEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if event.Err != nil {
		label := a.sessions[event.Session]
		if label == "" {
			label = "root"
		}
		text := event.Err.Error()
		if event.Err.Error() == "EOF" {
			text = "connection closed"
		}
		a.addLineLocked(a.bufferForSessionLocked(label, "BouncerServ", "server"), ChatLine{When: time.Now(), Kind: "error", Text: text})
		if network := a.networks[label]; network != nil {
			network.State = "disconnected"
			network.Error = text
		}
		if label == "root" {
			a.setStatusLocked("bouncer connection closed: "+text, 0)
		}
		return
	}
	if event.Message == nil {
		return
	}
	label := a.sessions[event.Session]
	if label == "" {
		label = "root"
	}
	a.handleMessageLocked(label, event.Message)
}

func (a *App) handleMessageLocked(label string, msg *IRCMessage) {
	if label == "root" {
		a.handleRootMessageLocked(msg)
	}

	switch msg.Command {
	case "PRIVMSG", "NOTICE":
		if len(msg.Params) < 2 {
			return
		}
		target, text := msg.Params[0], msg.Params[1]
		from := prefixNick(msg.Prefix)
		if from == "" {
			from = "server"
		}
		if target == "" {
			return
		}
		// A message addressed to this client is a query. The bouncer service is
		// always kept in the root buffer, which makes network management usable
		// even before an upstream network has been created.
		if target == localNick(a.cfg.Nick, a.cfg.Username, label) {
			if label == "root" && (from == "BouncerServ" || strings.EqualFold(from, "BouncerServ")) {
				target = "BouncerServ"
			} else {
				target = from
			}
		}
		kind := "message"
		if msg.Command == "NOTICE" {
			kind = "notice"
		}
		buffer := a.bufferForSessionLocked(label, target, kind)
		if buffer.OldestMsgID == "" && msg.Tags != nil {
			buffer.OldestMsgID = msg.Tags["msgid"]
		}
		a.addLineLocked(buffer, ChatLine{When: messageTime(msg), Kind: kind, From: from, Text: cleanIRCText(text)})
		if label == "root" && strings.EqualFold(from, "BouncerServ") {
			a.admin.output = append(a.admin.output, cleanIRCText(text))
			if len(a.admin.output) > 500 {
				a.admin.output = a.admin.output[len(a.admin.output)-500:]
			}
		}
	case "JOIN":
		if len(msg.Params) == 0 {
			return
		}
		target := msg.Params[0]
		from := prefixNick(msg.Prefix)
		buffer := a.bufferForSessionLocked(label, target, "channel")
		if from != "" && !containsFold(buffer.Members, from) {
			buffer.Members = append(buffer.Members, from)
			sort.Strings(buffer.Members)
		}
		if from != "" {
			a.addLineLocked(buffer, ChatLine{When: time.Now(), Kind: "event", From: from, Text: "joined " + target})
		}
	case "PART":
		if len(msg.Params) == 0 {
			return
		}
		target := msg.Params[0]
		from := prefixNick(msg.Prefix)
		text := "left " + target
		if len(msg.Params) > 1 && msg.Params[1] != "" {
			text += " (" + msg.Params[1] + ")"
		}
		buffer := a.bufferForSessionLocked(label, target, "channel")
		buffer.Members = removeFold(buffer.Members, from)
		a.addLineLocked(buffer, ChatLine{When: time.Now(), Kind: "event", From: from, Text: text})
	case "QUIT":
		from := prefixNick(msg.Prefix)
		text := "quit"
		if len(msg.Params) > 0 {
			text += " (" + msg.Params[0] + ")"
		}
		a.addLineLocked(a.bufferForSessionLocked(label, "BouncerServ", "server"), ChatLine{When: time.Now(), Kind: "event", From: from, Text: text})
	case "NICK":
		if len(msg.Params) > 0 {
			from := prefixNick(msg.Prefix)
			a.addLineLocked(a.bufferForSessionLocked(label, "BouncerServ", "server"), ChatLine{When: time.Now(), Kind: "event", From: from, Text: "is now known as " + msg.Params[0]})
		}
	case "KICK":
		if len(msg.Params) >= 2 {
			target := msg.Params[0]
			text := "kicked " + msg.Params[1]
			if len(msg.Params) > 2 {
				text += " (" + msg.Params[2] + ")"
			}
			a.addLineLocked(a.bufferForSessionLocked(label, target, "channel"), ChatLine{When: time.Now(), Kind: "event", From: prefixNick(msg.Prefix), Text: text})
		}
	case "332":
		if len(msg.Params) >= 3 {
			buffer := a.bufferForSessionLocked(label, msg.Params[1], "channel")
			a.addLineLocked(buffer, ChatLine{When: time.Now(), Kind: "topic", Text: "topic: " + msg.Params[2]})
		}
	case "353":
		if len(msg.Params) >= 4 {
			buffer := a.bufferForSessionLocked(label, msg.Params[2], "channel")
			for _, member := range strings.Fields(msg.Params[3]) {
				member = strings.TrimLeft(member, "~&@%+")
				if member != "" && !containsFold(buffer.Members, member) {
					buffer.Members = append(buffer.Members, member)
				}
			}
			sort.Strings(buffer.Members)
		}
	case "001", "002", "003", "004", "005", "375", "372", "376", "422":
		if label == "root" {
			a.addNumericLocked("root", msg)
		}
	case "ERROR", "FAIL":
		a.addLineLocked(a.bufferForSessionLocked(label, "BouncerServ", "server"), ChatLine{When: time.Now(), Kind: "error", Text: strings.Join(msg.Params, " ")})
	}
}

func (a *App) handleRootMessageLocked(msg *IRCMessage) {
	if msg.Command != "BOUNCER" || len(msg.Params) < 3 || !strings.EqualFold(msg.Params[0], "NETWORK") {
		if msg.Command == "BATCH" && len(msg.Params) > 0 {
			return
		}
		return
	}
	id := msg.Params[1]
	attrs := parseNetworkAttrs(msg.Params[2])
	network := a.networks[id]
	if network == nil {
		network = &NetworkInfo{ID: id, Attrs: make(map[string]string)}
		a.networks[id] = network
	}
	for key, value := range attrs {
		network.Attrs[key] = value
	}
	if value, ok := attrs["name"]; ok {
		network.Name = value
	}
	if value, ok := attrs["host"]; ok {
		network.Host = value
	}
	if value, ok := attrs["port"]; ok {
		network.Port = parsePort(value, network.TLS)
	}
	if value, ok := attrs["tls"]; ok {
		network.TLS = value != "0"
		if _, hasPort := attrs["port"]; !hasPort {
			network.Port = parsePort("", network.TLS)
		}
	}
	if value, ok := attrs["nickname"]; ok {
		network.Nick = value
	}
	if value, ok := attrs["username"]; ok {
		network.Username = value
	}
	if value, ok := attrs["realname"]; ok {
		network.Realname = value
	}
	if value, ok := attrs["state"]; ok {
		network.State = value
	}
	if value, ok := attrs["error"]; ok {
		network.Error = value
	}
	if network.State == "" {
		network.State = "disconnected"
	}
	if network.Session == nil && a.cfg.NetworkFilter == "" || network.Session == nil && strings.EqualFold(a.cfg.NetworkFilter, network.displayName()) {
		a.startNetworkLocked(network)
	}
}

func (a *App) startNetworkLocked(network *NetworkInfo) {
	if network.Host == "" || network.ID == "" {
		return
	}
	nick := network.Nick
	if nick == "" {
		nick = a.cfg.Nick
	}
	realname := network.Realname
	if realname == "" {
		realname = a.cfg.Realname
	}
	clientName := a.cfg.ClientName + "-" + network.ID
	// Network bindings are made during registration, as required by
	// soju.im/bouncer-networks. They run in a goroutine so a slow upstream does
	// not stall the root connection or the TUI.
	go func() {
		session, err := connectSession(SessionConfig{
			Address:         a.cfg.Server,
			TLS:             a.cfg.TLS,
			TLSServerName:   a.cfg.TLSServerName,
			InsecureSkipTLS: a.cfg.InsecureSkipTLS,
			Username:        a.cfg.Username,
			Password:        a.cfg.Password,
			Nick:            nick,
			Realname:        realname,
			ClientName:      clientName,
			BindNetworkID:   network.ID,
			Label:           network.ID,
		})
		a.mu.Lock()
		defer a.mu.Unlock()
		if err != nil {
			network.State = "error"
			network.Error = err.Error()
			a.addLineLocked(a.bufferForSessionLocked(network.ID, "BouncerServ", "server"), ChatLine{When: time.Now(), Kind: "error", Text: "network connection: " + err.Error()})
			return
		}
		network.Session = session
		network.State = "connected"
		a.sessions[session] = network.ID
		a.ensureBuffer(network.ID, "", network.displayName(), "server")
		session.Start(a.events)
		a.setStatusLocked("connected to "+network.displayName(), 3*time.Second)
	}()
}

func (a *App) addNumericLocked(label string, msg *IRCMessage) {
	text := strings.Join(msg.Params, " ")
	if len(msg.Params) > 1 {
		text = strings.Join(msg.Params[1:], " ")
	}
	a.addLineLocked(a.bufferForSessionLocked(label, "BouncerServ", "server"), ChatLine{When: time.Now(), Kind: "server", Text: text})
}

func (a *App) sendInput() {
	a.mu.Lock()
	defer a.mu.Unlock()
	text := strings.TrimSpace(string(a.input))
	a.input = nil
	a.historyPos = len(a.history)
	if text == "" {
		return
	}
	a.history = append(a.history, text)
	a.executeInputLocked(text)
}

func (a *App) executeInputLocked(text string) {
	if !strings.HasPrefix(text, "/") {
		a.sendToActiveLocked("PRIVMSG", text)
		return
	}
	commandText := strings.TrimPrefix(text, "/")
	name, rest, _ := strings.Cut(commandText, " ")
	name = strings.ToLower(name)
	rest = strings.TrimSpace(rest)
	words := strings.Fields(rest)
	switch name {
	case "quit", "q", "exit":
		reason := rest
		for session := range a.sessions {
			if reason == "" {
				_ = session.Send("QUIT")
			} else {
				_ = session.Send("QUIT", reason)
			}
		}
		a.quit = true
	case "join":
		if len(words) == 0 {
			a.setStatusLocked("usage: /join #channel [key]", 4*time.Second)
			return
		}
		params := append([]string{words[0]}, words[1:]...)
		a.sendToActiveLocked("JOIN", params...)
	case "part":
		target := a.activeTargetLocked()
		if len(words) > 0 {
			target = words[0]
		}
		if target == "" || target == "BouncerServ" {
			a.setStatusLocked("select a channel before using /part", 4*time.Second)
			return
		}
		params := []string{target}
		if len(words) > 1 {
			params = append(params, strings.TrimSpace(strings.TrimPrefix(rest, words[0])))
		}
		a.sendToActiveLocked("PART", params...)
	case "msg", "privmsg", "query":
		if len(words) == 0 {
			a.setStatusLocked("usage: /msg target text", 4*time.Second)
			return
		}
		target := words[0]
		message := strings.TrimSpace(strings.TrimPrefix(rest, target))
		if name == "query" {
			if message == "" {
				a.selectBufferLocked(a.bufferForSessionLocked(a.activeLabelLocked(), target, "query").Key)
				return
			}
		}
		a.sendToSessionTargetLocked(a.activeSessionLocked(), target, "PRIVMSG", message)
	case "notice":
		if len(words) < 2 {
			a.setStatusLocked("usage: /notice target text", 4*time.Second)
			return
		}
		target := words[0]
		message := strings.TrimSpace(strings.TrimPrefix(rest, target))
		a.sendToSessionTargetLocked(a.activeSessionLocked(), target, "NOTICE", message)
	case "me", "action":
		if rest == "" {
			return
		}
		target := a.activeTargetLocked()
		if target == "" || target == "BouncerServ" {
			a.setStatusLocked("select a channel or query before using /me", 4*time.Second)
			return
		}
		a.sendToSessionTargetLocked(a.activeSessionLocked(), target, "PRIVMSG", "\x01ACTION "+rest+"\x01")
	case "nick":
		if len(words) == 1 {
			a.sendToActiveLocked("NICK", words[0])
		}
	case "topic":
		target := a.activeTargetLocked()
		if target == "" || target == "BouncerServ" {
			return
		}
		if rest == "" {
			a.sendToActiveLocked("TOPIC", target)
		} else {
			a.sendToActiveLocked("TOPIC", target, rest)
		}
	case "names":
		target := a.activeTargetLocked()
		if len(words) > 0 {
			target = words[0]
		}
		if target != "" {
			a.sendToActiveLocked("NAMES", target)
		}
	case "away":
		if rest == "" {
			a.sendToActiveLocked("AWAY")
		} else {
			a.sendToActiveLocked("AWAY", rest)
		}
	case "back":
		a.sendToActiveLocked("AWAY")
	case "network", "net":
		a.executeNetworkCommandLocked(words)
	case "raw":
		if rest != "" {
			a.sendRawToActiveLocked(rest)
		}
	case "clear":
		if buffer := a.buffers[a.active]; buffer != nil {
			buffer.Lines = nil
		}
	case "help":
		a.showHelpLocked()
	default:
		a.setStatusLocked("unknown command: /"+name+" (try /help)", 4*time.Second)
	}
}

func (a *App) executeNetworkCommandLocked(words []string) {
	if len(words) == 0 || strings.EqualFold(words[0], "list") {
		if a.root != nil {
			_ = a.root.Send("BOUNCER", "LISTNETWORKS")
			a.setStatusLocked("requested network list", 3*time.Second)
		}
		return
	}
	query := strings.Join(words, " ")
	for id, network := range a.networks {
		if strings.EqualFold(id, query) || strings.EqualFold(network.Name, query) || strings.EqualFold(network.Host, query) {
			key := a.ensureBuffer(id, "", network.displayName(), "server").Key
			a.selectBufferLocked(key)
			a.setStatusLocked("selected network "+network.displayName(), 3*time.Second)
			return
		}
	}
	a.setStatusLocked("network not found: "+query, 4*time.Second)
}

func (a *App) showHelpLocked() {
	buffer := a.buffers[a.active]
	if buffer == nil {
		buffer = a.buffers["root"]
	}
	for _, line := range []string{
		"commands: /join /part /msg /query /notice /me /nick /topic /names",
		"          /away /back /network /raw /clear /quit",
		"keys: Tab next buffer, Ctrl-P/Ctrl-N previous/next, PgUp/PgDn scroll, F2 admin, Ctrl-C quit",
	} {
		a.addLineLocked(buffer, ChatLine{When: time.Now(), Kind: "help", Text: line})
	}
}

func (a *App) sendToActiveLocked(commandName string, params ...string) {
	if commandName == "PRIVMSG" || commandName == "NOTICE" {
		a.sendToSessionTargetLocked(a.activeSessionLocked(), a.activeTargetLocked(), commandName, params...)
		return
	}
	if session := a.activeSessionLocked(); session != nil {
		if err := session.Send(commandName, params...); err != nil {
			a.setStatusLocked("send failed: "+err.Error(), 4*time.Second)
		}
	} else {
		a.setStatusLocked("no connection for this buffer", 4*time.Second)
	}
}

func (a *App) sendToSessionTargetLocked(session *IRCSession, target, commandName string, params ...string) {
	if session == nil {
		a.setStatusLocked("no connection for this buffer", 4*time.Second)
		return
	}
	if target != "" {
		params = append([]string{target}, params...)
	}
	if err := session.Send(commandName, params...); err != nil {
		a.setStatusLocked("send failed: "+err.Error(), 4*time.Second)
	}
}

func (a *App) sendRawToActiveLocked(raw string) {
	msg, err := parseIRCLine(raw)
	if err != nil {
		a.setStatusLocked("invalid raw IRC line: "+err.Error(), 4*time.Second)
		return
	}
	if session := a.activeSessionLocked(); session != nil {
		if err := session.SendMessage(msg); err != nil {
			a.setStatusLocked("send failed: "+err.Error(), 4*time.Second)
		}
	} else {
		a.setStatusLocked("no connection for this buffer", 4*time.Second)
	}
}

func (a *App) activeSessionLocked() *IRCSession {
	return a.sessionForLabelLocked(a.activeLabelLocked())
}

func (a *App) activeLabelLocked() string {
	if buffer := a.buffers[a.active]; buffer != nil {
		return buffer.NetworkID
	}
	return "root"
}

func (a *App) activeTargetLocked() string {
	if buffer := a.buffers[a.active]; buffer != nil {
		return buffer.Target
	}
	return ""
}

func (a *App) sessionForLabelLocked(label string) *IRCSession {
	if label == "root" {
		return a.root
	}
	if network := a.networks[label]; network != nil {
		return network.Session
	}
	return nil
}

func (a *App) bufferForSessionLocked(label, target, kind string) *Buffer {
	title := target
	if title == "" {
		if network := a.networks[label]; network != nil {
			title = network.displayName()
		} else {
			title = "BouncerServ"
		}
	}
	return a.ensureBuffer(label, target, title, kind)
}

func (a *App) ensureBuffer(label, target, title, kind string) *Buffer {
	key := label + ":" + target
	if label == "root" && target == "" {
		key = "root"
	}
	if buffer := a.buffers[key]; buffer != nil {
		return buffer
	}
	buffer := &Buffer{Key: key, NetworkID: label, Target: target, Title: title, Kind: kind, AtBottom: true}
	a.buffers[key] = buffer
	a.order = append(a.order, key)
	return buffer
}

func (a *App) selectBufferLocked(key string) {
	if a.buffers[key] == nil {
		return
	}
	a.active = key
	a.scroll = 0
	a.buffers[key].Unread = 0
}

func (a *App) nextBuffer(delta int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.order) == 0 {
		return
	}
	idx := 0
	for i, key := range a.order {
		if key == a.active {
			idx = i
			break
		}
	}
	idx = (idx + delta) % len(a.order)
	if idx < 0 {
		idx += len(a.order)
	}
	a.selectBufferLocked(a.order[idx])
}

func (a *App) addLineLocked(buffer *Buffer, line ChatLine) {
	if buffer == nil {
		return
	}
	buffer.Lines = append(buffer.Lines, line)
	if len(buffer.Lines) > 2000 {
		buffer.Lines = buffer.Lines[len(buffer.Lines)-2000:]
	}
	if buffer.Key != a.active {
		buffer.Unread++
	}
	a.scroll = 0
}

func (a *App) setStatus(text string, duration time.Duration) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setStatusLocked(text, duration)
}

func (a *App) setStatusLocked(text string, duration time.Duration) {
	a.status = text
	if duration > 0 {
		a.statusUntil = time.Now().Add(duration)
	} else {
		a.statusUntil = time.Time{}
	}
}

func (a *App) currentStatusLocked() string {
	if !a.statusUntil.IsZero() && time.Now().After(a.statusUntil) {
		return "connected"
	}
	return a.status
}

func prefixNick(prefix string) string {
	if prefix == "" {
		return ""
	}
	if idx := strings.IndexAny(prefix, "!@"); idx >= 0 {
		return prefix[:idx]
	}
	return prefix
}

func localNick(configured, username, label string) string {
	if configured != "" {
		return configured
	}
	if slash := strings.IndexByte(username, '/'); slash > 0 {
		return username[:slash]
	}
	return username
}

func messageTime(msg *IRCMessage) time.Time {
	if msg.Tags != nil {
		if raw := msg.Tags["time"]; raw != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
				return parsed
			}
		}
	}
	return time.Now()
}

func cleanIRCText(text string) string {
	if strings.HasPrefix(text, "\x01ACTION ") && strings.HasSuffix(text, "\x01") {
		return "* " + strings.TrimSuffix(strings.TrimPrefix(text, "\x01ACTION "), "\x01")
	}
	return text
}

func containsFold(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}

func removeFold(values []string, needle string) []string {
	result := values[:0]
	for _, value := range values {
		if !strings.EqualFold(value, needle) {
			result = append(result, value)
		}
	}
	return result
}
