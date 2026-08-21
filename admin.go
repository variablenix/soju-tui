package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type AdminView string

const (
	adminDashboard AdminView = "dashboard"
	adminOutput    AdminView = "output"
	adminForm      AdminView = "form"
)

type AdminState struct {
	active   bool
	view     AdminView
	cursor   int
	output   []string
	form     *AdminForm
	confirm  *AdminConfirmation
	status   string
	lastMenu string
}

type AdminField struct {
	Label    string
	Value    string
	Secret   bool
	Required bool
	Kind     string
	Help     string
	Options  []string
}

type AdminForm struct {
	Kind   string
	Title  string
	Fields []AdminField
	Cursor int
}

type AdminOperation struct {
	Summary  string
	Command  string
	Preview  string
	Refresh  string
	Mutating bool
}

type AdminConfirmation struct {
	Operation AdminOperation
}

// This is deliberately narrower than IRC's character set: it contains only
// characters that are inert when emitted as an unquoted POSIX shell token.
// Everything else is single-quoted before it is sent to BouncerServ.
var safeAdminToken = regexp.MustCompile(`^[A-Za-z0-9_@+.,:/=-]+$`)

func newAdminState() AdminState {
	return AdminState{view: adminDashboard}
}

func (a *App) toggleAdmin() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if strings.Contains(a.cfg.Username, "/") {
		a.setStatusLocked("admin view requires a global (unbound) admin login", 5e9)
		return
	}
	if a.root == nil {
		a.setStatusLocked("admin view is unavailable before connecting", 4e9)
		return
	}
	a.admin.active = !a.admin.active
	a.admin.form = nil
	a.admin.confirm = nil
	a.admin.view = adminDashboard
	a.admin.cursor = 0
	if a.admin.active {
		a.setStatusLocked("admin view: read-only until you confirm an operation", 5e9)
		a.adminRequestLocked("server status")
	} else {
		a.setStatusLocked("chat view", 3e9)
	}
}

func (a *App) adminMenu() []string {
	return []string{
		"Users — view all accounts",
		"Networks — view this admin's networks",
		"Channels — view this admin's channels",
		"Server status",
		"BouncerServ help",
		"Create user",
		"Update user",
		"Delete user",
		"Create network for user",
		"Update network for user",
		"Delete network for user",
		"Create channel for user",
		"Update channel for user",
		"Delete channel for user",
		"Generate upstream certificate",
		"Show certificate fingerprints",
		"Set upstream SASL PLAIN",
		"Reset upstream SASL",
		"Quote raw command to network",
		"Broadcast server notice",
		"Toggle server debug logging",
	}
}

func (a *App) adminHandleKey(key string, r rune) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.admin.active {
		return
	}
	if a.admin.confirm != nil {
		if key == "esc" {
			a.admin.confirm = nil
			a.setStatusLocked("operation cancelled", 3e9)
			return
		}
		switch r {
		case 'y', 'Y':
			op := a.admin.confirm.Operation
			a.admin.confirm = nil
			a.adminExecuteLocked(op)
		case 'n', 'N':
			a.admin.confirm = nil
			a.setStatusLocked("operation cancelled", 3e9)
		}
		return
	}
	if a.admin.form != nil {
		a.adminFormKeyLocked(key, r)
		return
	}
	menu := a.adminMenu()
	switch key {
	case "up":
		if a.admin.cursor > 0 {
			a.admin.cursor--
		}
	case "down":
		if a.admin.cursor < len(menu)-1 {
			a.admin.cursor++
		}
	case "enter":
		a.adminActivateMenuLocked(a.admin.cursor)
	case "esc":
		a.admin.view = adminDashboard
	case "r":
		a.adminRefreshLocked()
	case "1":
		a.adminRequestLocked("user status")
	case "2":
		a.adminOpenFormLocked("network-status")
	case "3":
		a.adminOpenFormLocked("channel-status")
	case "4":
		a.adminRequestLocked("server status")
	case "5":
		a.adminRequestLocked("help")
	case "+", "c":
		a.adminOpenFormLocked("user-create")
	case "e":
		a.adminOpenFormLocked("user-update")
	case "d":
		a.adminOpenFormLocked("user-delete")
	case "N":
		a.adminOpenFormLocked("network-create")
	case "E":
		a.adminOpenFormLocked("network-update")
	case "X":
		a.adminOpenFormLocked("network-delete")
	case "C":
		a.adminOpenFormLocked("channel-create")
	case "V":
		a.adminOpenFormLocked("channel-update")
	case "D":
		a.adminOpenFormLocked("channel-delete")
	case "g":
		a.adminOpenFormLocked("cert-generate")
	case "f":
		a.adminOpenFormLocked("cert-fingerprint")
	case "p":
		a.adminOpenFormLocked("sasl-set-plain")
	case "P":
		a.adminOpenFormLocked("sasl-reset")
	case "q":
		a.adminOpenFormLocked("network-quote")
	case "m":
		a.adminOpenFormLocked("server-notice")
	case "b":
		a.adminOpenFormLocked("server-debug")
	}
}

