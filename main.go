package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"
	"time"
)

var version = "dev"

func main() {
	configFlag := flag.String("config", os.Getenv("SOJU_CONFIG"), "soju config path (contains the unix+admin:// listener)")
	sojuctlFlag := flag.String("sojuctl", os.Getenv("SOJUCTL"), "path to sojuctl")
	profileFlag := flag.String("profile", os.Getenv("SOJU_TUI_PROFILE"), "saved local admin profile path")
	timeoutFlag := flag.Duration("timeout", 30*time.Second, "maximum duration for each sojuctl operation")
	setupFlag := flag.Bool("setup", false, "recreate the local admin profile")
	acceptConfigFlag := flag.Bool("accept-config", false, "accept discovered first-run settings without prompting")
	versionFlag := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(version)
		return
	}

	profilePath := defaultAdminProfilePath(*profileFlag)
	profileExists, err := regularFileExists(profilePath)
	if err != nil {
		fatal(err)
	}
	savedProfile, err := loadAdminProfile(profilePath)
	if err != nil {
		fatal(err)
	}
	if *setupFlag {
		savedProfile = AdminProfile{}
	}

	configPath := *configFlag
	if configPath == "" {
		configPath = savedProfile.ConfigPath
	}
	if configPath == "" {
		configPath = "/etc/soju/config"
	}
	sojuctlPath, err := resolveSojuCtl(*sojuctlFlag, savedProfile.SojuCtl)
	if err != nil {
		fatal(err)
	}

	profile := AdminProfile{ConfigPath: configPath, SojuCtl: sojuctlPath}
	if !profileExists || *setupFlag {
		info, err := readSojuConfig(configPath)
		if err != nil {
			fatal(err)
		}
		if !*acceptConfigFlag {
			accepted, err := confirmAdminProfile(profile, info, os.Stdin, os.Stdout)
			if err != nil {
				fatal(err)
			}
			if !accepted {
				fatal(errors.New("first-run configuration was not accepted; no profile was saved"))
			}
		}
	}
	if err := saveAdminProfile(profilePath, profile); err != nil {
		fmt.Fprintf(os.Stderr, "soju-tui: warning: cannot save admin profile: %v\n", err)
	}

	backend := &SojuCtl{Path: sojuctlPath, Config: configPath, Timeout: *timeoutFlag}
	app := newAdminApp(backend)
	if err := runUI(app); err != nil {
		app.close()
		fatal(err)
	}
}

func regularFileExists(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect admin profile %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("admin profile %s is not a regular file", path)
	}
	return true, nil
}

func confirmAdminProfile(profile AdminProfile, info SojuConfigInfo, input io.Reader, output io.Writer) (bool, error) {
	localUser := "(unknown)"
	if current, err := user.Current(); err == nil && current.Username != "" {
		localUser = current.Username
	}
	fmt.Fprintln(output, "soju-tui first-run configuration review")
	fmt.Fprintln(output)
	fmt.Fprintf(output, "  Local Linux user:   %s\n", localUser)
	fmt.Fprintf(output, "  Soju config:        %s\n", profile.ConfigPath)
	fmt.Fprintf(output, "  Soju hostname:      %s\n", displayUnknown(info.Hostname))
	fmt.Fprintf(output, "  Soju title:         %s\n", displayUnknown(info.Title))
	fmt.Fprintf(output, "  Admin socket:       %s\n", displayUnknown(info.AdminSocket))
	fmt.Fprintf(output, "  Server TLS cert:    %s\n", displayUnknown(info.TLSCertPath))
	fmt.Fprintf(output, "  sojuctl:            %s\n", profile.SojuCtl)
	fmt.Fprintln(output, "\nNo IRC password, Soju password, or private key will be saved.")
	fmt.Fprint(output, "Save these settings and start the TUI? [y/N] ")
	answer, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

func fatal(err error) {
	if err == nil {
		err = errors.New("unknown error")
	}
	fmt.Fprintln(os.Stderr, "soju-tui:", err)
	os.Exit(1)
}
