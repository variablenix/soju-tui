package main

import (
	"context"
	"encoding/json"
	"errors"
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

func TestAdminNavigationAliasMapping(t *testing.T) {
	tests := map[string]string{
		"k": "up", "K": "up", "w": "up", "W": "up",
		"j": "down", "J": "down", "s": "down", "S": "down",
		"h": "left", "H": "left", "a": "left", "A": "left",
		"l": "right", "L": "right", "d": "right", "D": "right",
		"enter": "enter", "pageup": "pageup",
	}
	for input, want := range tests {
		if got := adminNavigationAlias(input); got != want {
			t.Errorf("adminNavigationAlias(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestMenuAndHelpSupportAlternateNavigation(t *testing.T) {
	app := newTestApp()
	defer app.close()
	items := adminMenuItems()

	for _, key := range []string{"k", "K", "w", "W"} {
		app.admin.Cursor = 1
		app.adminHandleKey(key, []rune(key)[0])
		if app.admin.Cursor != 0 {
			t.Fatalf("%q moved cursor to %d, want 0", key, app.admin.Cursor)
		}
	}
	for _, key := range []string{"j", "J", "s", "S"} {
		app.admin.Cursor = 1
		app.adminHandleKey(key, []rune(key)[0])
		if app.admin.Cursor != 2 {
			t.Fatalf("%q moved cursor to %d, want 2", key, app.admin.Cursor)
		}
	}

	helpIndex := -1
	for index, item := range items {
		if item.Kind == "tui-help" {
			helpIndex = index
			break
		}
	}
	if helpIndex < 0 {
		t.Fatal("Soju-TUI help item is missing")
	}
	for _, pair := range [][2]string{{"l", "h"}, {"L", "H"}, {"d", "a"}, {"D", "A"}} {
		app.admin.Cursor = helpIndex
		app.adminHandleKey(pair[0], []rune(pair[0])[0])
		if !app.admin.HelpOpen {
			t.Fatalf("%q did not open the selected help action", pair[0])
		}
		app.adminHandleKey(pair[1], []rune(pair[1])[0])
		if app.admin.HelpOpen {
			t.Fatalf("%q did not close help", pair[1])
		}
	}

	app.admin.Cursor = helpIndex
	handleAdminKeyEvent(app, tcell.NewEventKey(tcell.KeyRight, 0, tcell.ModNone))
	if !app.admin.HelpOpen {
		t.Fatal("Right arrow did not open the selected help action")
	}
	handleAdminKeyEvent(app, tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModNone))
	if app.admin.HelpOpen {
		t.Fatal("Left arrow did not close help")
	}

	app.adminHandleKey("?", '?')
	for _, test := range []struct {
		key  string
		want int
	}{{"j", 1}, {"k", 0}, {"s", 1}, {"w", 0}} {
		app.adminHandleKey(test.key, []rune(test.key)[0])
		if app.admin.HelpScroll != test.want {
			t.Fatalf("help key %q set scroll to %d, want %d", test.key, app.admin.HelpScroll, test.want)
		}
	}
}

func TestAlternateNavigationLettersRemainLiteralInput(t *testing.T) {
	app := newTestApp()
	defer app.close()
	const input = "hjklwasdHJKLWASD"
	app.admin.Form = &AdminForm{Fields: []AdminField{{Kind: "text"}}}
	for _, char := range input {
		app.adminHandleKey(string(char), char)
	}
	if got := app.admin.Form.Fields[0].Value; got != input {
		t.Fatalf("form value = %q, want %q", got, input)
	}

	app.admin.Form = nil
	app.admin.Confirm = &AdminConfirmation{Operation: AdminOperation{ConfirmPhrase: input}}
	for _, char := range input {
		app.adminHandleKey(string(char), char)
	}
	if got := string(app.admin.Confirm.Input); got != input {
		t.Fatalf("confirmation input = %q, want %q", got, input)
	}
}

func TestMainOutputPagingDoesNotChangeMenuSelection(t *testing.T) {
	app := newTestApp()
	defer app.close()
	app.admin.View = adminOutput
	app.admin.Output = []string{"line 1", "line 2", "line 3"}
	app.admin.Cursor = 4
	app.adminHandleKey("pageup", 0)
	if app.admin.OutputScroll != adminOutputPageSize || app.admin.Cursor != 4 {
		t.Fatalf("Page Up state = scroll %d cursor %d", app.admin.OutputScroll, app.admin.Cursor)
	}
	app.adminHandleKey("pagedown", 0)
	if app.admin.OutputScroll != 0 || app.admin.Cursor != 4 {
		t.Fatalf("Page Down state = scroll %d cursor %d", app.admin.OutputScroll, app.admin.Cursor)
	}
	app.adminHandleKey("pageup", 0)
	app.adminHandleKey("end", 0)
	if app.admin.OutputScroll != 0 || app.admin.Cursor != len(adminMenuItems())-1 {
		t.Fatalf("End state = scroll %d cursor %d", app.admin.OutputScroll, app.admin.Cursor)
	}
}

func TestStaticHelpIsOfflineAndAlwaysAvailable(t *testing.T) {
	app := newTestApp()
	defer app.close()
	app.admin.Capabilities = AdminCapabilities{Known: true, Commands: map[string]bool{}}

	foundLocalHelp := false
	foundServerHelp := false
	for _, item := range app.adminMenuItemsLocked() {
		foundLocalHelp = foundLocalHelp || item.Kind == "tui-help"
		foundServerHelp = foundServerHelp || item.Kind == "help"
	}
	if !foundLocalHelp {
		t.Fatal("static Soju-TUI help was filtered by server capabilities")
	}
	if foundServerHelp {
		t.Fatal("unsupported server-provided BouncerServ help remained visible")
	}

	app.adminHandleKey("?", '?')
	if !app.admin.HelpOpen || app.admin.Busy {
		t.Fatal("static help unexpectedly started a backend operation")
	}
	help := strings.Join(sojuTUIHelp(), "\n")
	for _, expected := range []string{
		"SOJU-TUI HELP & DOCUMENTATION",
		"https://soju.im/",
		"https://soju.im/doc/soju.1.html",
		"https://soju.im/doc/sojuctl.1.html",
		"https://codeberg.org/emersion/soju",
		"does not open them or fetch remote content",
	} {
		if !strings.Contains(help, expected) {
			t.Fatalf("static help is missing %q:\n%s", expected, help)
		}
	}
	if strings.ContainsAny(help, "\x00\x1b\r") {
		t.Fatal("static help contains terminal control characters")
	}

	app.adminHandleKey("down", 0)
	if app.admin.HelpScroll != 1 {
		t.Fatalf("help scroll = %d, want 1", app.admin.HelpScroll)
	}
	app.adminHandleKey("end", 0)
	wantLastPage := adminHelpMaxScroll(adminDefaultHelpWidth, adminDefaultHelpHeight)
	if app.admin.HelpScroll != wantLastPage {
		t.Fatalf("help End scroll = %d, want %d", app.admin.HelpScroll, wantLastPage)
	}
	app.adminHandleKey("down", 0)
	app.adminHandleKey("pagedown", 0)
	if app.admin.HelpScroll != wantLastPage {
		t.Fatalf("help scrolled past final page: got %d, want %d", app.admin.HelpScroll, wantLastPage)
	}
	app.adminHandleKey("home", 0)
	app.adminHandleKey("?", '?')
	if app.admin.HelpOpen {
		t.Fatal("question mark did not close static help")
	}

	handleAdminKeyEvent(app, tcell.NewEventKey(tcell.KeyF1, 0, tcell.ModNone))
	if !app.admin.HelpOpen || app.admin.HelpScroll != 0 {
		t.Fatal("F1 did not open static help")
	}
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

func TestCertificateGenerationIsOmittedWithoutSafePreflightCommand(t *testing.T) {
	app := newTestApp()
	app.admin.Capabilities = AdminCapabilities{Known: true, Commands: map[string]bool{
		"certfp generate": true,
	}}
	for _, item := range app.adminMenuItemsLocked() {
		if item.Kind == "cert-generate" {
			t.Fatalf("certificate generation remained visible without certfp fingerprint: %#v", item)
		}
	}
	app.admin.Capabilities.Commands["certfp fingerprint"] = true
	found := false
	for _, item := range app.adminMenuItemsLocked() {
		found = found || item.Kind == "cert-generate"
	}
	if !found {
		t.Fatal("certificate generation stayed hidden after safe preflight became available")
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
	output := "ak (admin): 3 networks\nalice: 1 networks\nteam lead:ops (disabled): 2 networks (5 max)\nak (admin): 3 networks\n(2 more users omitted)\n"
	users := parseSojuUsernames(output)
	if strings.Join(users, ",") != "ak,alice,team lead:ops" {
		t.Fatalf("users = %#v", users)
	}
}

func TestUserTargetFormsReceiveDiscoverableAndCustomSelector(t *testing.T) {
	userTargetedKinds := []string{
		"user-status-specific", "user-password-change", "user-password-reset", "user-update", "user-identity-update", "user-delete",
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

func TestChangePasswordPrefersMatchingLocalUsername(t *testing.T) {
	app := newTestApp()
	defer app.close()
	app.localUsername = "alice"
	if err := app.adminOpenFormWithUsersLocked("user-password-change", []string{"ak", "alice", "operator"}); err != nil {
		t.Fatal(err)
	}
	field := app.admin.Form.Fields[0]
	if field.Value != "alice" || strings.Join(field.Options, ",") != "alice,ak,operator" {
		t.Fatalf("preferred account selector = %#v", field)
	}
}

func TestPreferExactUserDoesNotMutateDiscoveryOrder(t *testing.T) {
	users := []string{"ak", "alice", "operator"}
	ordered := preferExactUser(users, "alice")
	if strings.Join(ordered, ",") != "alice,ak,operator" {
		t.Fatalf("preferred users = %#v", ordered)
	}
	if strings.Join(users, ",") != "ak,alice,operator" {
		t.Fatalf("discovered users were mutated: %#v", users)
	}
	if got := preferExactUser(users, "missing"); strings.Join(got, ",") != "ak,alice,operator" {
		t.Fatalf("missing preference changed order: %#v", got)
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

func TestUserDeletionFailsClosedOnUnexpectedConfirmation(t *testing.T) {
	app := newTestApp()
	defer app.close()
	app.processResult(adminResult{
		Operation: AdminOperation{NeedsSojuConfirmation: true, TargetUser: "alice"},
		Output:    "success without a confirmation token\n",
	})
	if app.admin.Confirm != nil {
		t.Fatalf("unexpected confirmation was accepted: %#v", app.admin.Confirm)
	}
	output := strings.Join(app.admin.Output, "\n")
	if !strings.Contains(output, "Deletion was blocked") || !strings.Contains(app.currentStatusLocked(), "blocked") {
		t.Fatalf("fail-closed result was not explained: output=%q status=%q", output, app.currentStatusLocked())
	}
}

func TestCompletedSecretOperationIsNotRetained(t *testing.T) {
	app := newTestApp()
	defer app.close()
	app.processResult(adminResult{
		Operation: AdminOperation{Args: []string{"user", "update", "alice", "-password", "correct-horse"}, Secrets: []string{"correct-horse"}},
		Output:    "updated user correct-horse\n",
	})
	state, err := json.Marshal(app.admin)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), "correct-horse") {
		t.Fatalf("completed secret remained in admin state: %s", state)
	}
	if !strings.Contains(string(state), "••••••") {
		t.Fatalf("captured output was not redacted: %s", state)
	}
}

func TestUnixSchemeFallbackRetriesOnlyEquivalentAddress(t *testing.T) {
	app := newTestApp()
	defer app.close()
	app.backend = &SojuCtl{Path: "/bin/true", Config: "/etc/soju/config"}
	op := AdminOperation{
		Args:                  []string{"user", "run", "alice", "network", "create", "-addr", "unix:///run/irc.sock"},
		CompatibilityFallback: []string{"user", "run", "alice", "network", "create", "-addr", "irc+unix:///run/irc.sock"},
		FallbackPreview:       "sojuctl -config /etc/soju/config user run alice network create -addr irc+unix:///run/irc.sock",
	}
	app.processResult(adminResult{
		Operation: op,
		Output:    `unknown scheme "unix" (supported schemes: ircs, irc+insecure, irc+unix)`,
		Err:       errors.New("sojuctl failed: exit status 1"),
	})
	if !app.admin.Busy || !strings.Contains(strings.Join(app.admin.Output, "\n"), "retrying the equivalent") {
		t.Fatalf("compatibility fallback was not scheduled: busy=%v output=%#v", app.admin.Busy, app.admin.Output)
	}
}

func TestNetworkTargetedActionDiscoversNetworksBeforeOpening(t *testing.T) {
	app := newTestApp()
	defer app.close()
	if err := app.adminOpenFormWithUsersLocked("cert-fingerprint", []string{"alice", "bob"}); err != nil {
		t.Fatal(err)
	}
	if app.admin.Form == nil || app.admin.Form.Kind != "network-discovery" || app.admin.Form.TargetKind != "cert-fingerprint" {
		t.Fatalf("discovery form = %#v", app.admin.Form)
	}
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "open-network-form", FormKind: "cert-fingerprint", TargetUser: "alice", Quiet: true},
		Output:    "libera (ircs://irc.libera.chat:6697) [connected]: 4 channels\nouch (ircs://irc.ouch.chat:6697) [connected]: 2 channels\n",
	})
	if app.admin.Form == nil || app.admin.Form.Kind != "cert-fingerprint" {
		t.Fatalf("target form = %#v", app.admin.Form)
	}
	network := app.admin.Form.Fields[1]
	if network.Kind != "network" || network.Value != allNetworksSelection || len(network.Options) != 3 || network.Options[1] != "libera" {
		t.Fatalf("network selector = %#v", network)
	}
}

func TestChannelUpdateDiscoveryAndPrefill(t *testing.T) {
	app := newTestApp()
	defer app.close()
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "open-network-form", FormKind: "channel-update", TargetUser: "alice", Quiet: true},
		Output:    "libera (ircs://irc.libera.chat:6697) [connected]: 2 channels\n",
	})
	if app.admin.Form == nil || app.admin.Form.Kind != "channel-discovery" || app.admin.Form.Fields[1].Value != "libera" {
		t.Fatalf("network discovery result = %#v", app.admin.Form)
	}
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "open-channel-form", FormKind: "channel-update", TargetUser: "alice", TargetNetwork: "libera", Quiet: true},
		Output:    "#chat [joined]\n#ops [parted, detached]\n",
	})
	if app.admin.Form == nil || app.admin.Form.Kind != "channel-update-lookup" || app.admin.Form.Fields[2].Kind != "channel" || len(app.admin.Form.Fields[2].Options) != 2 {
		t.Fatalf("channel selector = %#v", app.admin.Form)
	}
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "channel-update", TargetUser: "alice", TargetNetwork: "libera", TargetChannel: "#ops", Quiet: true},
		Output:    "#chat [joined]\n#ops [parted, detached]\n",
	})
	if app.admin.Form == nil || app.admin.Form.Kind != "channel-update" {
		t.Fatalf("update form = %#v", app.admin.Form)
	}
	if got := app.admin.Form.Fields[3]; got.Label != "Detached" || got.Value != "true" || got.Original != "true" {
		t.Fatalf("prefilled detached field = %#v", got)
	}
	if got := app.admin.Form.Fields[4]; got.Value != "" || !strings.Contains(got.Help, "blank keeps current") {
		t.Fatalf("undisclosed field = %#v", got)
	}
}

