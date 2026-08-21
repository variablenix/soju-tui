package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

var version = "dev"

func main() {
	var tlsEnabled = envBool("SOJU_TLS", true)
	var (
		server          = flag.String("server", os.Getenv("SOJU_SERVER"), "soju address, e.g. irc.example.org:6697")
		username        = flag.String("username", os.Getenv("SOJU_USERNAME"), "soju username; SOJU_USERNAME is also supported")
		password        = flag.String("password", os.Getenv("SOJU_PASSWORD"), "soju password (prefer the interactive prompt or SOJU_PASSWORD)")
		nick            = flag.String("nick", os.Getenv("SOJU_NICK"), "IRC nickname (defaults to the username)")
		realname        = flag.String("realname", envString("SOJU_REALNAME", "soju-tui"), "IRC real name")
		clientName      = flag.String("client", envString("SOJU_CLIENT", "soju-tui"), "client name used by soju for per-client history")
		tlsServerName   = flag.String("tls-server-name", os.Getenv("SOJU_TLS_SERVER_NAME"), "TLS server name override")
		insecureSkipTLS = flag.Bool("insecure-skip-verify", false, "skip TLS certificate verification (unsafe; useful for self-signed certificates)")
		networkFilter   = flag.String("network", os.Getenv("SOJU_NETWORK"), "only open the named soju network")
		profilePath     = flag.String("profile", os.Getenv("SOJU_TUI_PROFILE"), "saved profile path")
		sojuConfigPath  = flag.String("soju-config", envString("SOJU_CONFIG", "/etc/soju/config"), "soju daemon config to auto-detect")
		setup           = flag.Bool("setup", false, "run the connection setup wizard")
	)
	flag.BoolVar(&tlsEnabled, "tls", tlsEnabled, "use TLS for the soju connection")
	flag.Parse()

	profileFile := defaultProfilePath(*profilePath)
	saved, err := loadSavedProfile(profileFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "soju-tui: warning: cannot load profile: %v\n", err)
	}
	discovered, err := discoverSojuConfig(*sojuConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "soju-tui: warning: cannot inspect %s: %v\n", *sojuConfigPath, err)
	}

	serverExplicit := os.Getenv("SOJU_SERVER") != "" || wasFlagSet("server")
	tlsExplicit := os.Getenv("SOJU_TLS") != "" || wasFlagSet("tls")
	usernameExplicit := os.Getenv("SOJU_USERNAME") != "" || wasFlagSet("username")
	if !*setup {
		if !serverExplicit && saved.Server != "" {
			*server = saved.Server
			if !tlsExplicit && saved.TLS != nil {
				tlsEnabled = *saved.TLS
			}
			if *tlsServerName == "" {
				*tlsServerName = saved.TLSServerName
			}
		} else if !serverExplicit && discovered.Server != "" && confirmDetectedConfig(discovered, *sojuConfigPath) {
			*server = discovered.Server
			if !tlsExplicit {
				tlsEnabled = discovered.TLS
			}
			if *tlsServerName == "" {
				*tlsServerName = discovered.TLSServerName
			}
		}
		if !usernameExplicit && saved.Username != "" {
			*username = saved.Username
		}
	}
	if *server == "" {
		var err error
		*server, tlsEnabled, *tlsServerName, err = runSetupWizard(tlsEnabled)
		if err != nil {
			fatal(err)
		}
	}

	if *username == "" {
		defaultUsername := os.Getenv("USER")
		*username = promptLineDefault("soju username", defaultUsername)
	}
	if *username == "" {
		fatal(errors.New("a soju username is required"))
	}
	if *password == "" {
		var err error
		*password, err = promptPassword("soju password: ")
		if err != nil {
			fatal(err)
		}
	}
	if *nick == "" {
		*nick = *username
		if slash := strings.IndexAny(*nick, "/@"); slash > 0 {
			*nick = (*nick)[:slash]
		}
	}
	*server = normalizeAddress(*server, tlsEnabled)
	profileTLS := tlsEnabled
	profile := SavedProfile{
		Server:        *server,
		TLS:           &profileTLS,
		TLSServerName: *tlsServerName,
		Username:      *username,
		Nick:          *nick,
		Realname:      *realname,
		ClientName:    *clientName,
		NetworkFilter: *networkFilter,
	}
	if err := saveProfile(profileFile, profile); err != nil {
		fmt.Fprintf(os.Stderr, "soju-tui: warning: cannot save profile: %v\n", err)
	} else if *setup || (!serverExplicit && saved.Server == "") {
		fmt.Fprintf(os.Stderr, "saved connection profile to %s\n", profileFile)
	}
	if *insecureSkipTLS && tlsEnabled {
		fmt.Fprintln(os.Stderr, "warning: TLS certificate verification is disabled")
	}

	app := newApp(AppConfig{
		Server:          *server,
		TLS:             tlsEnabled,
		TLSServerName:   *tlsServerName,
		InsecureSkipTLS: *insecureSkipTLS,
		Username:        *username,
		Password:        *password,
		Nick:            *nick,
		Realname:        *realname,
		ClientName:      *clientName,
		NetworkFilter:   *networkFilter,
	})
	if err := app.start(); err != nil {
		fatal(err)
	}
	if err := runUI(app); err != nil {
		app.close()
		fatal(err)
	}
}

