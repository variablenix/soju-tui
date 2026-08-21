package main

import "testing"

func TestParseChannelStatuses(t *testing.T) {
	output := "#chat [joined]\n#ops [parted, detached]\n"
	channels := parseChannelStatuses(output)
	if len(channels) != 2 {
		t.Fatalf("got %d channels: %#v", len(channels), channels)
	}
	if channels[0].Name != "#chat" || channels[0].Detached {
		t.Fatalf("unexpected joined channel: %#v", channels[0])
	}
	if channels[1].Name != "#ops" || !channels[1].Detached {
		t.Fatalf("unexpected detached channel: %#v", channels[1])
	}
}

func TestFindChannelStatus(t *testing.T) {
	channel, err := findChannelStatus("#chat [joined, detached]\n", "#chat")
	if err != nil || channel.Name != "#chat" || !channel.Detached {
		t.Fatalf("channel=%#v err=%v", channel, err)
	}
	if _, err := findChannelStatus("#chat [joined]\n", "#missing"); err == nil {
		t.Fatal("missing channel was accepted")
	}
}
