package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
)

var version = "dev"

func main() {
	configFlag := flag.String("config", os.Getenv("SOJU_CONFIG"), "soju config path (contains the unix+admin:// listener)")
	sojuctlFlag := flag.String("sojuctl", os.Getenv("SOJUCTL"), "path to sojuctl")
	profileFlag := flag.String("profile", os.Getenv("SOJU_TUI_PROFILE"), "saved local admin profile path")
	timeoutFlag := flag.Duration("timeout", 30*time.Second, "maximum duration for each sojuctl operation")
	setupFlag := flag.Bool("setup", false, "recreate the local admin profile")
	versionFlag := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *versionFlag {
		fmt.Println(version)
		return
	}

	profilePath := defaultAdminProfilePath(*profileFlag)
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

func fatal(err error) {
	if err == nil {
		err = errors.New("unknown error")
	}
	fmt.Fprintln(os.Stderr, "soju-tui:", err)
	os.Exit(1)
}
