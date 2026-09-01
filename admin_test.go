package main

import (
	"fmt"
	"strings"
	"testing"
)

func setAdminField(t *testing.T, form *AdminForm, label, value string) {
	t.Helper()
	for index := range form.Fields {
		if form.Fields[index].Label == label {
			form.Fields[index].Value = value
			return
		}
	}
	t.Fatalf("field %q not found in %s form", label, form.Kind)
}

func TestAdminUserCreateUsesArgvAndRedactsPassword(t *testing.T) {
	form, err := newAdminForm("user-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "p@ss word"
	form.Fields[2].Value = "false"
	form.Fields[5].Value = "true"
	form.Fields[6].Value = "-1"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if !op.Mutating || len(op.Args) == 0 {
		t.Fatalf("unexpected operation: %#v", op)
	}
	if op.ConfirmationImpact != adminConfirmationAddition {
		t.Fatalf("create-user confirmation impact = %v, want addition", op.ConfirmationImpact)
	}
	if op.CapabilityUser != "alice" {
		t.Fatalf("created user was not retained for capability refresh: %#v", op)
	}
	if strings.Contains(op.Preview, "p@ss word") || !strings.Contains(op.Preview, "••••••") {
		t.Fatalf("secret leaked in preview: %q", op.Preview)
	}
	if strings.Contains(op.Preview, "sh -c") {
		t.Fatal("operation preview suggests shell execution")
	}
	if got := strings.Join(op.Args, " "); !strings.Contains(got, "p@ss word") {
		t.Fatalf("password was not preserved as one argv value: %q", got)
	}
}

func TestPrivilegeAndBroadcastChangesRequireTypedConfirmation(t *testing.T) {
	adminUser, err := newAdminForm("user-create")
	if err != nil {
		t.Fatal(err)
	}
	adminUser.Fields[0].Value = "operator"
	adminUser.Fields[1].Value = "secret"
	adminUser.Fields[2].Value = "true"
	op, err := buildAdminOperation("/etc/soju/config", adminUser)
	if err != nil {
		t.Fatal(err)
	}
	if op.ConfirmPhrase != "CREATE ADMIN USER" {
		t.Fatalf("admin creation confirmation = %q", op.ConfirmPhrase)
	}
	if !containsArg(op.Args, "-admin=true") || containsArg(op.Args, "-admin") {
		t.Fatalf("standard Go boolean flag was not encoded safely: %#v", op.Args)
	}

	notice, err := newAdminForm("server-notice")
	if err != nil {
		t.Fatal(err)
	}
	notice.Fields[0].Value = "maintenance soon"
	op, err = buildAdminOperation("/etc/soju/config", notice)
	if err != nil {
		t.Fatal(err)
	}
	if op.ConfirmPhrase != "BROADCAST SERVER NOTICE" {
		t.Fatalf("broadcast confirmation = %q", op.ConfirmPhrase)
	}
}

func TestAdminUserCreateOmitsUnchangedDefaults(t *testing.T) {
	form, err := newAdminForm("user-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "secret"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	for _, optional := range []string{"-admin", "-enabled", "-max-networks"} {
		if containsArg(op.Args, optional) {
			t.Fatalf("unchanged default %s was sent: %#v", optional, op.Args)
		}
	}
}

func TestAdminUserCreateDisablePasswordUsesPresenceFlag(t *testing.T) {
	form, err := newAdminForm("user-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "service"
	form.Fields[7].Value = "true"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(op.Args, "\x00"), "-disable-password\x00true") {
		t.Fatalf("presence-only flag received a value: %#v", op.Args)
	}
	if !containsArg(op.Args, "-disable-password") {
		t.Fatalf("missing disable-password flag: %#v", op.Args)
	}
	if containsArg(op.Args, "-password") {
		t.Fatalf("password flag should be omitted when disabled: %#v", op.Args)
	}
}

func TestAdminRejectsPasswordWithDisablePassword(t *testing.T) {
	form, err := newAdminForm("user-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "service"
	form.Fields[1].Value = "secret"
	form.Fields[7].Value = "true"
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil {
		t.Fatal("expected password/disable-password conflict")
	}
}

func TestChangePasswordRequiresMatchingConfirmationAndRedactsSecret(t *testing.T) {
	form, err := newAdminForm("user-password-change")
	if err != nil {
		t.Fatal(err)
	}
	setAdminField(t, form, "Username", "alice")
	setAdminField(t, form, "New password", "correct horse battery staple")
	setAdminField(t, form, "Confirm password", "different password")
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("expected mismatched-password rejection, got %v", err)
	}

	setAdminField(t, form, "Confirm password", "correct horse battery staple")
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	wantArgs := []string{"user", "update", "alice", "-password", "correct horse battery staple"}
	if strings.Join(op.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("password change argv = %#v, want %#v", op.Args, wantArgs)
	}
	if op.ConfirmationImpact != adminConfirmationChange || op.ConfirmPhrase != "" {
		t.Fatalf("password change confirmation = impact %v phrase %q", op.ConfirmationImpact, op.ConfirmPhrase)
	}
	if strings.Contains(op.Preview, "correct horse battery staple") || !strings.Contains(op.Preview, "••••••") {
		t.Fatalf("password leaked in preview: %q", op.Preview)
	}
}

func TestResetUserPasswordRequiresTypedConfirmationAndRedactsSecret(t *testing.T) {
	form, err := newAdminForm("user-password-reset")
	if err != nil {
		t.Fatal(err)
	}
	setAdminField(t, form, "Username", "alice")
	setAdminField(t, form, "New password", "replacement secret")
	setAdminField(t, form, "Confirm password", "replacement secret")
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if op.ConfirmationImpact != adminConfirmationDestructive {
		t.Fatalf("password reset impact = %v, want destructive", op.ConfirmationImpact)
	}
	if op.ConfirmPhrase != "RESET USER PASSWORD alice" {
		t.Fatalf("password reset phrase = %q", op.ConfirmPhrase)
	}
	if strings.Contains(op.Preview, "replacement secret") || !strings.Contains(op.Preview, "••••••") {
		t.Fatalf("replacement password leaked in preview: %q", op.Preview)
	}
}

func TestCancelledPasswordChangeDoesNotRetainSecret(t *testing.T) {
	app := newTestApp()
	form, err := newAdminForm("user-password-change")
	if err != nil {
		t.Fatal(err)
	}
	setAdminField(t, form, "Username", "alice")
	setAdminField(t, form, "New password", "correct horse battery staple")
	setAdminField(t, form, "Confirm password", "correct horse battery staple")
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	app.admin.Confirm = &AdminConfirmation{Operation: op}
	app.adminHandleKey("esc", 0)
	if app.admin.Confirm != nil {
		t.Fatal("cancelled confirmation was retained")
	}
	state := fmt.Sprintf("%#v", app.admin)
	if strings.Contains(state, "correct horse battery staple") {
		t.Fatal("cancelled password remained in application state")
	}
	app.close()
}

func TestGenericUserUpdateRequiresPasswordConfirmation(t *testing.T) {
	form, err := newAdminForm("user-update")
	if err != nil {
		t.Fatal(err)
	}
	setAdminField(t, form, "Username", "alice")
	setAdminField(t, form, "New password", "new secret")
	setAdminField(t, form, "Confirm new password", "wrong secret")
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("expected mismatched-password rejection, got %v", err)
	}

	setAdminField(t, form, "Confirm new password", "new secret")
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(op.Args, "-password") || strings.Contains(op.Preview, "new secret") {
		t.Fatalf("generic password update was not safely built: %#v preview=%q", op.Args, op.Preview)
	}
}

