package main

import (
	"context"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func newTestApp() *App {
	ctx, cancel := context.WithCancel(context.Background())
	return &App{
		ctx:     ctx,
		cancel:  cancel,
		results: make(chan adminResult, 1),
		done:    make(chan struct{}),
	}
}

func TestQuitRequiresConfirmation(t *testing.T) {
	app := newTestApp()
	app.adminHandleKey("q", 'q')
	if !app.admin.ExitConfirm || app.quit {
		t.Fatalf("first q must prompt without exiting: exitConfirm=%v quit=%v", app.admin.ExitConfirm, app.quit)
	}

	app.adminHandleKey("", 'n')
	if app.admin.ExitConfirm || app.quit {
		t.Fatalf("n must cancel exit: exitConfirm=%v quit=%v", app.admin.ExitConfirm, app.quit)
	}

	app.adminHandleKey("quit", 0)
	app.adminHandleKey("", 'y')
	if !app.quit {
		t.Fatal("y must confirm exit")
	}
	select {
	case <-app.ctx.Done():
	default:
		t.Fatal("confirmed exit must cancel the application context")
	}
}

func TestQuitRuneRemainsTextInsideForm(t *testing.T) {
	app := newTestApp()
	app.admin.Form = &AdminForm{Fields: []AdminField{{Kind: "text"}}}
	app.adminHandleKey("Q", 'Q')
	if app.admin.ExitConfirm {
		t.Fatal("q in a text field must not open exit confirmation")
	}
	if got := app.admin.Form.Fields[0].Value; got != "Q" {
		t.Fatalf("text field value = %q, want Q", got)
	}
	app.close()
}

func TestCtrlCRequiresConfirmationAndCancelsRunningOperation(t *testing.T) {
	app := newTestApp()
	app.admin.Busy = true
	handleAdminKeyEvent(app, tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone))
	if !app.admin.ExitConfirm || app.quit {
		t.Fatalf("Ctrl-C must prompt without exiting: exitConfirm=%v quit=%v", app.admin.ExitConfirm, app.quit)
	}
	app.adminHandleKey("", 'y')
	if !app.quit {
		t.Fatal("confirmed exit must close the application")
	}
	select {
	case <-app.ctx.Done():
	default:
		t.Fatal("confirmed exit must cancel the running operation context")
	}
}

func TestPermissionDeniedHint(t *testing.T) {
	output := "dial /run/soju/admin: dial unix /run/soju/admin: connect: permission denied\n"
	hint := sojuCtlFailureHint(output)
	if !strings.Contains(hint, "ADMIN SOCKET ACCESS DENIED") ||
		!strings.Contains(hint, "scripts/setup.sh") ||
		!strings.Contains(hint, "/run/soju/admin") {
		t.Fatalf("unexpected permission hint: %q", hint)
	}
}

func TestUnrelatedFailureHasNoPermissionHint(t *testing.T) {
	if hint := sojuCtlFailureHint("connection refused"); hint != "" {
		t.Fatalf("unexpected hint: %q", hint)
	}
}

func TestTypedConfirmationRequiresExactPhrase(t *testing.T) {
	app := newTestApp()
	app.backend = &SojuCtl{Path: "/bin/false"}
	app.admin.Confirm = &AdminConfirmation{Operation: AdminOperation{
		ConfirmPhrase: "RESET SASL",
	}}
	for _, char := range "reset sasl" {
		app.adminHandleKey(string(char), char)
	}
	app.adminHandleKey("enter", 0)
	if app.admin.Confirm == nil || app.admin.Busy {
		t.Fatal("a mismatched phrase must not execute or close confirmation")
	}
	for range []rune("reset sasl") {
		app.adminHandleKey("backspace", 0)
	}
	for _, char := range "RESET SASL" {
		app.adminHandleKey(string(char), char)
	}
	app.adminHandleKey("enter", 0)
	if app.admin.Confirm != nil || !app.admin.Busy {
		t.Fatal("the exact phrase must execute the operation")
	}
	app.close()
}

func TestDeviceCertificateDisabledHint(t *testing.T) {
	hint := sojuCtlFailureHint("client certificate authentication is disabled")
	if !strings.Contains(hint, "CLIENT DEVICE CERTIFICATES ARE DISABLED") ||
		!strings.Contains(hint, "Server TLS certificate") {
		t.Fatalf("unexpected device-certificate hint: %q", hint)
	}
}

func TestDeviceCertificateUnsupportedHint(t *testing.T) {
	output := `command "device-certificate" not found (type "help" for a list of commands)`
	hint := sojuCtlFailureHint(output)
	if !isDeviceCertificateUnsupported(output) ||
		!strings.Contains(hint, "NOT SUPPORTED BY THIS SOJU SERVER") ||
		!strings.Contains(hint, "Server TLS certificate") {
		t.Fatalf("unexpected unsupported-command hint: %q", hint)
	}
}