func promptLine(prompt string) string {
	fmt.Fprint(os.Stderr, prompt)
	line, _ := stdinReader.ReadString('\n')
	return strings.TrimSpace(line)
}

var stdinReader = bufio.NewReader(os.Stdin)

func promptLineDefault(label, fallback string) string {
	prompt := label
	if fallback != "" {
		prompt += " [" + fallback + "]"
	}
	prompt += ": "
	value := promptLine(prompt)
	if value == "" {
		return fallback
	}
	return value
}

func confirmDetectedConfig(discovered discoveredSojuConfig, path string) bool {
	tlsText := "plain"
	if discovered.TLS {
		tlsText = "TLS"
	}
	fmt.Fprintf(os.Stderr, "Detected %s soju listener %s from %s\n", tlsText, discovered.Server, path)
	if discovered.TLSServerName != "" {
		fmt.Fprintf(os.Stderr, "Detected TLS hostname: %s\n", discovered.TLSServerName)
	}
	answer := strings.ToLower(promptLine("Use these settings? [Y/n]: "))
	return answer == "" || answer == "y" || answer == "yes"
}

func runSetupWizard(defaultTLS bool) (server string, tlsEnabled bool, tlsServerName string, err error) {
	fmt.Fprintln(os.Stderr, "soju-tui first-run setup")
	host := promptLineDefault("soju server address", "127.0.0.1")
	if host == "" {
		return "", false, "", errors.New("a soju server address is required")
	}
	tlsAnswer := strings.ToLower(promptLineDefault("use TLS? (y/n)", map[bool]string{true: "y", false: "n"}[defaultTLS]))
	tlsEnabled = tlsAnswer != "n" && tlsAnswer != "no" && tlsAnswer != "false"
	defaultPort := "6667"
	if tlsEnabled {
		defaultPort = "6697"
	}
	port := promptLineDefault("soju port", defaultPort)
	if _, _, splitErr := net.SplitHostPort(net.JoinHostPort(host, port)); splitErr != nil {
		return "", false, "", fmt.Errorf("invalid server address or port: %w", splitErr)
	}
	server = net.JoinHostPort(host, port)
	tlsServerName = promptLineDefault("soju TLS hostname (optional)", "")
	return server, tlsEnabled, tlsServerName, nil
}

func wasFlagSet(name string) bool {
	set := false
	flag.Visit(func(current *flag.Flag) {
		if current.Name == name {
			set = true
		}
	})
	return set
}

func promptPassword(prompt string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("SOJU_PASSWORD is not set and stdin is not an interactive terminal")
	}
	fmt.Fprint(os.Stderr, prompt)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(password), err
}

func normalizeAddress(address string, tlsEnabled bool) string {
	if address == "" {
		if tlsEnabled {
			return "127.0.0.1:6697"
		}
		return "127.0.0.1:6667"
	}
	if _, _, err := net.SplitHostPort(address); err == nil {
		return address
	}
	if strings.HasPrefix(address, "[") && strings.HasSuffix(address, "]") {
		if tlsEnabled {
			return address + ":6697"
		}
		return address + ":6667"
	}
	if strings.Count(address, ":") > 1 {
		if tlsEnabled {
			return "[" + address + "]:6697"
		}
		return "[" + address + "]:6667"
	}
	if tlsEnabled {
		return address + ":6697"
	}
	return address + ":6667"
}

func envString(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "soju-tui:", err)
	os.Exit(1)
}