func (a *App) adminActivateMenuLocked(cursor int) {
	switch cursor {
	case 0:
		a.adminRequestLocked("user status")
	case 1:
		a.adminOpenFormLocked("network-status")
	case 2:
		a.adminOpenFormLocked("channel-status")
	case 3:
		a.adminRequestLocked("server status")
	case 4:
		a.adminRequestLocked("help")
	case 5:
		a.adminOpenFormLocked("user-create")
	case 6:
		a.adminOpenFormLocked("user-update")
	case 7:
		a.adminOpenFormLocked("user-delete")
	case 8:
		a.adminOpenFormLocked("network-create")
	case 9:
		a.adminOpenFormLocked("network-update")
	case 10:
		a.adminOpenFormLocked("network-delete")
	case 11:
		a.adminOpenFormLocked("channel-create")
	case 12:
		a.adminOpenFormLocked("channel-update")
	case 13:
		a.adminOpenFormLocked("channel-delete")
	case 14:
		a.adminOpenFormLocked("cert-generate")
	case 15:
		a.adminOpenFormLocked("cert-fingerprint")
	case 16:
		a.adminOpenFormLocked("sasl-set-plain")
	case 17:
		a.adminOpenFormLocked("sasl-reset")
	case 18:
		a.adminOpenFormLocked("network-quote")
	case 19:
		a.adminOpenFormLocked("server-notice")
	case 20:
		a.adminOpenFormLocked("server-debug")
	}
}

func (a *App) adminRequestLocked(commandText string) {
	if a.root == nil {
		a.setStatusLocked("admin connection is not available", 4e9)
		return
	}
	a.admin.output = append(a.admin.output, "> "+commandText)
	if err := a.root.Send("PRIVMSG", "BouncerServ", commandText); err != nil {
		a.admin.output = append(a.admin.output, "send failed: "+err.Error())
		a.setStatusLocked("admin request failed: "+err.Error(), 4e9)
		return
	}
	a.admin.view = adminOutput
	a.admin.status = "request sent"
	a.admin.lastMenu = commandText
	a.setStatusLocked("admin request sent; press r to refresh", 4e9)
}

func (a *App) adminRefreshLocked() {
	if a.admin.lastMenu == "" {
		a.adminRequestLocked("server status")
	} else {
		a.adminRequestLocked(a.admin.lastMenu)
	}
}

func (a *App) adminOpenFormLocked(kind string) {
	form, err := newAdminForm(kind)
	if err != nil {
		a.setStatusLocked(err.Error(), 4e9)
		return
	}
	a.admin.form = form
	a.admin.view = adminForm
	a.admin.confirm = nil
	a.admin.cursor = 0
	a.setStatusLocked("fill the form; Enter advances, Ctrl-S previews the operation, Esc cancels", 5e9)
}

