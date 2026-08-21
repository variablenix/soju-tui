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
		server          = flag.String("server", envString("SOJU_SERVER", "127.0.0.1:6697"), "soju address, e.g. irc.example.org:6697")
		username        = flag.String("username", os.Getenv("SOJU_USERNAME"), "soju username; SOJU_USERNAME is also supported")
		password        = flag.String("password", os.Getenv("SOJU_PASSWORD"), "soju password (prefer the interactive prompt or SOJU_PASSWORD)")
		nick            = flag.String("nick", os.Getenv("SOJU_NICK"), "IRC nickname (defaults to the username)")
		realname        = flag.String("realname", envString("SOJU_REALNAME", "soju-tui"), "IRC real name")
		clientName      = flag.String("client", envString("SOJU_CLIENT", "soju-tui"), "client name used by soju for per-client history")
		tlsServerName   = flag.String("tls-server-name", os.Getenv("SOJU_TLS_SERVER_NAME"), "TLS server name override")
		insecureSkipTLS = flag.Bool("insecure-skip-verify", false, "skip TLS certificate verification (unsafe; useful for self-signed certificates)")
		networkFilter   = flag.String("network", os.Getenv("SOJU_NETWORK"), "only open the named soju network")
	)
	flag.BoolVar(&tlsEnabled, "tls", tlsEnabled, "use TLS for the soju connection")
	flag.Parse()

	if *username == "" {
		*username = promptLine("soju username: ")
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
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
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
