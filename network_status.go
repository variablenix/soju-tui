package main

import (
	"fmt"
	"regexp"
	"strings"
)

type NetworkStatus struct {
	Name      string
	Address   string
	Disabled  bool
	Connected bool
}

func (n NetworkStatus) Target() string {
	if n.Name != "" {
		return n.Name
	}
	return n.Address
}

var (
	namedNetworkStatus   = regexp.MustCompile(`^(.+) \(([^()]*)\) \[([^]]*)\](:.*)?$`)
	unnamedNetworkStatus = regexp.MustCompile(`^(\S+) \[([^]]*)\](:.*)?$`)
)

func parseNetworkStatuses(output string) []NetworkStatus {
	var networks []NetworkStatus
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if match := namedNetworkStatus.FindStringSubmatch(line); len(match) == 5 {
			networks = append(networks, NetworkStatus{
				Name:      strings.TrimSpace(match[1]),
				Address:   strings.TrimSpace(match[2]),
				Disabled:  statusContains(match[3], "disabled"),
				Connected: statusStartsWith(match[3], "connected"),
			})
			continue
		}
		if match := unnamedNetworkStatus.FindStringSubmatch(line); len(match) == 4 {
			networks = append(networks, NetworkStatus{
				Address:   strings.TrimSpace(match[1]),
				Disabled:  statusContains(match[2], "disabled"),
				Connected: statusStartsWith(match[2], "connected"),
			})
		}
	}
	return networks
}

func statusStartsWith(statuses, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, status := range strings.Split(statuses, ",") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(status)), want) {
			return true
		}
	}
	return false
}

func statusContains(statuses, want string) bool {
	for _, status := range strings.Split(statuses, ",") {
		if strings.EqualFold(strings.TrimSpace(status), want) {
			return true
		}
	}
	return false
}

func findNetworkStatus(output, target string) (NetworkStatus, error) {
	target = strings.TrimSpace(target)
	for _, network := range parseNetworkStatuses(output) {
		if network.Name == target || network.Address == target {
			return network, nil
		}
	}
	return NetworkStatus{}, fmt.Errorf("network %q was not found in sojuctl network status", target)
}
