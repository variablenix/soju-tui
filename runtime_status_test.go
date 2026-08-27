package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRuntimeStatusSource struct {
	serverStartedAt time.Time
	connections     map[string]time.Time
	serverErr       error
}

func (s fakeRuntimeStatusSource) ServerStartedAt(context.Context) (time.Time, error) {
	return s.serverStartedAt, s.serverErr
}

func (s fakeRuntimeStatusSource) NetworkConnectedAt(_ context.Context, _ time.Time, user, network string) (time.Time, error) {
	connectedAt, ok := s.connections[user+"/"+network]
	if !ok {
		return time.Time{}, errRuntimeStatusUnavailable
	}
	return connectedAt, nil
}

func TestEnrichServerRuntimeStatus(t *testing.T) {
	startedAt := time.Date(2026, 7, 25, 5, 41, 2, 0, time.UTC)
	now := startedAt.Add(31*24*time.Hour + 4*time.Hour)
	source := fakeRuntimeStatusSource{serverStartedAt: startedAt}
	input := "2/2 users, 6 downstreams, 6 upstreams, 6 networks, 39 channels\n"
	want := "2/2 users, 6 downstreams, 6 upstreams, 6 networks, 39 channels; uptime 31d 4h (since 2026-07-25T05:41:02Z)\n"
	if got := enrichRuntimeStatus(context.Background(), source, []string{"server", "status"}, input, now); got != want {
		t.Fatalf("enriched server status:\n got %q\nwant %q", got, want)
	}
	if got := enrichRuntimeStatus(context.Background(), fakeRuntimeStatusSource{serverErr: errors.New("unavailable")}, []string{"server", "status"}, input, now); got != input {
		t.Fatalf("unavailable source changed output: %q", got)
	}
}

func TestEnrichNetworkRuntimeStatus(t *testing.T) {
	startedAt := time.Date(2026, 7, 25, 5, 0, 0, 0, time.UTC)
	connectedAt := time.Date(2026, 7, 25, 5, 41, 2, 0, time.UTC)
	now := connectedAt.Add(31*24*time.Hour + 4*time.Hour)
	source := fakeRuntimeStatusSource{
		serverStartedAt: startedAt,
		connections: map[string]time.Time{
			"ak/AlienIRCd":                    connectedAt,
			"ak/ircs://irc.example.test:6697": connectedAt.Add(2 * time.Hour),
		},
	}
	input := strings.Join([]string{
		"AlienIRCd (ircs://irc.zrgw.dev:6697) [connected]: 2 channels",
		"ircs://irc.example.test:6697 [connected as ak_]: 1 channels",
		"Snoonet (ircs://irc.snoonet.org:6697) [disconnected]: connection refused",
		"Disabled (ircs://irc.disabled.test:6697) [disabled]: 0 channels",
		"",
	}, "\n")
	want := strings.Join([]string{
		"AlienIRCd (ircs://irc.zrgw.dev:6697) [connected]: 2 channels; connected for 31d 4h (since 2026-07-25T05:41:02Z)",
		"ircs://irc.example.test:6697 [connected as ak_]: 1 channels; connected for 31d 2h (since 2026-07-25T07:41:02Z)",
		"Snoonet (ircs://irc.snoonet.org:6697) [disconnected]: connection refused",
		"Disabled (ircs://irc.disabled.test:6697) [disabled]: 0 channels",
		"",
	}, "\n")
	args := []string{"user", "run", "ak", "network", "status"}
	if got := enrichRuntimeStatus(context.Background(), source, args, input, now); got != want {
		t.Fatalf("enriched network status:\n got %q\nwant %q", got, want)
	}
}

