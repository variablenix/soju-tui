package main

import (
	"fmt"
	"regexp"
	"strings"
)

type ChannelStatus struct {
	Name     string
	Detached bool
}

var channelStatusLine = regexp.MustCompile(`^(.+) \[([^]]*)\]$`)

func parseChannelStatuses(output string) []ChannelStatus {
	var channels []ChannelStatus
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		match := channelStatusLine.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		channels = append(channels, ChannelStatus{
			Name:     strings.TrimSpace(match[1]),
			Detached: statusContains(match[2], "detached"),
		})
	}
	return channels
}

func findChannelStatus(output, target string) (ChannelStatus, error) {
	target = strings.TrimSpace(target)
	for _, channel := range parseChannelStatuses(output) {
		if channel.Name == target {
			return channel, nil
		}
	}
	return ChannelStatus{}, fmt.Errorf("channel %q was not found in sojuctl channel status", target)
}