func TestCertFingerprintBatchFormatsConfiguredAndMissingNetworks(t *testing.T) {
	app := newTestApp()
	defer app.close()
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "cert-fingerprint-batch", TargetNetwork: "libera", Quiet: true},
		Output:    "SHA-256 fingerprint: AA:BB\n",
	})
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "cert-fingerprint-batch", TargetNetwork: "ouch", Quiet: true},
		Output:    "CertFP not set up\n",
		Err:       errors.New("sojuctl failed: exit status 1"),
	})
	output := strings.Join(app.admin.Output, "\n")
	for _, want := range []string{"CERTFP NETWORK: libera", "SHA-256 fingerprint: AA:BB", "CERTFP NETWORK: ouch", "Not configured"} {
		if !strings.Contains(output, want) {
			t.Fatalf("batch output missing %q: %s", want, output)
		}
	}
	if app.admin.BatchFailures != 0 {
		t.Fatalf("unconfigured CertFP counted as a batch failure: %d", app.admin.BatchFailures)
	}
}

func TestCertificatePreflightRequiresReplacementPhraseWhenExisting(t *testing.T) {
	app := newTestApp()
	planned := AdminOperation{Summary: "Generate upstream CertFP", Mutating: true}
	app.admin.PendingOperation = &planned
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "cert-generate-preflight", Quiet: true},
		Output:    "SHA-256 fingerprint: AA:BB:CC\n",
	})
	if app.admin.Confirm == nil || app.admin.Confirm.Operation.ConfirmPhrase != "REPLACE EXISTING UPSTREAM CERTIFICATE" {
		t.Fatalf("existing certificate confirmation = %#v", app.admin.Confirm)
	}
	if app.admin.Confirm.Operation.CertificateState != "existing" || app.admin.Confirm.Operation.CertificateReport == "" {
		t.Fatalf("existing certificate guard state = %#v", app.admin.Confirm.Operation)
	}
	if app.admin.Confirm.Operation.ConfirmationImpact != adminConfirmationDestructive {
		t.Fatalf("existing certificate impact = %v, want destructive", app.admin.Confirm.Operation.ConfirmationImpact)
	}
	output := strings.Join(app.admin.Output, "\n")
	if !strings.Contains(output, "EXISTING UPSTREAM SASL CERTIFICATE FOUND") || !strings.Contains(output, "AA:BB:CC") || !strings.Contains(output, "Let's Encrypt files are not touched") {
		t.Fatalf("existing certificate warning = %q", output)
	}
	app.close()
}

