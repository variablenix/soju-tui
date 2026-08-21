package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var adminSocketPermissionPattern = regexp.MustCompile(`(?m)dial unix ([^\r\n]+): connect: permission denied`)
var clientCertificateDisabledPattern = regexp.MustCompile(`(?i)client (certificate|certification) authentication.*disabled`)
var deviceCertificateUnsupportedPattern = regexp.MustCompile(`(?i)command ["']device-certificate["'] not found`)

func isDeviceCertificateUnsupported(output string) bool {
	return deviceCertificateUnsupportedPattern.MatchString(output)
}

type SojuCtl struct {
	Path    string
	Config  string
	Timeout time.Duration
}

func (s *SojuCtl) Run(parent context.Context, args []string) (string, error) {
	if s.Path == "" {
		return "", errors.New("sojuctl path is empty")
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	argv := make([]string, 0, len(args)+2)
	if s.Config != "" {
		argv = append(argv, "-config", s.Config)
	}
	argv = append(argv, args...)
	// #nosec G204 -- the executable is resolved from an explicit local setting,
	// arguments are passed as an argv vector, and no command shell is invoked.
	command := exec.CommandContext(ctx, s.Path, argv...)
	output, err := command.CombinedOutput()
	text := string(output)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return text, fmt.Errorf("sojuctl timed out after %s", timeout)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return text, errors.New("sojuctl operation cancelled")
	}
	if err != nil {
		return text, fmt.Errorf("sojuctl failed: %w", err)
	}
	return text, nil
}

func formatSojuCtlCommand(config string, args []string) string {
	parts := []string{"sojuctl"}
	if config != "" {
		parts = append(parts, "-config", config)
	}
	parts = append(parts, args...)
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		quoted = append(quoted, quoteDisplayArg(part))
	}
	return strings.Join(quoted, " ")
}

func quoteDisplayArg(value string) string {
	if value != "" && safeDisplayArg.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func sojuCtlFailureHint(output string) string {
	match := adminSocketPermissionPattern.FindStringSubmatch(output)
	if len(match) == 2 {
		socketPath := strings.TrimSpace(match[1])
		if socketPath == "" {
			return ""
		}
		return strings.Join([]string{
			"ADMIN SOCKET ACCESS DENIED",
			"The current Linux account cannot write " + socketPath + ".",
			"Run the first-time setup wizard from the repository, then retry:",
			"  ./scripts/setup.sh",
			"Do not make the admin socket world-writable.",
		}, "\n")
	}
	if clientCertificateDisabledPattern.MatchString(output) {
		return strings.Join([]string{
			"CLIENT DEVICE CERTIFICATES ARE DISABLED",
			"These authenticate downstream IRC clients; they are not the Soju server's TLS certificate.",
			"Enable client-cert-auth true in the Soju config and restart Soju before registering devices.",
			"Use Server TLS certificate in the TUI to inspect the certificate presented by this host.",
		}, "\n")
	}
	if isDeviceCertificateUnsupported(output) {
		return strings.Join([]string{
			"CLIENT DEVICE CERTIFICATES ARE NOT SUPPORTED BY THIS SOJU SERVER",
			"The running Soju version does not expose the device-certificate command.",
			"Upgrade Soju to use downstream client-certificate administration.",
			"This is unrelated to the host TLS certificate; use Server TLS certificate to inspect it.",
		}, "\n")
	}
	return ""
}