func TestRuntimeStatusDoesNotDuplicateOrInventTimes(t *testing.T) {
	now := time.Date(2026, 8, 25, 9, 41, 2, 0, time.UTC)
	future := now.Add(time.Hour)
	server := "status; uptime 2h (since 2026-08-25T07:41:02Z)\n"
	if got := appendServerUptime(server, now.Add(-time.Hour), now); got != server {
		t.Fatalf("duplicated uptime: %q", got)
	}
	if got := appendServerUptime("status\n", future, now); got != "status\n" {
		t.Fatalf("reported future uptime: %q", got)
	}
	network := "AlienIRCd (ircs://irc.zrgw.dev:6697) [connected]: 2 channels\n"
	if got := appendNetworkConnectionAges(network, map[string]time.Time{"AlienIRCd": future}, now); got != network {
		t.Fatalf("reported future connection age: %q", got)
	}
	if got := appendNetworkConnectionAges(network, nil, now); got != network {
		t.Fatalf("changed output without evidence: %q", got)
	}
}

func TestFormatStatusAge(t *testing.T) {
	tests := map[time.Duration]string{
		0:                             "0s",
		42 * time.Second:              "42s",
		3*time.Minute + 4*time.Second: "3m 4s",
		2*time.Hour + 5*time.Minute:   "2h 5m",
		31*24*time.Hour + 4*time.Hour: "31d 4h",
		-1 * time.Second:              "0s",
	}
	for input, want := range tests {
		if got := formatStatusAge(input); got != want {
			t.Errorf("formatStatusAge(%s) = %q, want %q", input, got, want)
		}
	}
}

func TestRuntimeStatusArgumentDetection(t *testing.T) {
	if !isServerStatusArgs([]string{"server", "status"}) || isServerStatusArgs([]string{"user", "status"}) {
		t.Fatal("server status argument detection failed")
	}
	user, ok := networkStatusUser([]string{"user", "run", "alice", "network", "status"})
	if !ok || user != "alice" {
		t.Fatalf("network status detection = %q, %v", user, ok)
	}
	for _, args := range [][]string{
		{"network", "status"},
		{"user", "run", "", "network", "status"},
		{"user", "run", "alice", "channel", "status"},
	} {
		if _, ok := networkStatusUser(args); ok {
			t.Fatalf("accepted non-network-status arguments: %#v", args)
		}
	}
}

func TestParseSystemdStatus(t *testing.T) {
	properties := parseSystemdProperties("ActiveState=active\nExecMainStartTimestamp=Sat 2026-07-25 05:41:02 UTC\nignored\n")
	if properties["ActiveState"] != "active" {
		t.Fatalf("active state = %q", properties["ActiveState"])
	}
	startedAt, err := parseSystemdTimestamp(properties["ExecMainStartTimestamp"])
	if err != nil || startedAt.UTC().Format(time.RFC3339) != "2026-07-25T05:41:02Z" {
		t.Fatalf("start timestamp = %v, %v", startedAt, err)
	}
	if _, err := parseSystemdTimestamp("n/a"); err == nil {
		t.Fatal("accepted missing systemd timestamp")
	}
}