func TestCertificatePreflightRequiresGenerationPhraseWhenAbsent(t *testing.T) {
	app := newTestApp()
	planned := AdminOperation{Summary: "Generate upstream CertFP", Mutating: true}
	app.admin.PendingOperation = &planned
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "cert-generate-preflight", Quiet: true},
		Output:    "CertFP not set up\n",
		Err:       errors.New("sojuctl failed: exit status 1"),
	})
	if app.admin.Confirm == nil || app.admin.Confirm.Operation.ConfirmPhrase != "GENERATE UPSTREAM CERTIFICATE" {
		t.Fatalf("new certificate confirmation = %#v", app.admin.Confirm)
	}
	if app.admin.Confirm.Operation.CertificateState != "absent" {
		t.Fatalf("absent certificate guard state = %#v", app.admin.Confirm.Operation)
	}
	if app.admin.Confirm.Operation.ConfirmationImpact != adminConfirmationAddition {
		t.Fatalf("new certificate impact = %v, want addition", app.admin.Confirm.Operation.ConfirmationImpact)
	}
	if !strings.Contains(strings.Join(app.admin.Output, "\n"), "No existing upstream SASL CertFP") {
		t.Fatalf("missing no-certificate explanation: %#v", app.admin.Output)
	}
	app.close()
}

