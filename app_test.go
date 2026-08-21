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

func TestUnsupportedDeviceCertificateActionDoesNotExecute(t *testing.T) {
	app := newTestApp()
	app.admin.Capabilities = AdminCapabilities{Known: true}
	for index, item := range adminMenuItems() {
		if item.Kind == "device-cert-status" {
			app.adminActivateMenuLocked(index)
			break
		}
	}
	if app.admin.Form != nil || app.admin.Busy {
		t.Fatal("unsupported action must not open a form or execute sojuctl")
	}
	if len(app.admin.Output) == 0 || !strings.HasPrefix(app.admin.Output[len(app.admin.Output)-1], "UNAVAILABLE:") {
		t.Fatalf("missing friendly unavailable message: %#v", app.admin.Output)
	}
	app.close()
}

func TestParseAdminCapabilities(t *testing.T) {
	old := parseAdminCapabilities("network status\nserver status\n")
	if !old.Known || old.DeviceCertificates {
		t.Fatalf("unexpected old-server capabilities: %#v", old)
	}
	current := parseAdminCapabilities("device-certificate status\ndevice-certificate create\n")
	if !current.Known || !current.DeviceCertificates {
		t.Fatalf("unexpected current-server capabilities: %#v", current)
	}
}