func TestAdminNetworkIgnoreLimitUsesPresenceFlag(t *testing.T) {
	form, err := newAdminForm("network-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "ircs://irc.example.test:6697"
	form.Fields[10].Value = "true"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(op.Args, "\x00"), "-ignore-limit\x00true") {
		t.Fatalf("presence-only flag received a value: %#v", op.Args)
	}
	if !containsArg(op.Args, "-ignore-limit") {
		t.Fatalf("missing ignore-limit flag: %#v", op.Args)
	}
}

func TestAdminNetworkQuotePreservesRawCommand(t *testing.T) {
	form, err := newAdminForm("network-quote")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "libera"
	form.Fields[2].Value = "PRIVMSG NickServ :IDENTIFY a password"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if len(op.Args) == 0 || op.Args[len(op.Args)-1] != "PRIVMSG NickServ :IDENTIFY a password" {
		t.Fatalf("raw command was not kept as one argv value: %#v", op.Args)
	}
	if strings.Contains(op.Preview, "IDENTIFY a password") || !strings.Contains(op.Preview, "••••••") {
		t.Fatalf("potentially secret raw command leaked in preview: %q", op.Preview)
	}
}

func TestAdminRejectsIRCLineInjection(t *testing.T) {
	form, err := newAdminForm("server-notice")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "hello\r\nserver debug true"
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil {
		t.Fatal("expected control-character rejection")
	}
}