func TestCertificatePreflightFailsClosed(t *testing.T) {
	app := newTestApp()
	planned := AdminOperation{Summary: "Generate upstream CertFP", Mutating: true}
	app.admin.PendingOperation = &planned
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "cert-generate-preflight", Quiet: true},
		Output:    "permission denied\n",
		Err:       errors.New("sojuctl failed: exit status 1"),
	})
	if app.admin.Confirm != nil || app.admin.PendingOperation != nil {
		t.Fatalf("failed preflight allowed confirmation: confirm=%#v pending=%#v", app.admin.Confirm, app.admin.PendingOperation)
	}
	if !strings.Contains(strings.Join(app.admin.Output, "\n"), "blocked") && !strings.Contains(app.currentStatusLocked(), "blocked") {
		t.Fatalf("failed preflight was not explained: output=%#v status=%q", app.admin.Output, app.currentStatusLocked())
	}
	app.close()
}

func TestCertificatePreflightRejectsMalformedSuccess(t *testing.T) {
	app := newTestApp()
	planned := AdminOperation{Summary: "Generate upstream CertFP", Mutating: true}
	app.admin.PendingOperation = &planned
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "cert-generate-preflight", Quiet: true},
		Output:    "certificate exists maybe\n",
	})
	if app.admin.Confirm != nil || app.admin.PendingOperation != nil {
		t.Fatalf("malformed successful response allowed generation: confirm=%#v pending=%#v", app.admin.Confirm, app.admin.PendingOperation)
	}
	if !strings.Contains(strings.Join(app.admin.Output, "\n"), "unexpected fingerprint response") {
		t.Fatalf("malformed response was not explained: %#v", app.admin.Output)
	}
	app.close()
}

