package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSojuSystemdUnit = "soju.service"
	runtimeCommandLimit    = 64 << 10
	runtimeEnrichmentLimit = 10 * time.Second
	maxJournalMicroseconds = int64((1<<63 - 1) / 1000)
)

var (
	errRuntimeStatusUnavailable = errors.New("runtime status unavailable")
	systemdUnitPattern          = regexp.MustCompile(`^[A-Za-z0-9_.@:-]+$`)
)

// RuntimeStatusSource provides optional, read-only timestamps that are not
// exposed by sojuctl. Implementations must return an error rather than infer a
// timestamp when their source data is missing or ambiguous.
type RuntimeStatusSource interface {
	ServerStartedAt(context.Context) (time.Time, error)
	NetworkConnectedAt(context.Context, time.Time, string, string) (time.Time, error)
}

type systemdRuntimeStatusSource struct {
	systemctl  string
	journalctl string
	unit       string
	timeout    time.Duration
}

func discoverRuntimeStatusSource(unit string, timeout time.Duration) (RuntimeStatusSource, error) {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return nil, nil
	}
	if strings.HasPrefix(unit, "-") || !systemdUnitPattern.MatchString(unit) {
		return nil, fmt.Errorf("invalid Soju systemd unit %q", unit)
	}
	if runtime.GOOS != "linux" {
		return nil, nil
	}
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return nil, nil
	}
	journalctl, _ := exec.LookPath("journalctl")
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	return &systemdRuntimeStatusSource{
		systemctl:  systemctl,
		journalctl: journalctl,
		unit:       unit,
		timeout:    timeout,
	}, nil
}

func (s *systemdRuntimeStatusSource) ServerStartedAt(parent context.Context) (time.Time, error) {
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	output, err := s.run(ctx, s.systemctl,
		"show", "--no-pager", "--property=ActiveState", "--property=ExecMainStartTimestamp", s.unit)
	if err != nil {
		return time.Time{}, errRuntimeStatusUnavailable
	}
	properties := parseSystemdProperties(string(output))
	if properties["ActiveState"] != "active" {
		return time.Time{}, errRuntimeStatusUnavailable
	}
	startedAt, err := parseSystemdTimestamp(properties["ExecMainStartTimestamp"])
	if err != nil {
		return time.Time{}, errRuntimeStatusUnavailable
	}
	return startedAt.UTC(), nil
}

func (s *systemdRuntimeStatusSource) NetworkConnectedAt(parent context.Context, serverStartedAt time.Time, user, network string) (time.Time, error) {
	if s.journalctl == "" || user == "" || network == "" || serverStartedAt.IsZero() {
		return time.Time{}, errRuntimeStatusUnavailable
	}
	needle := fmt.Sprintf("user %q: upstream %q: connection registered with nick", user, network)
	pattern := regexp.QuoteMeta(needle)
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()
	output, err := s.run(ctx, s.journalctl,
		"--unit="+s.unit,
		"--output=json",
		"--reverse",
		"--lines=1",
		"--no-pager",
		"--quiet",
		"--since=@"+strconv.FormatInt(serverStartedAt.Unix(), 10),
		"--grep="+pattern,
	)
	if err != nil {
		return time.Time{}, errRuntimeStatusUnavailable
	}
	connectedAt, err := parseJournalTimestamp(output)
	if err != nil || connectedAt.Before(serverStartedAt) {
		return time.Time{}, errRuntimeStatusUnavailable
	}
	return connectedAt.UTC(), nil
}

func (s *systemdRuntimeStatusSource) run(ctx context.Context, path string, args ...string) ([]byte, error) {
	// #nosec G204 -- executable paths come from exec.LookPath, arguments are
	// fixed or validated local identifiers, and no command shell is involved.
	command := exec.CommandContext(ctx, path, args...)
	command.Env = runtimeStatusEnvironment(os.Environ())
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, runtimeCommandLimit+1))
	if len(output) > runtimeCommandLimit {
		_ = command.Process.Kill()
		_ = command.Wait()
		return nil, errors.New("runtime status output exceeded limit")
	}
	waitErr := command.Wait()
	if readErr != nil {
		return nil, readErr
	}
	if waitErr != nil {
		return nil, waitErr
	}
	return output, nil
}

func runtimeStatusEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment)+3)
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "LANG", "LC_ALL", "TZ":
			continue
		default:
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, "LANG=C", "LC_ALL=C", "TZ=UTC")
}