func TestAdminNetworkCreateDoesNotRequireNetworkName(t *testing.T) {
	form, err := newAdminForm("network-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "ircs://irc.example.test:6697"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if len(op.Args) < 6 || op.Args[0] != "user" || op.Args[1] != "run" || op.Args[3] != "network" || op.Args[4] != "create" {
		t.Fatalf("unexpected network create argv: %#v", op.Args)
	}
}

func TestAdminChannelCreateTargetsNetworkInName(t *testing.T) {
	form, err := newAdminForm("channel-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "libera"
	form.Fields[2].Value = "#chat"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if len(op.Args) < 6 || op.Args[3] != "channel" || op.Args[4] != "create" || op.Args[5] != "#chat/libera" {
		t.Fatalf("channel network selector was not encoded in the name: %#v", op.Args)
	}
}

func TestAdminChannelUpdateRejectsNoChanges(t *testing.T) {
	form, err := newAdminForm("channel-update")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "libera"
	form.Fields[2].Value = "#chat"
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil || !strings.Contains(err.Error(), "no channel settings changed") {
		t.Fatalf("expected no-change rejection, got %v", err)
	}
}

func TestLoadedChannelUpdateSubmitsOnlyChangedDetachedState(t *testing.T) {
	form := newChannelUpdateForm("alice", "libera", ChannelStatus{Name: "#chat", Detached: true})
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil || !strings.Contains(err.Error(), "no channel settings changed") {
		t.Fatalf("expected no-change rejection, got %v", err)
	}
	form.Fields[3].Value = "false"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(op.Args, "\x00")
	if !strings.Contains(joined, "channel\x00update\x00#chat/libera\x00-detached\x00false") {
		t.Fatalf("unexpected channel update args: %#v", op.Args)
	}
	if strings.Contains(joined, "-relay-detached") || strings.Contains(joined, "-reattach-on") || strings.Contains(joined, "-detach-after") || strings.Contains(joined, "-detach-on") {
		t.Fatalf("undisclosed settings were submitted: %#v", op.Args)
	}
}

func TestCertFingerprintBatchCoversEveryDiscoveredNetwork(t *testing.T) {
	ops := certFingerprintBatchOperations("/etc/soju/config", "alice", []string{allNetworksSelection, "libera", "ouch"})
	if len(ops) != 2 {
		t.Fatalf("operations = %#v", ops)
	}
	for index, want := range []string{"libera", "ouch"} {
		if ops[index].TargetNetwork != want || !ops[index].Quiet || ops[index].FollowUpKind != "cert-fingerprint-batch" {
			t.Fatalf("operation %d = %#v", index, ops[index])
		}
		if got := strings.Join(ops[index].Args, " "); got != "user run alice certfp fingerprint -network "+want {
			t.Fatalf("operation %d args = %q", index, got)
		}
	}
}

func TestSASLStatusBatchCoversEveryDiscoveredNetwork(t *testing.T) {
	ops := saslStatusBatchOperations("/etc/soju/config", "alice", []string{allNetworksSelection, "libera", "ouch"})
	if len(ops) != 2 {
		t.Fatalf("operations = %#v", ops)
	}
	for index, want := range []string{"libera", "ouch"} {
		if ops[index].TargetNetwork != want || !ops[index].Quiet || ops[index].FollowUpKind != "sasl-status-batch" {
			t.Fatalf("operation %d = %#v", index, ops[index])
		}
		if got := strings.Join(ops[index].Args, " "); got != "user run alice sasl status -network "+want {
			t.Fatalf("operation %d args = %q", index, got)
		}
	}
}

func TestParseUserDeleteConfirmation(t *testing.T) {
	args, username, ok := parseUserDeleteConfirmation(`To confirm user deletion, send "user delete alice 0123ab"`, "alice")
	if !ok || username != "alice" || len(args) != 4 || args[2] != "alice" || args[3] != "0123ab" {
		t.Fatalf("unexpected delete confirmation parse: args=%#v username=%q ok=%v", args, username, ok)
	}
}

func TestParseUserDeleteConfirmationSupportsSpacesAndRejectsMismatch(t *testing.T) {
	args, username, ok := parseUserDeleteConfirmation(`To confirm user deletion, send "user delete team lead:ops abc123"`, "team lead:ops")
	if !ok || username != "team lead:ops" || strings.Join(args, "|") != "user|delete|team lead:ops|abc123" {
		t.Fatalf("unexpected delete confirmation parse: args=%#v username=%q ok=%v", args, username, ok)
	}
	if _, _, ok := parseUserDeleteConfirmation(`To confirm user deletion, send "user delete mallory abc123"`, "alice"); ok {
		t.Fatal("accepted a confirmation for a different username")
	}
}

func TestUnixSchemeCompatibilityFallback(t *testing.T) {
	args := []string{"user", "run", "alice", "network", "create", "-addr", "unix:///run/irc.sock", "-name", "local"}
	fallback := unixSchemeFallbackArgs(args)
	if got := strings.Join(fallback, " "); got != "user run alice network create -addr irc+unix:///run/irc.sock -name local" {
		t.Fatalf("fallback = %q", got)
	}
	if !isUnixSchemeCompatibilityError(`unknown scheme "unix" (supported schemes: ircs, irc+insecure, irc+unix)`) {
		t.Fatal("did not recognize the explicit forward-compatibility error")
	}
	if isUnixSchemeCompatibilityError(`dial unix /run/irc.sock: permission denied`) {
		t.Fatal("treated an unrelated Unix socket error as a compatibility signal")
	}
}

func TestCreateUserRejectsAmbiguousDiscoveryNames(t *testing.T) {
	for _, username := range []string{" alice", "alice ", "alice (admin)", "alice (disabled)"} {
		form, err := newAdminForm("user-create")
		if err != nil {
			t.Fatal(err)
		}
		form.Fields[0].Value = username
		form.Fields[1].Value = "password"
		if _, err := buildAdminOperation("/etc/soju/config", form); err == nil {
			t.Fatalf("ambiguous username %q was accepted", username)
		}
	}
	form, err := newAdminForm("user-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "team lead:ops"
	form.Fields[1].Value = "password"
	if _, err := buildAdminOperation("/etc/soju/config", form); err != nil {
		t.Fatalf("discoverable username with spaces and colon was rejected: %v", err)
	}
}

func TestResetSASLRequiresTypedConfirmation(t *testing.T) {
	form, err := newAdminForm("sasl-reset")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "libera"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if op.ConfirmPhrase != "RESET SASL" {
		t.Fatalf("confirmation phrase = %q", op.ConfirmPhrase)
	}
	if op.ConfirmationImpact != adminConfirmationDestructive {
		t.Fatalf("reset-SASL confirmation impact = %v, want destructive", op.ConfirmationImpact)
	}
}

func TestSetSASLPlainRequiresTypedConfirmationAndRedactsPassword(t *testing.T) {
	form, err := newAdminForm("sasl-set-plain")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "libera"
	form.Fields[2].Value = "alice-account"
	form.Fields[3].Value = "upstream secret"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if op.ConfirmPhrase != "SET SASL PLAIN" {
		t.Fatalf("confirmation phrase = %q", op.ConfirmPhrase)
	}
	if strings.Contains(op.Preview, "upstream secret") || !strings.Contains(op.Preview, "••••••") {
		t.Fatalf("SASL password leaked in preview: %q", op.Preview)
	}
}

func TestCertificateGenerationUsesCompatibleDefaults(t *testing.T) {
	form, err := newAdminForm("cert-generate")
	if err != nil {
		t.Fatal(err)
	}
	if got := form.Fields[2].Value; got != "rsa" {
		t.Fatalf("initial key type = %q, want rsa", got)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "libera"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if containsArg(op.Args, "-key-type") || containsArg(op.Args, "-bits") {
		t.Fatalf("default key options should rely on Soju defaults: %#v", op.Args)
	}
	wantPreflight := []string{"user", "run", "alice", "certfp", "fingerprint", "-network", "libera"}
	if strings.Join(op.Preflight, "\x00") != strings.Join(wantPreflight, "\x00") {
		t.Fatalf("certificate preflight = %#v, want %#v", op.Preflight, wantPreflight)
	}
	if op.ConfirmPhrase != "GENERATE OR REPLACE UPSTREAM CERTIFICATE" {
		t.Fatalf("fallback confirmation phrase = %q", op.ConfirmPhrase)
	}
	form.Fields[2].Value = "ed25519"
	op, err = buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(op.Args, "-key-type") || containsArg(op.Args, "-bits") {
		t.Fatalf("ed25519 options are invalid: %#v", op.Args)
	}
}

func TestCertificateGenerationRejectsInvalidRSABits(t *testing.T) {
	form, err := newAdminForm("cert-generate")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "libera"
	for _, invalid := range []string{"", "zero", "0", "1024", "8193"} {
		form.Fields[3].Value = invalid
		if _, err := buildAdminOperation("/etc/soju/config", form); err == nil {
			t.Fatalf("RSA bits %q were accepted", invalid)
		}
	}
}

func TestNetworkFormUsesCorrectUnixSchemeAndServerPinTerminology(t *testing.T) {
	form, err := newAdminForm("network-create")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(form.Fields[1].Help, "irc+unix:///path") {
		t.Fatalf("network address help = %q", form.Fields[1].Help)
	}
	if form.Fields[7].Label != "Server TLS fingerprint" || !strings.Contains(form.Fields[7].Help, "not SASL CertFP") {
		t.Fatalf("server pin field = %#v", form.Fields[7])
	}
}

func TestNetworkNormalizesDocumentedUnixAddressForSojuCtl(t *testing.T) {
	form, err := newAdminForm("network-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "irc+unix:///run/irc/upstream.sock"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(op.Args, "\x00"); !strings.Contains(got, "-addr\x00unix:///run/irc/upstream.sock") {
		t.Fatalf("Unix address was not normalized: %#v", op.Args)
	}
}

func TestNetworkSupportsRepeatedAndClearedConnectCommands(t *testing.T) {
	form, err := newAdminForm("network-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "ircs://irc.example.test:6697"
	form.Fields[11].Value = "MODE alice +i"
	form.Fields[12].Value = `["PRIVMSG NickServ :IDENTIFY account secret","JOIN #ops"]`
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	var commands []string
	for index, arg := range op.Args {
		if arg == "-connect-command" && index+1 < len(op.Args) {
			commands = append(commands, op.Args[index+1])
		}
	}
	if got := strings.Join(commands, "|"); got != "MODE alice +i|PRIVMSG NickServ :IDENTIFY account secret|JOIN #ops" {
		t.Fatalf("connect commands = %q; args=%#v", got, op.Args)
	}
	if strings.Contains(op.Preview, "IDENTIFY account secret") || !strings.Contains(op.Preview, "••••••") {
		t.Fatalf("connect command leaked in preview: %q", op.Preview)
	}
	if op.ConfirmPhrase != "SET NETWORK CONNECT COMMANDS" {
		t.Fatalf("connect command confirmation = %q", op.ConfirmPhrase)
	}

	update := newNetworkUpdateForm("alice", NetworkStatus{Name: "libera", Address: "ircs://irc.example.test:6697"})
	update.Fields[len(update.Fields)-1].Value = "connect commands"
	op, err = buildAdminOperation("/etc/soju/config", update)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(op.Args, "\x00"); !strings.Contains(got, "-connect-command\x00") {
		t.Fatalf("clear connect commands missing: %#v", op.Args)
	}
	if op.ConfirmPhrase != "CLEAR NETWORK SETTING" {
		t.Fatalf("clear confirmation = %q", op.ConfirmPhrase)
	}
	if op.ConfirmationImpact != adminConfirmationDestructive {
		t.Fatalf("clear-network confirmation impact = %v, want destructive", op.ConfirmationImpact)
	}
}

func TestNetworkCanExplicitlyClearUndisclosedSettings(t *testing.T) {
	form := newNetworkUpdateForm("alice", NetworkStatus{Name: "libera", Address: "ircs://irc.example.test:6697"})
	form.Fields[len(form.Fields)-1].Value = "server TLS fingerprint"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(op.Args, "\x00"); !strings.Contains(got, "-certfp\x00") {
		t.Fatalf("clear server pin missing: %#v", op.Args)
	}
	if op.ConfirmPhrase != "CLEAR NETWORK SETTING" {
		t.Fatalf("clear confirmation = %q", op.ConfirmPhrase)
	}
	form.Fields[8].Value = strings.Repeat("AA", 32)
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil {
		t.Fatal("clear and replace of the same setting was accepted")
	}
}

func TestNetworkTLSPinRequiresTypedConfirmation(t *testing.T) {
	form, err := newAdminForm("network-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "ircs://irc.example.test:6697"
	form.Fields[7].Value = strings.Repeat("AA", 32)
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if op.ConfirmPhrase != "CHANGE SERVER TLS PIN" {
		t.Fatalf("server TLS pin confirmation = %q", op.ConfirmPhrase)
	}
}

func TestAdminRejectsInvalidLimitsFingerprintsAndDurations(t *testing.T) {
	user, err := newAdminForm("user-create")
	if err != nil {
		t.Fatal(err)
	}
	user.Fields[0].Value = "alice"
	user.Fields[1].Value = "secret"
	user.Fields[6].Value = "-2"
	if _, err := buildAdminOperation("/etc/soju/config", user); err == nil {
		t.Fatal("invalid max-networks was accepted")
	}

	network, err := newAdminForm("network-create")
	if err != nil {
		t.Fatal(err)
	}
	network.Fields[0].Value = "alice"
	network.Fields[1].Value = "ircs://irc.example.test:6697"
	network.Fields[7].Value = "not-a-fingerprint"
	if _, err := buildAdminOperation("/etc/soju/config", network); err == nil {
		t.Fatal("invalid server TLS fingerprint was accepted")
	}

	channel, err := newAdminForm("channel-create")
	if err != nil {
		t.Fatal(err)
	}
	channel.Fields[0].Value = "alice"
	channel.Fields[1].Value = "libera"
	channel.Fields[2].Value = "#chat"
	channel.Fields[6].Value = "-1m"
	if _, err := buildAdminOperation("/etc/soju/config", channel); err == nil {
		t.Fatal("negative detach duration was accepted")
	}
}

func TestDeviceCertificateFingerprintValidation(t *testing.T) {
	form, err := newAdminForm("device-cert-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = strings.Repeat("AA", 64)
	form.Fields[2].Value = "laptop"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatalf("valid SHA-512 fingerprint rejected: %v", err)
	}
	if op.ConfirmPhrase != "REGISTER DEVICE CERTIFICATE" {
		t.Fatalf("device registration confirmation = %q", op.ConfirmPhrase)
	}
	form.Fields[1].Value = strings.Repeat("AA", 32)
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil {
		t.Fatal("SHA-256 device registration fingerprint was accepted")
	}

	deleteForm, err := newAdminForm("device-cert-delete")
	if err != nil {
		t.Fatal(err)
	}
	deleteForm.Fields[0].Value = "alice"
	deleteForm.Fields[1].Value = strings.Repeat("AA", 10)
	if _, err := buildAdminOperation("/etc/soju/config", deleteForm); err != nil {
		t.Fatalf("valid device fingerprint prefix rejected: %v", err)
	}
}

func TestNetworkRejectsUnsafeAdditionalConnectCommands(t *testing.T) {
	form, err := newAdminForm("network-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "ircs://irc.example.test:6697"
	for _, invalid := range []string{`not-json`, `["JOIN #ok","PRIVMSG x :bad\nline"]`} {
		form.Fields[12].Value = invalid
		if _, err := buildAdminOperation("/etc/soju/config", form); err == nil {
			t.Fatalf("additional connect commands %q were accepted", invalid)
		}
	}
}

func TestChannelStatusCanListAllNetworks(t *testing.T) {
	form, err := newAdminForm("channel-status")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if containsArg(op.Args, "-network") {
		t.Fatalf("blank network unexpectedly added a filter: %#v", op.Args)
	}
	form.Fields[1].Value = allNetworksSelection
	op, err = buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if containsArg(op.Args, "-network") {
		t.Fatalf("All networks unexpectedly added a literal filter: %#v", op.Args)
	}
}

func TestSASLStatusCanListAllNetworks(t *testing.T) {
	form, err := newAdminForm("sasl-status")
	if err != nil {
		t.Fatal(err)
	}
	if !networkSelectionAllowsAll(form.Kind) {
		t.Fatal("SASL status does not allow all-network selection")
	}
	if err := addNetworkChoices(form, "alice", []NetworkStatus{
		{Name: "libera"},
		{Name: "ouch"},
	}, true); err != nil {
		t.Fatal(err)
	}
	if got := form.Fields[1]; got.Kind != "network" || got.Value != allNetworksSelection || len(got.Options) != 3 {
		t.Fatalf("network selector = %#v", got)
	}
}

func TestSpecificUserStatusAndIdentityUpdate(t *testing.T) {
	status, err := newAdminForm("user-status-specific")
	if err != nil {
		t.Fatal(err)
	}
	status.Fields[0].Value = "alice"
	op, err := buildAdminOperation("/etc/soju/config", status)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(op.Args, " "); got != "user status alice" || op.Mutating {
		t.Fatalf("specific user status = %#v", op)
	}

	identity, err := newAdminForm("user-identity-update")
	if err != nil {
		t.Fatal(err)
	}
	identity.Fields[0].Value = "alice"
	if _, err := buildAdminOperation("/etc/soju/config", identity); err == nil {
		t.Fatal("empty identity update was accepted")
	}
	identity.Fields[1].Value = "alice_"
	op, err = buildAdminOperation("/etc/soju/config", identity)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(op.Args, " "); got != "user run alice user update -nick alice_" {
		t.Fatalf("identity update args = %q", got)
	}
	identity.Fields[1].Value = ""
	identity.Fields[3].Value = "nickname"
	op, err = buildAdminOperation("/etc/soju/config", identity)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(op.Args, "\x00"); !strings.Contains(got, "-nick\x00") || op.ConfirmPhrase != "CLEAR USER IDENTITY SETTING" {
		t.Fatalf("identity clear = %#v", op)
	}
}

func TestNetworkUpdatePrefillSubmitsOnlyChanges(t *testing.T) {
	form := newNetworkUpdateForm("alice", NetworkStatus{Name: "libera", Address: "ircs://irc.libera.chat:6697"})
	form.Fields[4].Value = "alice_"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if op.ConfirmationImpact != adminConfirmationChange {
		t.Fatalf("network-update confirmation impact = %v, want change", op.ConfirmationImpact)
	}
	joined := strings.Join(op.Args, "\x00")
	if !strings.Contains(joined, "-nick\x00alice_") {
		t.Fatalf("nickname change missing: %#v", op.Args)
	}
	if strings.Contains(joined, "-addr") || strings.Contains(joined, "-name") || strings.Contains(joined, "-enabled") {
		t.Fatalf("unchanged prefilled values were submitted: %#v", op.Args)
	}
}

func TestNetworkUpdateRejectsNoChangesAndTargetChange(t *testing.T) {
	form := newNetworkUpdateForm("alice", NetworkStatus{Name: "libera", Address: "ircs://irc.libera.chat:6697"})
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil || !strings.Contains(err.Error(), "no network settings changed") {
		t.Fatalf("expected no-change error, got %v", err)
	}
	form.Fields[1].Value = "other"
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil || !strings.Contains(err.Error(), "cannot be changed") {
		t.Fatalf("expected immutable-target error, got %v", err)
	}
}

func TestNetworkReconnectBuildsSafeUpdateOperation(t *testing.T) {
	form, err := newAdminForm("network-reconnect")
	if err != nil {
		t.Fatal(err)
	}
	setAdminField(t, form, "User", "ak")
	setAdminField(t, form, "Network", "oftc")
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"user", "run", "ak", "network", "update", "oftc"}
	if strings.Join(op.Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("reconnect argv = %#v, want %#v", op.Args, want)
	}
	if !op.Mutating {
		t.Fatal("reconnect must be treated as a mutating operation")
	}
	if op.ConfirmationImpact != adminConfirmationChange {
		t.Fatalf("reconnect confirmation impact = %v, want change", op.ConfirmationImpact)
	}
	if strings.Contains(op.Preview, "sh -c") {
		t.Fatal("reconnect operation must not invoke a shell")
	}
}

func TestNonTextFormFieldIgnoresTyping(t *testing.T) {
	app := newTestApp()
	app.admin.Form = &AdminForm{Fields: []AdminField{{Kind: "readonly", Value: "alice"}}}
	app.adminHandleKey("x", 'x')
	app.adminHandleKey("backspace", 0)
	if got := app.admin.Form.Fields[0].Value; got != "alice" {
		t.Fatalf("read-only field changed to %q", got)
	}
	app.close()
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