func TestCertificateStateGuardMatchesOnlyReviewedState(t *testing.T) {
	existing := AdminOperation{CertificateState: "existing", CertificateReport: "SHA-256 fingerprint: AA\nSHA-512 fingerprint: BB"}
	if !certificateStateMatches(existing, " SHA-256 fingerprint: AA \n SHA-512 fingerprint: BB\n", nil) {
		t.Fatal("unchanged existing certificate did not match")
	}
	if certificateStateMatches(existing, "SHA-256 fingerprint: CC\nSHA-512 fingerprint: DD", nil) {
		t.Fatal("changed existing certificate matched")
	}
	absent := AdminOperation{CertificateState: "absent"}
	if !certificateStateMatches(absent, "CertFP not set up\n", errors.New("exit status 1")) {
		t.Fatal("unchanged absent state did not match")
	}
	if certificateStateMatches(absent, "permission denied\n", errors.New("exit status 1")) {
		t.Fatal("unexpected error matched absent state")
	}
}

func TestCertificateGuardBlocksStateChange(t *testing.T) {
	app := newTestApp()
	planned := AdminOperation{CertificateState: "existing", CertificateReport: "SHA-256 fingerprint: AA"}
	app.admin.PendingOperation = &planned
	app.processResult(adminResult{
		Operation: AdminOperation{FollowUpKind: "cert-generate-guard", Quiet: true},
		Output:    "SHA-256 fingerprint: BB\n",
	})
	if app.admin.PendingOperation != nil || app.admin.Busy {
		t.Fatalf("changed state was not stopped: pending=%#v busy=%v", app.admin.PendingOperation, app.admin.Busy)
	}
	if !strings.Contains(strings.Join(app.admin.Output, "\n"), "state changed") {
		t.Fatalf("state change was not explained: %#v", app.admin.Output)
	}
	app.close()
}