func newAdminForm(kind string) (*AdminForm, error) {
	f := func(label string, value string, required bool, secret bool, fieldKind string, help string) AdminField {
		return AdminField{Label: label, Value: value, Required: required, Secret: secret, Kind: fieldKind, Help: help}
	}
	boolField := func(label, value string, required bool) AdminField {
		return f(label, value, required, false, "bool", "space cycles true/false")
	}
	optionalBool := func(label string) AdminField {
		return f(label, "", false, false, "optional-bool", "space cycles unset/true/false")
	}
	selectField := func(label, value, help string, options ...string) AdminField {
		field := f(label, value, false, false, "select", help)
		field.Options = options
		return field
	}
	userTarget := f("User", "", true, false, "text", "soju username")
	networkTarget := f("Network", "", true, false, "text", "network name or address")
	baseNetwork := []AdminField{
		userTarget,
		networkTarget,
		f("Address", "", false, false, "text", "ircs://host[:port], irc+insecure://host, or irc+unix:///path"),
		f("Name", "", false, false, "text", "short display name"),
		f("Nickname", "", false, false, "text", "upstream nickname"),
		f("Username", "", false, false, "text", "upstream username"),
		f("Password", "", false, true, "text", "upstream server password"),
		f("Realname", "", false, false, "text", "upstream real name"),
		f("CertFP", "", false, false, "text", "SHA-512 certificate fingerprint"),
		optionalBool("Auto-away"),
		optionalBool("Enabled"),
		optionalBool("Ignore limit"),
		f("Connect command", "", false, false, "text", "raw IRC line sent after connecting"),
	}
	baseChannel := []AdminField{
		userTarget,
		networkTarget,
		f("Channel", "", true, false, "text", "channel name, e.g. #chat"),
		optionalBool("Detached"),
		selectField("Relay detached", "", "message, highlight, none, default", "", "message", "highlight", "none", "default"),
		selectField("Reattach on", "", "message, highlight, none, default", "", "message", "highlight", "none", "default"),
		f("Detach after", "", false, false, "text", "duration such as 30m or 0"),
		selectField("Detach on", "", "message, highlight, none, default", "", "message", "highlight", "none", "default"),
	}
	switch kind {
	case "user-create":
		return &AdminForm{Kind: kind, Title: "Create soju user", Fields: []AdminField{
			f("Username", "", true, false, "text", "immutable account name"),
			f("Password", "", true, true, "text", "bouncer login password"),
			boolField("Admin", "false", false),
			f("Nickname", "", false, false, "text", "fallback IRC nickname"),
			f("Realname", "", false, false, "text", "fallback IRC real name"),
			boolField("Enabled", "true", false),
			f("Max networks", "-1", false, false, "text", "0 none, -1 global default"),
		}}, nil
	case "user-update":
		return &AdminForm{Kind: kind, Title: "Update soju user", Fields: []AdminField{
			f("Username", "", true, false, "text", "account to update"),
			f("New password", "", false, true, "text", "leave blank to keep current"),
			boolField("Admin", "", true),
			boolField("Enabled", "", true),
			f("Max networks", "", false, false, "text", "leave blank to keep current"),
		}}, nil
	case "user-delete":
		return &AdminForm{Kind: kind, Title: "Delete soju user", Fields: []AdminField{
			f("Username", "", true, false, "text", "account to delete"),
		}}, nil
	case "network-create", "network-update":
		if kind == "network-update" {
			return &AdminForm{Kind: kind, Title: "Update network", Fields: baseNetwork}, nil
		}
		createFields := append([]AdminField{baseNetwork[0]}, baseNetwork[2:]...)
		createFields[1].Required = true
		return &AdminForm{Kind: kind, Title: "Create network", Fields: createFields}, nil
	case "network-delete":
		return &AdminForm{Kind: kind, Title: "Delete network", Fields: []AdminField{userTarget, networkTarget}}, nil
	case "network-status":
		return &AdminForm{Kind: kind, Title: "Show network status", Fields: []AdminField{userTarget}}, nil
	case "channel-create", "channel-update":
		if kind == "channel-create" {
			baseChannel[2].Required = true
		}
		return &AdminForm{Kind: kind, Title: map[string]string{"channel-create": "Create channel", "channel-update": "Update channel"}[kind], Fields: baseChannel}, nil
	case "channel-delete", "channel-status":
		fields := []AdminField{userTarget, networkTarget}
		if kind == "channel-delete" {
			fields = append(fields, f("Channel", "", true, false, "text", "channel to delete"))
		}
		return &AdminForm{Kind: kind, Title: map[string]string{"channel-delete": "Delete channel", "channel-status": "Show channel status"}[kind], Fields: fields}, nil
	case "cert-generate":
		return &AdminForm{Kind: kind, Title: "Generate upstream certificate", Fields: []AdminField{userTarget, networkTarget, selectField("Key type", "rsa", "rsa, ecdsa, ed25519", "rsa", "ecdsa", "ed25519"), f("RSA bits", "3072", false, false, "text", "ignored for ecdsa/ed25519")}}, nil
	case "cert-fingerprint":
		return &AdminForm{Kind: kind, Title: "Show certificate fingerprints", Fields: []AdminField{userTarget, networkTarget}}, nil
	case "sasl-set-plain":
		return &AdminForm{Kind: kind, Title: "Set upstream SASL PLAIN", Fields: []AdminField{userTarget, networkTarget, f("Upstream username", "", true, false, "text", "IRC account username"), f("Upstream password", "", true, true, "text", "IRC account password")}}, nil
	case "sasl-reset":
		return &AdminForm{Kind: kind, Title: "Reset upstream SASL", Fields: []AdminField{userTarget, networkTarget}}, nil
	case "network-quote":
		return &AdminForm{Kind: kind, Title: "Send raw command to network", Fields: []AdminField{userTarget, networkTarget, f("IRC command", "", true, false, "text", "sent literally to the upstream")}}, nil
	case "server-notice":
		return &AdminForm{Kind: kind, Title: "Broadcast server notice", Fields: []AdminField{f("Message", "", true, false, "text", "sent to every connected bouncer user")}}, nil
	case "server-debug":
		return &AdminForm{Kind: kind, Title: "Change debug logging", Fields: []AdminField{f("Debug", "false", true, false, "bool", "warning: debug logging may include passwords")}}, nil
	default:
		return nil, fmt.Errorf("unknown admin form %q", kind)
	}
}

