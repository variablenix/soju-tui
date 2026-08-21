package main

import (
	"strings"
	"testing"
)

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

func TestParseUserDeleteConfirmation(t *testing.T) {
	args, username, ok := parseUserDeleteConfirmation(`To confirm user deletion, send "user delete alice 0123ab"`)
	if !ok || username != "alice" || len(args) != 4 || args[2] != "alice" || args[3] != "0123ab" {
		t.Fatalf("unexpected delete confirmation parse: args=%#v username=%q ok=%v", args, username, ok)
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
	form.Fields[2].Value = "ed25519"
	op, err = buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(op.Args, "-key-type") || containsArg(op.Args, "-bits") {
		t.Fatalf("ed25519 options are invalid: %#v", op.Args)
	}
}

func TestNetworkUpdatePrefillSubmitsOnlyChanges(t *testing.T) {
	form := newNetworkUpdateForm("alice", NetworkStatus{Name: "libera", Address: "ircs://irc.libera.chat:6697"})
	form.Fields[4].Value = "alice_"
	op, err := buildAdminOperation("/etc/soju/config", form)
	if err != nil {
		t.Fatal(err)
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
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil || !strings.Contains(err.Error(), "No network settings changed") {
		t.Fatalf("expected no-change error, got %v", err)
	}
	form.Fields[1].Value = "other"
	if _, err := buildAdminOperation("/etc/soju/config", form); err == nil || !strings.Contains(err.Error(), "cannot be changed") {
		t.Fatalf("expected immutable-target error, got %v", err)
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