func parseSystemdProperties(output string) map[string]string {
	properties := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			properties[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return properties
}

func parseSystemdTimestamp(value string) (time.Time, error) {
	for _, layout := range []string{
		"Mon 2006-01-02 15:04:05 MST",
		"Mon 2006-01-02 15:04:05.999999 MST",
	} {
		if parsed, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, errRuntimeStatusUnavailable
}

func parseJournalTimestamp(output []byte) (time.Time, error) {
	line := strings.TrimSpace(string(output))
	if line == "" {
		return time.Time{}, errRuntimeStatusUnavailable
	}
	var record struct {
		Realtime string `json:"__REALTIME_TIMESTAMP"`
	}
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		return time.Time{}, errRuntimeStatusUnavailable
	}
	microseconds, err := strconv.ParseInt(record.Realtime, 10, 64)
	if err != nil || microseconds <= 0 || microseconds > maxJournalMicroseconds {
		return time.Time{}, errRuntimeStatusUnavailable
	}
	return time.Unix(0, microseconds*int64(time.Microsecond)), nil
}

func enrichRuntimeStatus(ctx context.Context, source RuntimeStatusSource, args []string, output string, now time.Time) string {
	ctx, cancel := context.WithTimeout(ctx, runtimeEnrichmentLimit)
	defer cancel()
	if isServerStatusArgs(args) {
		startedAt, err := source.ServerStartedAt(ctx)
		if err != nil {
			return output
		}
		return appendServerUptime(output, startedAt, now)
	}
	user, ok := networkStatusUser(args)
	if !ok {
		return output
	}
	serverStartedAt, err := source.ServerStartedAt(ctx)
	if err != nil {
		return output
	}
	connectedAt := make(map[string]time.Time)
	for _, network := range parseNetworkStatuses(output) {
		if !network.Connected {
			continue
		}
		when, err := source.NetworkConnectedAt(ctx, serverStartedAt, user, network.Target())
		if err == nil {
			connectedAt[network.Target()] = when
		}
	}
	return appendNetworkConnectionAges(output, connectedAt, now)
}

func isServerStatusArgs(args []string) bool {
	return len(args) == 2 && args[0] == "server" && args[1] == "status"
}

func networkStatusUser(args []string) (string, bool) {
	if len(args) == 5 && args[0] == "user" && args[1] == "run" && args[2] != "" && args[3] == "network" && args[4] == "status" {
		return args[2], true
	}
	return "", false
}

func appendServerUptime(output string, startedAt, now time.Time) string {
	if startedAt.IsZero() || !now.After(startedAt) || strings.Contains(output, "; uptime ") {
		return output
	}
	return appendStatusSuffix(output, fmt.Sprintf("uptime %s (since %s)", formatStatusAge(now.Sub(startedAt)), startedAt.UTC().Format(time.RFC3339)))
}

func appendNetworkConnectionAges(output string, connectedAt map[string]time.Time, now time.Time) string {
	if len(connectedAt) == 0 {
		return output
	}
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	for index, line := range lines {
		if strings.Contains(line, "; connected for ") {
			continue
		}
		networks := parseNetworkStatuses(line)
		if len(networks) != 1 || !networks[0].Connected {
			continue
		}
		when, ok := connectedAt[networks[0].Target()]
		if !ok || when.IsZero() || !now.After(when) {
			continue
		}
		lines[index] = strings.TrimRight(line, "; ") + fmt.Sprintf("; connected for %s (since %s)", formatStatusAge(now.Sub(when)), when.UTC().Format(time.RFC3339))
	}
	result := strings.Join(lines, "\n")
	if strings.HasSuffix(output, "\n") {
		result += "\n"
	}
	return result
}

func appendStatusSuffix(output, suffix string) string {
	trimmed := strings.TrimRight(output, "\n")
	if trimmed == "" {
		return output
	}
	lines := strings.Split(trimmed, "\n")
	lines[len(lines)-1] = strings.TrimRight(lines[len(lines)-1], "; ") + "; " + suffix
	result := strings.Join(lines, "\n")
	if strings.HasSuffix(output, "\n") {
		result += "\n"
	}
	return result
}

func formatStatusAge(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	seconds := int64(duration / time.Second)
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60
	remainingSeconds := seconds % 60
	switch {
	case days > 0:
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	case hours > 0:
		if minutes > 0 {
			return fmt.Sprintf("%dh %dm", hours, minutes)
		}
		return fmt.Sprintf("%dh", hours)
	case minutes > 0:
		if remainingSeconds > 0 {
			return fmt.Sprintf("%dm %ds", minutes, remainingSeconds)
		}
		return fmt.Sprintf("%dm", minutes)
	default:
		return fmt.Sprintf("%ds", remainingSeconds)
	}
}
