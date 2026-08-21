package main

import "testing"

func TestParseNetworkStatuses(t *testing.T) {
	output := "ouch (ircs://irc.ouch.chat:6697) [connected]: 7 channels\n" +
		"ircs://irc.example.test:6697 [disabled]: 0 channels\n"
	networks := parseNetworkStatuses(output)
	if len(networks) != 2 {
		t.Fatalf("got %d networks: %#v", len(networks), networks)
	}
	if networks[0].Name != "ouch" || networks[0].Address != "ircs://irc.ouch.chat:6697" || networks[0].Disabled {
		t.Fatalf("unexpected named network: %#v", networks[0])
	}
	if networks[1].Name != "" || !networks[1].Disabled {
		t.Fatalf("unexpected unnamed network: %#v", networks[1])
	}
}

func TestFindNetworkStatusByNameOrAddress(t *testing.T) {
	output := "libera (ircs://irc.libera.chat:6697) [connected]: 4 channels\n"
	for _, target := range []string{"libera", "ircs://irc.libera.chat:6697"} {
		network, err := findNetworkStatus(output, target)
		if err != nil || network.Name != "libera" {
			t.Fatalf("target %q: network=%#v err=%v", target, network, err)
		}
	}
}
