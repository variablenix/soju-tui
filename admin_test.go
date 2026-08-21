package main

import (
	"strings"
	"testing"
)

func TestBuildAdminUserCreateMasksSecrets(t *testing.T) {
	form, err := newAdminForm("user-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "p@ss word"
	form.Fields[2].Value = "false"
	form.Fields[5].Value = "true"
	form.Fields[6].Value = "-1"
	op, err := buildAdminOperation(form)
	if err != nil {
		t.Fatal(err)
	}
	if !op.Mutating || !strings.Contains(op.Command, "user create") {
		t.Fatalf("unexpected operation: %#v", op)
	}
	if strings.Contains(op.Preview, "p@ss word") || !strings.Contains(op.Preview, "••••••") {
		t.Fatalf("secret leaked in preview: %q", op.Preview)
	}
}

func TestBuildAdminNetworkQuotePreservesSpaces(t *testing.T) {
	form, err := newAdminForm("network-quote")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "libera"
	form.Fields[2].Value = "PRIVMSG NickServ :IDENTIFY a password"
	op, err := buildAdminOperation(form)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(op.Command, "network quote libera") || !strings.Contains(op.Command, "IDENTIFY a password") {
		t.Fatalf("raw command was not preserved: %q", op.Command)
	}
}

func TestNetworkCreateDoesNotRequireExistingNetwork(t *testing.T) {
	form, err := newAdminForm("network-create")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "alice"
	form.Fields[1].Value = "ircs://irc.example.test:6697"
	op, err := buildAdminOperation(form)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(op.Command, "network create") || strings.Contains(op.Command, "network create ''") {
		t.Fatalf("unexpected network create command: %q", op.Command)
	}
}

func TestBuildAdminRejectsIRCLineInjection(t *testing.T) {
	form, err := newAdminForm("server-notice")
	if err != nil {
		t.Fatal(err)
	}
	form.Fields[0].Value = "hello\r\nPRIVMSG BouncerServ :injected"
	if _, err := buildAdminOperation(form); err == nil {
		t.Fatal("expected control-character rejection")
	}
}

func TestAdminJoinQuotesShellMetacharacters(t *testing.T) {
	got := adminJoin("server", "notice", "hello; touch /tmp/should-not-run")
	want := "server notice 'hello; touch /tmp/should-not-run'"
	if got != want {
		t.Fatalf("unsafe token was not quoted: got %q want %q", got, want)
	}
}