func TestMenuNavigationWrapsAndSupportsHomeEnd(t *testing.T) {
	app := newTestApp()
	items := adminMenuItems()
	app.admin.Cursor = 0
	app.adminHandleKey("up", 0)
	if app.admin.Cursor != len(items)-1 {
		t.Fatalf("up from first item moved to %d, want %d", app.admin.Cursor, len(items)-1)
	}
	app.adminHandleKey("down", 0)
	if app.admin.Cursor != 0 {
		t.Fatalf("down from last item moved to %d, want 0", app.admin.Cursor)
	}
	app.adminHandleKey("end", 0)
	if app.admin.Cursor != len(items)-1 {
		t.Fatalf("end moved to %d, want %d", app.admin.Cursor, len(items)-1)
	}
	app.adminHandleKey("home", 0)
	if app.admin.Cursor != 0 {
		t.Fatalf("home moved to %d, want 0", app.admin.Cursor)
	}
	app.close()
}

func TestUnsupportedDeviceCertificateActionsAreOmitted(t *testing.T) {
	app := newTestApp()
	app.admin.Capabilities = AdminCapabilities{Known: true, Commands: map[string]bool{
		"help":          true,
		"server status": true,
		"user status":   true,
	}}
	for _, item := range app.adminMenuItemsLocked() {
		if strings.HasPrefix(item.Kind, "device-cert-") {
			t.Fatalf("unsupported device-certificate action remained visible: %#v", item)
		}
	}
	app.close()
}

func TestParseAdminCommandHelp(t *testing.T) {
	commands := parseAdminCommandHelp("available commands: help, network status, server status\n")
	for _, command := range []string{"help", "network status", "server status"} {
		if !commands[command] {
			t.Fatalf("command %q missing from %#v", command, commands)
		}
	}
	if commands["device-certificate status"] {
		t.Fatalf("unexpected device-certificate command in %#v", commands)
	}
}

func TestParseAdminCommandHelpRejectsUnexpectedOutput(t *testing.T) {
	commands := parseAdminCommandHelp("permission denied\nnetwork create")
	if len(commands) != 0 {
		t.Fatalf("unexpected output produced capabilities: %#v", commands)
	}
}

func TestParseFirstSojuUsername(t *testing.T) {
	output := "2/2 users\nak (admin): 3 networks\nakmobile: 2 networks\n"
	if got := parseFirstSojuUsername(output); got != "ak" {
		t.Fatalf("username = %q, want ak", got)
	}
}

func TestParseSojuUsernames(t *testing.T) {
	output := "ak (admin): 3 networks\nalice: 1 networks\nak (admin): 3 networks\n(2 more users omitted)\n"
	users := parseSojuUsernames(output)
	if strings.Join(users, ",") != "ak,alice" {
		t.Fatalf("users = %#v", users)
	}
}

func TestUserTargetFormsReceiveDiscoverableAndCustomSelector(t *testing.T) {
	userTargetedKinds := []string{
		"user-update", "user-delete",
		"network-create", "network-update", "network-delete", "network-status", "network-quote",
		"channel-create", "channel-update", "channel-delete", "channel-status",
		"cert-generate", "cert-fingerprint",
		"sasl-status", "sasl-set-plain", "sasl-reset",
		"device-cert-status", "device-cert-create", "device-cert-delete",
	}
	for _, kind := range userTargetedKinds {
		if !adminFormRequiresExistingUser(kind) {
			t.Fatalf("%s is not marked as user-targeted", kind)
		}
		form, err := newAdminForm(kind)
		if err != nil {
			t.Fatal(err)
		}
		if err := addUserChoices(form, []string{"ak", "alice"}); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		field := &form.Fields[0]
		if field.Kind != "user" || field.Value != "ak" || len(field.Options) != 2 {
			t.Fatalf("%s selector = %#v", kind, field)
		}
		adminCycleField(field)
		if field.Value != "alice" {
			t.Fatalf("%s did not cycle users: %#v", kind, field)
		}
	}
	for _, kind := range []string{"user-create", "server-notice", "server-debug"} {
		if adminFormRequiresExistingUser(kind) {
			t.Fatalf("%s unexpectedly requires an existing user", kind)
		}
	}
}

func TestUserTargetActionOpensWithFreshUsers(t *testing.T) {
	app := newTestApp()
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "open-user-form", FormKind: "network-status", Quiet: true},
		Output:    "ak (admin): 3 networks\nalice: 1 networks\n",
	})
	if app.admin.Form == nil || app.admin.Form.Kind != "network-status" {
		t.Fatalf("form was not opened: %#v", app.admin.Form)
	}
	if got := app.admin.Form.Fields[0]; got.Kind != "user" || got.Value != "ak" || len(got.Options) != 2 {
		t.Fatalf("fresh user selector = %#v", got)
	}
	app.close()
}

func TestUserTargetActionHandlesEmptyInstance(t *testing.T) {
	app := newTestApp()
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "open-user-form", FormKind: "network-status", Quiet: true},
		Output:    "No users configured.\n",
	})
	if app.admin.Form != nil || !strings.Contains(strings.Join(app.admin.Output, "\n"), "Create a user") {
		t.Fatalf("empty-user result was not explained: form=%#v output=%#v", app.admin.Form, app.admin.Output)
	}
	app.close()
}
