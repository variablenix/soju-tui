package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseIRCLineAndTags(t *testing.T) {
	msg, err := parseIRCLine(`@time=2026-08-20T12:34:56.000Z;label=a\sb :alice!u@host PRIVMSG #chat :hello world`)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Command != "PRIVMSG" || msg.Prefix != "alice!u@host" {
		t.Fatalf("unexpected message: %#v", msg)
	}
	if msg.Tags["label"] != "a b" || msg.Params[1] != "hello world" {
		t.Fatalf("unexpected parsed fields: %#v", msg)
	}
	if got := msg.String(); !strings.Contains(got, "PRIVMSG #chat :hello world") {
		t.Fatalf("message did not round-trip: %q", got)
	}
}

func TestParseNetworkAttrs(t *testing.T) {
	attrs := parseNetworkAttrs(`name=Libera;host=irc.libera.chat;port=6697;tls=1;realname=My\sIRC\:client`)
	if attrs["name"] != "Libera" || attrs["realname"] != "My IRC;client" {
		t.Fatalf("unexpected attrs: %#v", attrs)
	}
}

func TestRegisterWithSASLAndBind(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		read := func() (IRCMessage, error) {
			line, err := reader.ReadString('\n')
			if err != nil {
				return IRCMessage{}, err
			}
			return parseIRCLine(line)
		}
		if msg, err := read(); err != nil || msg.Command != "CAP" {
			serverErr <- fmt.Errorf("expected CAP: %v", err)
			return
		}
		if msg, err := read(); err != nil || msg.Command != "NICK" {
			serverErr <- fmt.Errorf("expected NICK: %v", err)
			return
		}
		if msg, err := read(); err != nil || msg.Command != "USER" {
			serverErr <- fmt.Errorf("expected USER: %v", err)
			return
		}
		_, _ = conn.Write([]byte(":soju CAP * LS :sasl=PLAIN batch message-tags server-time soju.im/bouncer-networks draft/chathistory\r\n"))
		msg, err := read()
		if err != nil || msg.Command != "CAP" || len(msg.Params) < 1 || msg.Params[0] != "REQ" {
			serverErr <- fmt.Errorf("expected CAP REQ: %v (%#v)", err, msg)
			return
		}
		_, _ = conn.Write([]byte(":soju CAP * ACK :sasl batch message-tags server-time soju.im/bouncer-networks draft/chathistory\r\n"))
		if msg, err = read(); err != nil || msg.Command != "AUTHENTICATE" || msg.Params[0] != "PLAIN" {
			serverErr <- fmt.Errorf("expected AUTHENTICATE PLAIN: %v (%#v)", err, msg)
			return
		}
		_, _ = conn.Write([]byte(":soju AUTHENTICATE +\r\n"))
		if msg, err = read(); err != nil || msg.Command != "AUTHENTICATE" {
			serverErr <- fmt.Errorf("expected AUTHENTICATE payload: %v (%#v)", err, msg)
			return
		}
		decoded, err := base64.StdEncoding.DecodeString(msg.Params[0])
		if err != nil || string(decoded) != "\x00alice@client-7\x00secret" {
			serverErr <- fmt.Errorf("unexpected SASL payload: %v (%q)", err, decoded)
			return
		}
		_, _ = conn.Write([]byte(":soju 903 * :SASL authentication successful\r\n"))
		if msg, err = read(); err != nil || msg.Command != "BOUNCER" || len(msg.Params) != 2 || msg.Params[0] != "BIND" || msg.Params[1] != "42" {
			serverErr <- fmt.Errorf("expected BOUNCER BIND: %v (%#v)", err, msg)
			return
		}
		if msg, err = read(); err != nil || msg.Command != "CAP" || msg.Params[0] != "END" {
			serverErr <- fmt.Errorf("expected CAP END: %v (%#v)", err, msg)
			return
		}
		_, _ = conn.Write([]byte(":soju 001 alice :Welcome\r\n"))
		serverErr <- nil
	}()

	session, err := connectSession(SessionConfig{
		Address:       listener.Addr().String(),
		Username:      "alice",
		Password:      "secret",
		Nick:          "alice",
		Realname:      "test",
		ClientName:    "client-7",
		BindNetworkID: "42",
	})
	if err != nil {
		select {
		case serverErrValue := <-serverErr:
			t.Fatalf("%v; mock server: %v", err, serverErrValue)
		case <-time.After(2 * time.Second):
			t.Fatal(err)
		}
	}
	session.Close()
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mock soju server timed out")
	}
	if !session.EnabledCaps["sasl"] || !session.EnabledCaps["soju.im/bouncer-networks"] {
		t.Fatalf("capabilities were not recorded: %#v", session.EnabledCaps)
	}
}
