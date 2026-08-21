package main

import (
	"reflect"
	"testing"
)

// These fixtures are the exact command lists emitted by an administrator's
// global and per-user BouncerServ help in upstream Soju v0.9.0 and v0.10.1.
// Debian's 0.9.0-1 and 0.10.1-1 packages do not patch service.go or sojuctl.
const sojuV090GlobalHelp = "available commands: help, server debug, server notice, server status, user create, user delete, user run, user status, user update"
const sojuV090UserHelp = "available commands: certfp fingerprint, certfp generate, channel create, channel delete, channel status, channel update, help, network create, network delete, network quote, network status, network update, sasl reset, sasl set-plain, sasl status, server debug, server notice, server status, user create, user delete, user run, user status, user update"
const sojuV0101GlobalHelp = "available commands: help, server debug, server notice, server status, user create, user delete, user run, user status, user update"
const sojuV0101UserHelp = "available commands: certfp fingerprint, certfp generate, channel create, channel delete, channel status, channel update, help, network create, network delete, network quote, network status, network update, sasl reset, sasl set-plain, sasl status, server debug, server notice, server status, user create, user delete, user run, user status, user update"

func mergedHelpCommands(help ...string) map[string]bool {
	commands := make(map[string]bool)
	for _, output := range help {
		for command := range parseAdminCommandHelp(output) {
			commands[command] = true
		}
	}
	return commands
}

func TestSojuV090AndV0101ExposeTheSameAdminContract(t *testing.T) {
	releases := map[string]struct {
		global string
		user   string
	}{
		"upstream-v0.9.0-and-debian-0.9.0-1":   {sojuV090GlobalHelp, sojuV090UserHelp},
		"upstream-v0.10.1-and-debian-0.10.1-1": {sojuV0101GlobalHelp, sojuV0101UserHelp},
	}

	var baseline map[string]bool
	for release, fixture := range releases {
		commands := mergedHelpCommands(fixture.global, fixture.user)
		if baseline == nil {
			baseline = commands
		} else if !reflect.DeepEqual(commands, baseline) {
			t.Fatalf("%s command contract differs from the v0.9.0 baseline", release)
		}

		app := newTestApp()
		app.admin.Capabilities = AdminCapabilities{Known: true, Commands: commands}
		for _, item := range app.adminMenuItemsLocked() {
			if item.Command != "" && !commands[item.Command] {
				t.Errorf("%s exposed unsupported menu command %q", release, item.Command)
			}
			if item.Kind == "device-cert-status" || item.Kind == "device-cert-create" || item.Kind == "device-cert-delete" {
				t.Errorf("%s exposed post-v0.10.1 device-certificate action %q", release, item.Kind)
			}
		}
		app.close()
	}

	for _, required := range []string{
		"certfp fingerprint", "certfp generate",
		"channel create", "channel delete", "channel status", "channel update",
		"network create", "network delete", "network quote", "network status", "network update",
		"sasl reset", "sasl set-plain", "sasl status",
		"server debug", "server notice", "server status",
		"user create", "user delete", "user run", "user status", "user update",
	} {
		if !baseline[required] {
			t.Errorf("stable compatibility fixture is missing %q", required)
		}
	}
}

func TestStableSojuUnixNetworkAddressCompatibility(t *testing.T) {
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
	for index, arg := range op.Args {
		if arg == "-addr" && index+1 < len(op.Args) {
			if got, want := op.Args[index+1], "unix:///run/irc/upstream.sock"; got != want {
				t.Fatalf("stable Soju Unix network address = %q, want %q", got, want)
			}
			return
		}
	}
	t.Fatalf("network create operation has no -addr value: %#v", op.Args)
}