func TestParseJournalTimestamp(t *testing.T) {
	got, err := parseJournalTimestamp([]byte(`{"__REALTIME_TIMESTAMP":"1784958062123456","MESSAGE":"registered"}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Unix(0, 1784958062123456*int64(time.Microsecond)); !got.Equal(want) {
		t.Fatalf("journal timestamp = %s, want %s", got, want)
	}
	for _, input := range []string{"", `{}`, `{"__REALTIME_TIMESTAMP":"bad"}`, `{"__REALTIME_TIMESTAMP":"9223372036854775807"}`, `not json`} {
		if _, err := parseJournalTimestamp([]byte(input)); err == nil {
			t.Errorf("accepted invalid journal record %q", input)
		}
	}
}

func TestSystemdRuntimeStatusSource(t *testing.T) {
	directory := t.TempDir()
	systemctl := writeRuntimeStub(t, directory, "systemctl", `#!/bin/sh
printf '%s\n' 'ActiveState=active' 'ExecMainStartTimestamp=Sat 2026-07-25 05:41:02 UTC'
`)
	journalctl := writeRuntimeStub(t, directory, "journalctl", `#!/bin/sh
printf '%s\n' '{"__REALTIME_TIMESTAMP":"1784958062000000","MESSAGE":"registered"}'
`)
	source := &systemdRuntimeStatusSource{systemctl: systemctl, journalctl: journalctl, unit: "soju.service", timeout: time.Second}
	startedAt, err := source.ServerStartedAt(context.Background())
	if err != nil || startedAt.Format(time.RFC3339) != "2026-07-25T05:41:02Z" {
		t.Fatalf("ServerStartedAt() = %s, %v", startedAt, err)
	}
	connectedAt, err := source.NetworkConnectedAt(context.Background(), startedAt, "ak", "AlienIRCd")
	if err != nil || connectedAt.UnixMicro() != 1784958062000000 {
		t.Fatalf("NetworkConnectedAt() = %s, %v", connectedAt, err)
	}
}

func TestSystemdRuntimeStatusSourceFailsClosed(t *testing.T) {
	directory := t.TempDir()
	inactive := writeRuntimeStub(t, directory, "inactive", `#!/bin/sh
printf '%s\n' 'ActiveState=inactive' 'ExecMainStartTimestamp=n/a'
`)
	failing := writeRuntimeStub(t, directory, "failing", "#!/bin/sh\nexit 1\n")
	source := &systemdRuntimeStatusSource{systemctl: inactive, journalctl: failing, unit: "soju.service", timeout: time.Second}
	if _, err := source.ServerStartedAt(context.Background()); err == nil {
		t.Fatal("inactive unit produced a start time")
	}
	source.systemctl = failing
	if _, err := source.ServerStartedAt(context.Background()); err == nil {
		t.Fatal("failed systemctl produced a start time")
	}
	if _, err := source.NetworkConnectedAt(context.Background(), time.Now(), "", "AlienIRCd"); err == nil {
		t.Fatal("empty user produced a connection time")
	}
	if _, err := source.NetworkConnectedAt(context.Background(), time.Now(), "ak", "AlienIRCd"); err == nil {
		t.Fatal("failed journalctl produced a connection time")
	}
}

func TestRuntimeStatusCommandOutputLimit(t *testing.T) {
	directory := t.TempDir()
	large := writeRuntimeStub(t, directory, "large", `#!/bin/sh
dd if=/dev/zero bs=1024 count=65 2>/dev/null
`)
	source := &systemdRuntimeStatusSource{systemctl: large, unit: "soju.service", timeout: time.Second}
	if _, err := source.run(context.Background(), large); err == nil {
		t.Fatal("accepted oversized runtime command output")
	}
}

func TestRuntimeStatusEnvironmentOverridesLocaleAndTimezone(t *testing.T) {
	environment := runtimeStatusEnvironment([]string{"PATH=/usr/bin", "LANG=fr_FR.UTF-8", "LC_ALL=de_DE.UTF-8", "TZ=America/Los_Angeles"})
	got := strings.Join(environment, "\n")
	for _, want := range []string{"PATH=/usr/bin", "LANG=C", "LC_ALL=C", "TZ=UTC"} {
		if !strings.Contains(got, want) {
			t.Errorf("environment does not contain %q: %q", want, got)
		}
	}
	for _, unwanted := range []string{"fr_FR", "de_DE", "America/Los_Angeles"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("environment retained %q: %q", unwanted, got)
		}
	}
}

func TestDiscoverRuntimeStatusSourceRejectsInvalidUnit(t *testing.T) {
	if _, err := discoverRuntimeStatusSource("--user", time.Second); err == nil {
		t.Fatal("accepted option-like systemd unit")
	}
	if source, err := discoverRuntimeStatusSource("", time.Second); err != nil || source != nil {
		t.Fatalf("disabled runtime source = %#v, %v", source, err)
	}
}

func writeRuntimeStub(t *testing.T, directory, name, content string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