func (a *App) adminFormKeyLocked(key string, r rune) {
	form := a.admin.form
	if form == nil {
		return
	}
	field := &form.Fields[form.Cursor]
	switch key {
	case "up":
		if form.Cursor > 0 {
			form.Cursor--
		}
	case "down", "tab":
		if form.Cursor < len(form.Fields)-1 {
			form.Cursor++
		}
	case "backtab":
		if form.Cursor > 0 {
			form.Cursor--
		}
	case "backspace":
		if len(field.Value) > 0 {
			field.Value = field.Value[:len(field.Value)-1]
		}
	case "space":
		if field.Kind == "text" {
			field.Value += " "
		} else {
			adminCycleField(field)
		}
	case "enter", "submit":
		if form.Cursor < len(form.Fields)-1 {
			form.Cursor++
		} else {
			a.adminSubmitFormLocked()
		}
	case "esc":
		a.admin.form = nil
		a.admin.view = adminDashboard
		a.setStatusLocked("form cancelled", 3e9)
	default:
		if r != 0 {
			field.Value += string(r)
		}
	}
}

func adminCycleField(field *AdminField) {
	switch field.Kind {
	case "bool":
		if field.Value == "true" {
			field.Value = "false"
		} else {
			field.Value = "true"
		}
	case "optional-bool":
		switch field.Value {
		case "":
			field.Value = "true"
		case "true":
			field.Value = "false"
		default:
			field.Value = ""
		}
	case "select":
		values := field.Options
		if len(values) == 0 {
			values = []string{"", "message", "highlight", "none", "default"}
		}
		for i, value := range values {
			if field.Value == value {
				field.Value = values[(i+1)%len(values)]
				return
			}
		}
		field.Value = values[0]
	}
}

func (a *App) adminSubmitFormLocked() {
	form := a.admin.form
	if form == nil {
		return
	}
	op, err := buildAdminOperation(form)
	if err != nil {
		a.setStatusLocked(err.Error(), 5e9)
		return
	}
	a.admin.form = nil
	if op.Mutating {
		a.admin.confirm = &AdminConfirmation{Operation: op}
		a.admin.view = adminOutput
		a.setStatusLocked("review the operation, then press y to apply or n to cancel", 0)
		return
	}
	a.adminExecuteLocked(op)
}

