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
