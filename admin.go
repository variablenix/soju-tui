package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type AdminMenuItem struct {
	Label   string
	Kind    string
	Command string
}

func parseAdminCommandHelp(help string) map[string]bool {
	commands := make(map[string]bool)
	lower := strings.ToLower(help)
	marker := "available commands:"
	index := strings.Index(lower, marker)
	if index < 0 {
		return commands
	}
	help = help[index+len(marker):]
	for _, command := range strings.Split(strings.ReplaceAll(help, "\n", ","), ",") {
		command = strings.TrimSpace(command)
		if command != "" {
			commands[command] = true
		}
	}
	return commands
}

var sojuUserStatusLine = regexp.MustCompile(`^([^[:space:]:]+)(?: \([^)]*\))?:`)

func parseFirstSojuUsername(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if match := sojuUserStatusLine.FindStringSubmatch(strings.TrimSpace(line)); len(match) == 2 {
			return match[1]
		}
	}
	return ""
}

func (a *App) adminMenuItemsLocked() []AdminMenuItem {
	items := adminMenuItems()
	if !a.admin.Capabilities.Known {
		return items
	}
	filtered := make([]AdminMenuItem, 0, len(items))
	for _, item := range items {
		if item.Command == "" || a.admin.Capabilities.Commands[item.Command] {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

var safeDisplayArg = regexp.MustCompile(`^[A-Za-z0-9_@+.,:/=-]+$`)

func adminMenuItems() []AdminMenuItem {
	return []AdminMenuItem{
		{Label: "Server status", Kind: "server-status", Command: "server status"},
		{Label: "Server TLS certificate", Kind: "server-tls-certificate"},
		{Label: "List users", Kind: "user-status", Command: "user status"},
		{Label: "Create user", Kind: "user-create", Command: "user create"},
		{Label: "Update user", Kind: "user-update", Command: "user update"},
		{Label: "Delete user", Kind: "user-delete", Command: "user delete"},
		{Label: "Network status for user", Kind: "network-status", Command: "network status"},
		{Label: "Create network for user", Kind: "network-create", Command: "network create"},
		{Label: "Update network for user", Kind: "network-update", Command: "network update"},
		{Label: "Delete network for user", Kind: "network-delete", Command: "network delete"},
		{Label: "Send raw network command", Kind: "network-quote", Command: "network quote"},
		{Label: "Channel status for user", Kind: "channel-status", Command: "channel status"},
		{Label: "Create channel for user", Kind: "channel-create", Command: "channel create"},
		{Label: "Update channel for user", Kind: "channel-update", Command: "channel update"},
		{Label: "Delete channel for user", Kind: "channel-delete", Command: "channel delete"},
		{Label: "Generate upstream SASL certificate", Kind: "cert-generate", Command: "certfp generate"},
		{Label: "Show upstream CertFP fingerprints", Kind: "cert-fingerprint", Command: "certfp fingerprint"},
		{Label: "Show SASL status", Kind: "sasl-status", Command: "sasl status"},
		{Label: "Set SASL PLAIN", Kind: "sasl-set-plain", Command: "sasl set-plain"},
		{Label: "Reset SASL", Kind: "sasl-reset", Command: "sasl reset"},
		{Label: "Client device certificates", Kind: "device-cert-status", Command: "device-certificate status"},
		{Label: "Register client device certificate", Kind: "device-cert-create", Command: "device-certificate create"},
		{Label: "Delete client device certificate", Kind: "device-cert-delete", Command: "device-certificate delete"},
		{Label: "Broadcast server notice", Kind: "server-notice", Command: "server notice"},
		{Label: "Toggle server debug", Kind: "server-debug", Command: "server debug"},
		{Label: "BouncerServ help", Kind: "help", Command: "help"},
	}
}

func (a *App) adminHandleKey(key string, r rune) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.admin.ExitConfirm {
		switch {
		case key == "esc", r == 'n' || r == 'N':
			a.admin.ExitConfirm = false
			a.setStatusLocked("exit cancelled", 3e9)
		case r == 'y' || r == 'Y':
			a.admin.ExitConfirm = false
			a.closeLocked()
		}
		return
	}
	if key == "quit" || ((r == 'q' || r == 'Q') && a.admin.Form == nil && a.admin.Confirm == nil) {
		a.admin.ExitConfirm = true
		if a.admin.Busy {
			a.setStatusLocked("confirm exit; the running operation will be cancelled", 0)
		} else {
			a.setStatusLocked("confirm exit", 0)
		}
		return
	}
	if a.admin.Busy {
		return
	}
	if a.admin.Confirm != nil {
		confirmation := a.admin.Confirm
		if confirmation.Operation.ConfirmPhrase != "" {
			switch key {
			case "esc":
				a.admin.Confirm = nil
				a.setStatusLocked("operation cancelled", 3e9)
			case "backspace":
				if len(confirmation.Input) > 0 {
					confirmation.Input = confirmation.Input[:len(confirmation.Input)-1]
				}
			case "enter":
				if string(confirmation.Input) != confirmation.Operation.ConfirmPhrase {
					a.setStatusLocked("confirmation phrase does not match", 4e9)
					return
				}
				op := confirmation.Operation
				a.admin.Confirm = nil
				a.mu.Unlock()
				a.requestOperation(op)
				a.mu.Lock()
			default:
				if r != 0 && r >= 0x20 && r != 0x7f {
					confirmation.Input = append(confirmation.Input, r)
				}
			}
			return
		}
		switch {
		case key == "esc", r == 'n' || r == 'N':
			a.admin.Confirm = nil
			a.setStatusLocked("operation cancelled", 3e9)
		case r == 'y' || r == 'Y':
			op := a.admin.Confirm.Operation
			a.admin.Confirm = nil
			a.mu.Unlock()
			a.requestOperation(op)
			a.mu.Lock()
		}
		return
	}
	if a.admin.Form != nil {
		a.adminFormKeyLocked(key, r)
		return
	}
	items := a.adminMenuItemsLocked()
	switch key {
	case "up":
		if len(items) > 0 {
			a.admin.Cursor = (a.admin.Cursor - 1 + len(items)) % len(items)
		}
	case "down":
		if len(items) > 0 {
			a.admin.Cursor = (a.admin.Cursor + 1) % len(items)
		}
	case "home":
		a.admin.Cursor = 0
	case "end":
		if len(items) > 0 {
			a.admin.Cursor = len(items) - 1
		}
	case "enter":
		a.adminActivateMenuLocked(a.admin.Cursor)
	case "esc":
		a.admin.View = adminDashboard
	case "r":
		a.adminRefreshLocked()
	}
}

func (a *App) adminActivateMenuLocked(cursor int) {
	items := a.adminMenuItemsLocked()
	if cursor < 0 || cursor >= len(items) {
		return
	}
	item := items[cursor]
	switch item.Kind {
	case "server-status":
		a.adminRequestReadOnlyLocked("Server status", []string{"server", "status"})
	case "server-tls-certificate":
		a.adminShowServerTLSLocked()
	case "user-status":
		a.adminRequestReadOnlyLocked("List users", []string{"user", "status"})
	case "help":
		a.adminRequestReadOnlyLocked("BouncerServ help", []string{"help"})
	case "device-cert-status":
		a.adminOpenFormLocked(item.Kind)
	default:
		a.adminOpenFormLocked(item.Kind)
	}
}

func (a *App) adminRequestReadOnlyLocked(summary string, args []string) {
	op := makeAdminOperation(a.backend.Config, summary, args, args, false, nil)
	a.mu.Unlock()
	a.requestOperation(op)
	a.mu.Lock()
}

func (a *App) adminRefreshLocked() {
	args := a.admin.LastRefresh
	if len(args) == 0 {
		args = []string{"server", "status"}
	}
	op := makeAdminOperation(a.backend.Config, "Refresh", args, args, false, nil)
	a.mu.Unlock()
	a.requestOperation(op)
	a.mu.Lock()
}

func (a *App) adminOpenFormLocked(kind string) {
	form, err := newAdminForm(kind)
	if err != nil {
		a.setStatusLocked(err.Error(), 5e9)
		return
	}
	a.admin.Form = form
	a.admin.View = adminForm
	a.admin.Confirm = nil
	a.setStatusLocked("Enter advances · Space cycles choices · Ctrl-S previews · Esc cancels", 0)
}

func newAdminForm(kind string) (*AdminForm, error) {
	field := func(label, value string, required, secret bool, fieldKind, help string) AdminField {
		return AdminField{Label: label, Value: value, Required: required, Secret: secret, Kind: fieldKind, Help: help}
	}
	boolField := func(label, value string, required bool) AdminField {
		return field(label, value, required, false, "bool", "Space cycles true/false")
	}
	optionalBool := func(label string) AdminField {
		return field(label, "", false, false, "optional-bool", "Space cycles unset/true/false")
	}
	selectField := func(label, help string, options ...string) AdminField {
		value := ""
		if len(options) > 0 {
			value = options[0]
		}
		return AdminField{Label: label, Value: value, Original: value, Kind: "select", Help: help, Options: options}
	}
	userTarget := field("User", "", true, false, "text", "soju username")
	networkTarget := field("Network", "", true, false, "text", "network name or address")

	baseNetwork := []AdminField{
		userTarget,
		networkTarget,
		field("Address", "", true, false, "text", "ircs://host[:port], irc+insecure://host, or irc+unix:///path"),
		field("Name", "", false, false, "text", "short display name"),
		field("Nickname", "", false, false, "text", "upstream nickname"),
		field("Username", "", false, false, "text", "upstream username"),
		field("Password", "", false, true, "text", "upstream server password"),
		field("Realname", "", false, false, "text", "upstream real name"),
		field("CertFP", "", false, false, "text", "SHA-512 certificate fingerprint"),
		optionalBool("Auto-away"),
		optionalBool("Enabled"),
		optionalBool("Ignore limit"),
		field("Connect command", "", false, false, "text", "raw IRC command sent after connecting"),
	}
	baseChannel := []AdminField{
		userTarget,
		networkTarget,
		field("Channel", "", true, false, "text", "channel name, e.g. #chat"),
		optionalBool("Detached"),
		selectField("Relay detached", "message, highlight, none, default", "", "message", "highlight", "none", "default"),
		selectField("Reattach on", "message, highlight, none, default", "", "message", "highlight", "none", "default"),
		field("Detach after", "", false, false, "text", "duration such as 30m or 0"),
		selectField("Detach on", "message, highlight, none, default", "", "message", "highlight", "none", "default"),
	}

	switch kind {
	case "user-create":
		return &AdminForm{Kind: kind, Title: "Create soju user", Fields: []AdminField{
			field("Username", "", true, false, "text", "immutable account name"),
			field("Password", "", false, true, "text", "bouncer login password; omit only when password is disabled"),
			boolField("Admin", "false", false),
			field("Nickname", "", false, false, "text", "fallback IRC nickname"),
			field("Realname", "", false, false, "text", "fallback IRC real name"),
			boolField("Enabled", "true", false),
			field("Max networks", "-1", false, false, "text", "0 none, -1 global default"),
			optionalBool("Disable password"),
		}}, nil
	case "user-update":
		return &AdminForm{Kind: kind, Title: "Update soju user", Fields: []AdminField{
			field("Username", "", true, false, "text", "account to update"),
			field("New password", "", false, true, "text", "leave blank to keep current"),
			optionalBool("Admin"),
			optionalBool("Enabled"),
			field("Max networks", "", false, false, "text", "leave blank to keep current"),
			optionalBool("Disable password"),
		}}, nil
	case "user-delete":
		return &AdminForm{Kind: kind, Title: "Delete soju user", Fields: []AdminField{field("Username", "", true, false, "text", "account to delete")}}, nil
	case "network-create":
		fields := append([]AdminField{baseNetwork[0]}, baseNetwork[2:]...)
		return &AdminForm{Kind: kind, Title: "Create network for user", Fields: fields}, nil
	case "network-update":
		return &AdminForm{Kind: "network-update-lookup", Title: "Load network settings", Fields: []AdminField{
			userTarget,
			networkTarget,
		}}, nil
	case "network-delete":
		return &AdminForm{Kind: kind, Title: "Delete network for user", Fields: []AdminField{userTarget, networkTarget}}, nil
	case "network-status":
		return &AdminForm{Kind: kind, Title: "Show networks for user", Fields: []AdminField{userTarget}}, nil
	case "network-quote":
		return &AdminForm{Kind: kind, Title: "Send raw command to network", Fields: []AdminField{userTarget, networkTarget, field("IRC command", "", true, false, "text", "sent literally to the upstream")}}, nil
	case "channel-create", "channel-update":
		return &AdminForm{Kind: kind, Title: map[string]string{"channel-create": "Create channel for user", "channel-update": "Update channel for user"}[kind], Fields: baseChannel}, nil
	case "channel-delete", "channel-status":
		fields := []AdminField{userTarget, networkTarget}
		if kind == "channel-delete" {
			fields = append(fields, field("Channel", "", true, false, "text", "channel to delete"))
		}
		return &AdminForm{Kind: kind, Title: map[string]string{"channel-delete": "Delete channel for user", "channel-status": "Show channels for user"}[kind], Fields: fields}, nil
	case "cert-generate":
		return &AdminForm{Kind: kind, Title: "Generate upstream SASL certificate", Fields: []AdminField{userTarget, networkTarget, selectField("Key type", "replaces the network's SASL EXTERNAL key: rsa, ecdsa, ed25519", "rsa", "ecdsa", "ed25519"), field("RSA bits", "3072", false, false, "text", "ignored for ecdsa/ed25519")}}, nil
	case "cert-fingerprint":
		return &AdminForm{Kind: kind, Title: "Show upstream CertFP fingerprints", Fields: []AdminField{userTarget, networkTarget}}, nil
	case "sasl-status":
		return &AdminForm{Kind: kind, Title: "Show SASL status", Fields: []AdminField{userTarget, networkTarget}}, nil
	case "sasl-set-plain":
		return &AdminForm{Kind: kind, Title: "Set SASL PLAIN", Fields: []AdminField{userTarget, networkTarget, field("Upstream username", "", true, false, "text", "IRC account username"), field("Upstream password", "", true, true, "text", "IRC account password")}}, nil
	case "sasl-reset":
		return &AdminForm{Kind: kind, Title: "Reset SASL", Fields: []AdminField{userTarget, networkTarget}}, nil
	case "device-cert-status":
		return &AdminForm{Kind: kind, Title: "Show client device certificates", Fields: []AdminField{userTarget}}, nil
	case "device-cert-create":
		return &AdminForm{Kind: kind, Title: "Register client device certificate", Fields: []AdminField{userTarget, field("Fingerprint", "", true, false, "text", "downstream client certificate fingerprint; requires client-cert-auth"), field("Label", "", true, false, "text", "human-readable device label")}}, nil
	case "device-cert-delete":
		return &AdminForm{Kind: kind, Title: "Delete device certificate", Fields: []AdminField{userTarget, field("Fingerprint", "", true, false, "text", "certificate fingerprint")}}, nil
	case "server-notice":
		return &AdminForm{Kind: kind, Title: "Broadcast server notice", Fields: []AdminField{field("Message", "", true, false, "text", "sent to every connected bouncer user")}}, nil
	case "server-debug":
		return &AdminForm{Kind: kind, Title: "Change debug logging", Fields: []AdminField{boolField("Debug", "false", true)}}, nil
	default:
		return nil, fmt.Errorf("unknown admin form %q", kind)
	}
}

func (a *App) adminFormKeyLocked(key string, r rune) {
	form := a.admin.Form
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
		if field.Kind == "text" {
			runes := []rune(field.Value)
			if len(runes) > 0 {
				field.Value = string(runes[:len(runes)-1])
			}
		}
	case "space":
		if field.Kind == "text" {
			field.Value += " "
		} else {
			adminCycleField(field)
		}
	case "enter":
		if form.Cursor < len(form.Fields)-1 {
			form.Cursor++
		} else {
			a.adminSubmitFormLocked()
		}
	case "submit":
		a.adminSubmitFormLocked()
	case "esc":
		a.admin.Form = nil
		a.admin.View = adminDashboard
		a.setStatusLocked("form cancelled", 3e9)
	default:
		if r != 0 && field.Kind == "text" {
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
		for i, value := range field.Options {
			if field.Value == value {
				field.Value = field.Options[(i+1)%len(field.Options)]
				return
			}
		}
		field.Value = field.Options[0]
	}
}

func (a *App) adminSubmitFormLocked() {
	form := a.admin.Form
	if form == nil {
		return
	}
	op, err := buildAdminOperation(a.backend.Config, form)
	if err != nil {
		a.setStatusLocked(err.Error(), 5e9)
		return
	}
	a.admin.Form = nil
	if op.Mutating {
		a.admin.Confirm = &AdminConfirmation{Operation: op}
		a.admin.View = adminOutput
		if op.ConfirmPhrase != "" {
			a.setStatusLocked("type the displayed phrase exactly and press Enter, or Esc to cancel", 0)
		} else {
			a.setStatusLocked("review the redacted command, then press y to apply or n to cancel", 0)
		}
		return
	}
	a.mu.Unlock()
	a.requestOperation(op)
	a.mu.Lock()
}

func newNetworkUpdateForm(user string, network NetworkStatus) *AdminForm {
	field := func(label, value string, required, secret bool, kind, help string) AdminField {
		return AdminField{Label: label, Value: value, Original: value, Required: required, Secret: secret, Kind: kind, Help: help}
	}
	optional := func(label, value, help string) AdminField {
		return field(label, value, false, false, "optional-bool", help)
	}
	enabled := "true"
	if network.Disabled {
		enabled = "false"
	}
	return &AdminForm{Kind: "network-update", Title: "Update network for user", Fields: []AdminField{
		field("User", user, true, false, "readonly", "loaded target; cancel to choose a different user"),
		field("Network", network.Target(), true, false, "readonly", "loaded target; cancel to choose a different network"),
		field("Address", network.Address, false, false, "text", "loaded from sojuctl; cannot be empty"),
		field("Name", network.Name, false, false, "text", "loaded from sojuctl; blank clears it only if changed"),
		field("Nickname", "", false, false, "text", "not exposed by sojuctl; blank keeps current"),
		field("Username", "", false, false, "text", "not exposed by sojuctl; blank keeps current"),
		field("Password", "", false, true, "text", "never read or displayed; blank keeps current"),
		field("Realname", "", false, false, "text", "not exposed by sojuctl; blank keeps current"),
		field("CertFP", "", false, false, "text", "not exposed by sojuctl; blank keeps current"),
		optional("Auto-away", "", "not exposed by sojuctl; unset keeps current"),
		optional("Enabled", enabled, "loaded from network status"),
		optional("Ignore limit", "", "not exposed by sojuctl; unset keeps current"),
		field("Connect command", "", false, false, "text", "not exposed by sojuctl; blank keeps current"),
	}}
}

func buildAdminOperation(config string, form *AdminForm) (AdminOperation, error) {
	values := make(map[string]string, len(form.Fields))
	originals := make(map[string]string, len(form.Fields))
	for _, field := range form.Fields {
		if strings.ContainsAny(field.Value, "\x00\r\n") {
			return AdminOperation{}, fmt.Errorf("%s contains a forbidden control character", field.Label)
		}
		values[field.Label] = field.Value
		originals[field.Label] = field.Original
		if field.Required && strings.TrimSpace(field.Value) == "" {
			return AdminOperation{}, fmt.Errorf("%s is required", field.Label)
		}
	}
	if form.Kind == "network-update" {
		if values["User"] != originals["User"] || values["Network"] != originals["Network"] {
			return AdminOperation{}, errors.New("User and Network identify the loaded target and cannot be changed; cancel and load a different network")
		}
		if originals["Address"] != "" && strings.TrimSpace(values["Address"]) == "" {
			return AdminOperation{}, errors.New("Address cannot be empty; cancel and delete the network if it is no longer needed")
		}
	}
	boolArg := func(args *[]string, flagName, value string) error {
		if value == "" {
			return nil
		}
		if value != "true" && value != "false" {
			return fmt.Errorf("%s must be true or false", flagName)
		}
		*args = append(*args, flagName, value)
		return nil
	}
	presenceFlag := func(args *[]string, flagName, value string) error {
		if value == "" {
			return nil
		}
		if value != "true" && value != "false" {
			return fmt.Errorf("%s must be true or false", flagName)
		}
		if value == "true" {
			*args = append(*args, flagName)
		}
		return nil
	}
	userRun := func(user string, commandParts ...string) []string {
		return append([]string{"user", "run", user}, commandParts...)
	}
	networkArgs := func(create bool) ([]string, error) {
		args := []string{}
		if create {
			args = append(args, "-addr", values["Address"])
		} else if values["Address"] != originals["Address"] {
			args = append(args, "-addr", values["Address"])
		}
		for _, pair := range [][2]string{{"-name", "Name"}, {"-nick", "Nickname"}, {"-username", "Username"}, {"-pass", "Password"}, {"-realname", "Realname"}, {"-certfp", "CertFP"}, {"-connect-command", "Connect command"}} {
			if (create && values[pair[1]] != "") || (!create && values[pair[1]] != originals[pair[1]]) {
				args = append(args, pair[0], values[pair[1]])
			}
		}
		for _, pair := range [][2]string{{"-auto-away", "Auto-away"}, {"-enabled", "Enabled"}} {
			value := values[pair[1]]
			if !create && value == originals[pair[1]] {
				value = ""
			}
			if err := boolArg(&args, pair[0], value); err != nil {
				return nil, err
			}
		}
		ignoreLimit := values["Ignore limit"]
		if !create && ignoreLimit == originals["Ignore limit"] {
			ignoreLimit = ""
		}
		if err := presenceFlag(&args, "-ignore-limit", ignoreLimit); err != nil {
			return nil, err
		}
		return args, nil
	}
	channelArgs := func() ([]string, error) {
		args := []string{}
		if err := boolArg(&args, "-detached", values["Detached"]); err != nil {
			return nil, err
		}
		for _, pair := range [][2]string{{"-relay-detached", "Relay detached"}, {"-reattach-on", "Reattach on"}, {"-detach-after", "Detach after"}, {"-detach-on", "Detach on"}} {
			if values[pair[1]] != "" {
				args = append(args, pair[0], values[pair[1]])
			}
		}
		return args, nil
	}

	mutating := true
	refresh := []string{"server", "status"}
	secrets := []string{}
	var args []string
	summary := form.Title
	switch form.Kind {
	case "user-create":
		if values["Disable password"] == "true" && values["Password"] != "" {
			return AdminOperation{}, errors.New("Password must be empty when Disable password is true")
		}
		if values["Disable password"] != "true" && strings.TrimSpace(values["Password"]) == "" {
			return AdminOperation{}, errors.New("Password is required unless Disable password is true")
		}
		args = []string{"user", "create", "-username", values["Username"]}
		if values["Password"] != "" {
			args = append(args, "-password", values["Password"])
		}
		secrets = append(secrets, values["Password"])
		if values["Admin"] != "false" {
			if err := boolArg(&args, "-admin", values["Admin"]); err != nil {
				return AdminOperation{}, err
			}
		}
		for _, pair := range [][2]string{{"-nick", "Nickname"}, {"-realname", "Realname"}} {
			if values[pair[1]] != "" {
				args = append(args, pair[0], values[pair[1]])
			}
		}
		if values["Max networks"] != "" && values["Max networks"] != "-1" {
			args = append(args, "-max-networks", values["Max networks"])
		}
		if values["Enabled"] != "true" {
			if err := boolArg(&args, "-enabled", values["Enabled"]); err != nil {
				return AdminOperation{}, err
			}
		}
		if err := presenceFlag(&args, "-disable-password", values["Disable password"]); err != nil {
			return AdminOperation{}, err
		}
		refresh = []string{"user", "status"}
	case "user-update":
		if values["Disable password"] == "true" && values["New password"] != "" {
			return AdminOperation{}, errors.New("New password must be empty when Disable password is true")
		}
		args = []string{"user", "update", values["Username"]}
		if values["New password"] != "" {
			args = append(args, "-password", values["New password"])
			secrets = append(secrets, values["New password"])
		}
		for _, pair := range [][2]string{{"-admin", "Admin"}, {"-enabled", "Enabled"}} {
			if err := boolArg(&args, pair[0], values[pair[1]]); err != nil {
				return AdminOperation{}, err
			}
		}
		if err := presenceFlag(&args, "-disable-password", values["Disable password"]); err != nil {
			return AdminOperation{}, err
		}
		if values["Max networks"] != "" {
			args = append(args, "-max-networks", values["Max networks"])
		}
		refresh = []string{"user", "status"}
	case "user-delete":
		args = []string{"user", "delete", values["Username"]}
		refresh = []string{"user", "status"}
	case "network-create", "network-update":
		networkOptions, err := networkArgs(form.Kind == "network-create")
		if err != nil {
			return AdminOperation{}, err
		}
		if form.Kind == "network-create" {
			args = userRun(values["User"], append([]string{"network", "create"}, networkOptions...)...)
		} else {
			if len(networkOptions) == 0 {
				return AdminOperation{}, errors.New("No network settings changed")
			}
			args = userRun(values["User"], append([]string{"network", "update", values["Network"]}, networkOptions...)...)
		}
		secrets = append(secrets, values["Password"])
		refresh = userRun(values["User"], "network", "status")
	case "network-update-lookup":
		mutating = false
		args = userRun(values["User"], "network", "status")
		refresh = args
	case "network-delete":
		args = userRun(values["User"], "network", "delete", values["Network"])
		refresh = userRun(values["User"], "network", "status")
	case "network-status":
		mutating = false
		args = userRun(values["User"], "network", "status")
		refresh = args
	case "network-quote":
		args = userRun(values["User"], "network", "quote", values["Network"], values["IRC command"])
		refresh = userRun(values["User"], "network", "status")
	case "channel-create", "channel-update":
		channelOptions, err := channelArgs()
		if err != nil {
			return AdminOperation{}, err
		}
		if form.Kind == "channel-create" {
			args = userRun(values["User"], append([]string{"channel", "create", values["Channel"] + "/" + values["Network"]}, channelOptions...)...)
		} else {
			args = userRun(values["User"], append([]string{"channel", "update", values["Channel"] + "/" + values["Network"]}, channelOptions...)...)
		}
		refresh = userRun(values["User"], "channel", "status", "-network", values["Network"])
	case "channel-delete":
		args = userRun(values["User"], "channel", "delete", values["Channel"]+"/"+values["Network"])
		refresh = userRun(values["User"], "channel", "status", "-network", values["Network"])
	case "channel-status":
		mutating = false
		args = userRun(values["User"], "channel", "status", "-network", values["Network"])
		refresh = args
	case "cert-generate":
		args = userRun(values["User"], "certfp", "generate", "-network", values["Network"])
		if values["Key type"] != "" && values["Key type"] != "rsa" {
			args = append(args, "-key-type", values["Key type"])
		}
		if (values["Key type"] == "" || values["Key type"] == "rsa") && values["RSA bits"] != "" && values["RSA bits"] != "3072" {
			args = append(args, "-bits", values["RSA bits"])
		}
		refresh = userRun(values["User"], "network", "status")
	case "cert-fingerprint":
		mutating = false
		args = userRun(values["User"], "certfp", "fingerprint", "-network", values["Network"])
		refresh = args
	case "sasl-status":
		mutating = false
		args = userRun(values["User"], "sasl", "status", "-network", values["Network"])
		refresh = args
	case "sasl-set-plain":
		args = userRun(values["User"], "sasl", "set-plain", "-network", values["Network"], values["Upstream username"], values["Upstream password"])
		secrets = append(secrets, values["Upstream password"])
		refresh = userRun(values["User"], "sasl", "status", "-network", values["Network"])
	case "sasl-reset":
		args = userRun(values["User"], "sasl", "reset", "-network", values["Network"])
		refresh = userRun(values["User"], "sasl", "status", "-network", values["Network"])
	case "device-cert-status":
		mutating = false
		args = userRun(values["User"], "device-certificate", "status")
		refresh = args
	case "device-cert-create":
		args = userRun(values["User"], "device-certificate", "create", "-fingerprint", values["Fingerprint"], "-label", values["Label"])
		refresh = userRun(values["User"], "device-certificate", "status")
	case "device-cert-delete":
		args = userRun(values["User"], "device-certificate", "delete", values["Fingerprint"])
		refresh = userRun(values["User"], "device-certificate", "status")
	case "server-notice":
		args = []string{"server", "notice", values["Message"]}
		refresh = []string{"server", "status"}
	case "server-debug":
		if values["Debug"] != "true" && values["Debug"] != "false" {
			return AdminOperation{}, errors.New("Debug must be true or false")
		}
		args = []string{"server", "debug", values["Debug"]}
		refresh = []string{"server", "status"}
	default:
		return AdminOperation{}, fmt.Errorf("unsupported admin operation %q", form.Kind)
	}
	if len(args) == 0 {
		return AdminOperation{}, errors.New("operation has no command")
	}
	if values["User"] != "" {
		summary += " for user " + values["User"]
	}
	if values["Username"] != "" {
		summary += " / " + values["Username"]
	}
	if values["Network"] != "" {
		summary += " / network " + values["Network"]
	}
	if values["Channel"] != "" {
		summary += " / channel " + values["Channel"]
	}
	op := makeAdminOperation(config, summary, args, refresh, mutating, secrets)
	switch form.Kind {
	case "network-update-lookup":
		op.FollowUpKind = "network-update"
		op.TargetUser = values["User"]
		op.TargetNetwork = values["Network"]
	case "user-delete":
		op.ConfirmPhrase = "DELETE USER " + values["Username"]
	case "network-delete":
		op.ConfirmPhrase = "DELETE NETWORK " + values["Network"]
	case "network-quote":
		op.ConfirmPhrase = "SEND RAW COMMAND"
	case "channel-delete":
		op.ConfirmPhrase = "DELETE CHANNEL " + values["Channel"]
	case "cert-generate":
		op.ConfirmPhrase = "REPLACE CERTIFICATE"
	case "sasl-reset":
		op.ConfirmPhrase = "RESET SASL"
	case "device-cert-delete":
		op.ConfirmPhrase = "DELETE DEVICE CERTIFICATE"
	case "server-debug":
		if values["Debug"] == "true" {
			op.ConfirmPhrase = "ENABLE DEBUG"
		}
	}
	if form.Kind == "user-create" {
		op.CapabilityUser = values["Username"]
	}
	if form.Kind == "user-delete" {
		op.NeedsSojuConfirmation = true
	}
	return op, nil
}

func makeAdminOperation(config, summary string, args, refresh []string, mutating bool, secrets []string) AdminOperation {
	previewArgs := append([]string(nil), args...)
	for i, value := range previewArgs {
		for _, secret := range secrets {
			if secret != "" && value == secret {
				previewArgs[i] = "••••••"
			}
		}
	}
	return AdminOperation{
		Summary:               summary,
		Args:                  append([]string(nil), args...),
		Refresh:               append([]string(nil), refresh...),
		Mutating:              mutating,
		Secrets:               append([]string(nil), secrets...),
		Preview:               formatSojuCtlCommand(config, previewArgs),
		NeedsSojuConfirmation: false,
	}
}

var userDeleteConfirmation = regexp.MustCompile(`To confirm user deletion, send "user delete ([^"[:space:]]+) ([0-9a-f]+)"`)

func parseUserDeleteConfirmation(output string) ([]string, string, bool) {
	match := userDeleteConfirmation.FindStringSubmatch(output)
	if len(match) != 3 {
		return nil, "", false
	}
	return []string{"user", "delete", match[1], match[2]}, match[1], true
}

func redactText(text string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, "••••••")
		}
	}
	return text
}