func (a *App) adminExecuteLocked(op AdminOperation) {
	if a.root == nil {
		a.setStatusLocked("admin connection is not available", 4e9)
		return
	}
	a.admin.output = append(a.admin.output, "> "+op.Preview)
	if err := a.root.Send("PRIVMSG", "BouncerServ", op.Command); err != nil {
		a.admin.output = append(a.admin.output, "send failed: "+err.Error())
		a.setStatusLocked("admin operation failed: "+err.Error(), 5e9)
		return
	}
	a.admin.output = append(a.admin.output, "confirmed: "+op.Summary)
	a.admin.lastMenu = op.Refresh
	a.admin.view = adminOutput
	a.setStatusLocked("operation sent; inspect the result below", 5e9)
}

func buildAdminOperation(form *AdminForm) (AdminOperation, error) {
	values := make(map[string]string, len(form.Fields))
	for _, field := range form.Fields {
		if strings.ContainsAny(field.Value, "\x00\r\n") {
			return AdminOperation{}, fmt.Errorf("%s contains a forbidden control character", field.Label)
		}
		values[field.Label] = field.Value
		if field.Required && strings.TrimSpace(field.Value) == "" {
			return AdminOperation{}, fmt.Errorf("%s is required", field.Label)
		}
	}
	boolArg := func(args *[]string, flag, value string, required bool) error {
		if value == "" && !required {
			return nil
		}
		if value != "true" && value != "false" {
			return fmt.Errorf("%s must be true or false", flag)
		}
		*args = append(*args, flag, value)
		return nil
	}
	userRun := func(user string, commandParts ...string) []string {
		return append([]string{"user", "run", user}, commandParts...)
	}
	networkArgs := func(includeAddress bool) ([]string, error) {
		args := []string{}
		if includeAddress {
			args = append(args, "-addr", values["Address"])
		}
		for _, pair := range [][2]string{{"-name", "Name"}, {"-nick", "Nickname"}, {"-username", "Username"}, {"-pass", "Password"}, {"-realname", "Realname"}, {"-certfp", "CertFP"}, {"-connect-command", "Connect command"}} {
			if values[pair[1]] != "" {
				args = append(args, pair[0], values[pair[1]])
			}
		}
		for _, pair := range [][2]string{{"-auto-away", "Auto-away"}, {"-enabled", "Enabled"}, {"-ignore-limit", "Ignore limit"}} {
			if values[pair[1]] != "" {
				if err := boolArg(&args, pair[0], values[pair[1]], false); err != nil {
					return nil, err
				}
			}
		}
		return args, nil
	}
	channelArgs := func() ([]string, error) {
		args := []string{}
		if values["Detached"] != "" {
			if err := boolArg(&args, "-detached", values["Detached"], false); err != nil {
				return nil, err
			}
		}
		for _, pair := range [][2]string{{"-relay-detached", "Relay detached"}, {"-reattach-on", "Reattach on"}, {"-detach-after", "Detach after"}, {"-detach-on", "Detach on"}} {
			if values[pair[1]] != "" {
				args = append(args, pair[0], values[pair[1]])
			}
		}
		return args, nil
	}
	mutating := true
	refresh := "server status"
	var parts []string
	switch form.Kind {
	case "user-create":
		parts = []string{"user", "create", "-username", values["Username"], "-password", values["Password"]}
		if err := boolArg(&parts, "-admin", values["Admin"], false); err != nil {
			return AdminOperation{}, err
		}
		for _, pair := range [][2]string{{"-nick", "Nickname"}, {"-realname", "Realname"}, {"-max-networks", "Max networks"}} {
			if values[pair[1]] != "" {
				parts = append(parts, pair[0], values[pair[1]])
			}
		}
		if err := boolArg(&parts, "-enabled", values["Enabled"], false); err != nil {
			return AdminOperation{}, err
		}
		refresh = "user status"
	case "user-update":
		parts = []string{"user", "update", values["Username"]}
		if values["New password"] != "" {
			parts = append(parts, "-password", values["New password"])
		}
		if err := boolArg(&parts, "-admin", values["Admin"], true); err != nil {
			return AdminOperation{}, err
		}
		if err := boolArg(&parts, "-enabled", values["Enabled"], true); err != nil {
			return AdminOperation{}, err
		}
		if values["Max networks"] != "" {
			parts = append(parts, "-max-networks", values["Max networks"])
		}
		refresh = "user status"
	case "user-delete":
		parts = []string{"user", "delete", values["Username"]}
		refresh = "user status"
	case "network-create", "network-update":
		args, err := networkArgs(form.Kind == "network-create")
		if err != nil {
			return AdminOperation{}, err
		}
		if form.Kind == "network-create" {
			parts = userRun(values["User"], append([]string{"network", "create"}, args...)...)
		} else {
			parts = userRun(values["User"], append([]string{"network", "update", values["Network"]}, args...)...)
		}
		refresh = adminJoin(userRun(values["User"], "network", "status")...)
	case "network-delete":
		parts = userRun(values["User"], "network", "delete", values["Network"])
		refresh = adminJoin(userRun(values["User"], "network", "status")...)
	case "network-status":
		mutating = false
		parts = userRun(values["User"], "network", "status")
		refresh = adminJoin(parts...)
	case "channel-create", "channel-update":
		args, err := channelArgs()
		if err != nil {
			return AdminOperation{}, err
		}
		if form.Kind == "channel-create" {
			parts = userRun(values["User"], append([]string{"channel", "create", values["Channel"]}, args...)...)
		} else {
			parts = userRun(values["User"], append([]string{"channel", "update", values["Channel"]}, args...)...)
		}
		refresh = adminJoin(userRun(values["User"], "channel", "status", "-network", values["Network"])...)
	case "channel-delete":
		parts = userRun(values["User"], "channel", "delete", values["Channel"])
		refresh = adminJoin(userRun(values["User"], "channel", "status", "-network", values["Network"])...)
	case "channel-status":
		mutating = false
		parts = userRun(values["User"], "channel", "status", "-network", values["Network"])
		refresh = adminJoin(parts...)
	case "cert-generate":
		parts = userRun(values["User"], "certfp", "generate", "-network", values["Network"], "-key-type", values["Key type"], "-bits", values["RSA bits"])
		refresh = adminJoin(userRun(values["User"], "network", "status")...)
	case "cert-fingerprint":
		mutating = false
		parts = userRun(values["User"], "certfp", "fingerprint", "-network", values["Network"])
		refresh = adminJoin(parts...)
	case "sasl-set-plain":
		parts = userRun(values["User"], "sasl", "set-plain", "-network", values["Network"], values["Upstream username"], values["Upstream password"])
		refresh = adminJoin(userRun(values["User"], "network", "status")...)
	case "sasl-reset":
		parts = userRun(values["User"], "sasl", "reset", "-network", values["Network"])
		refresh = adminJoin(userRun(values["User"], "network", "status")...)
	case "network-quote":
		parts = userRun(values["User"], "network", "quote", values["Network"], values["IRC command"])
		refresh = adminJoin(userRun(values["User"], "network", "status")...)
	case "server-notice":
		parts = []string{"server", "notice", values["Message"]}
		refresh = "server status"
	case "server-debug":
		if values["Debug"] != "true" && values["Debug"] != "false" {
			return AdminOperation{}, errors.New("Debug must be true or false")
		}
		parts = []string{"server", "debug", values["Debug"]}
		refresh = "server status"
	default:
		return AdminOperation{}, fmt.Errorf("unsupported admin operation %q", form.Kind)
	}
	if len(parts) == 0 {
		return AdminOperation{}, errors.New("operation has no command")
	}
	commandText := adminJoin(parts...)
	preview := commandText
	for _, secret := range []string{values["Password"], values["New password"], values["Upstream password"]} {
		if secret != "" {
			preview = strings.ReplaceAll(preview, adminQuote(secret), "'••••••'")
		}
	}
	summary := form.Title
	if values["Username"] != "" {
		summary += " for " + values["Username"]
	}
	if values["User"] != "" {
		summary += " for user " + values["User"]
	}
	if values["Network"] != "" {
		summary += " / network " + values["Network"]
	}
	if values["Channel"] != "" {
		summary += " / channel " + values["Channel"]
	}
	return AdminOperation{Summary: summary, Command: commandText, Preview: preview, Refresh: refresh, Mutating: mutating}, nil
}

func adminJoin(parts ...string) string {
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, adminQuote(part))
	}
	return strings.Join(result, " ")
}

func adminQuote(value string) string {
	if value != "" && safeAdminToken.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
