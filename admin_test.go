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

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
