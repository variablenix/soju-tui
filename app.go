package main

import (
	"context"
	"os/user"
	"strings"
	"sync"
	"time"
)

type AdminView string

const (
	adminDashboard AdminView = "dashboard"
	adminOutput    AdminView = "output"
	adminForm      AdminView = "form"
)

type AdminField struct {
	Label    string
	Value    string
	Original string
	Secret   bool
	Required bool
	Kind     string
	Help     string
	Options  []string
}

type AdminForm struct {
	Kind       string
	TargetKind string
	Title      string
	Fields     []AdminField
	Cursor     int
}

type AdminConfirmationImpact uint8

const (
	adminConfirmationChange AdminConfirmationImpact = iota
	adminConfirmationAddition
	adminConfirmationDestructive
)

type AdminOperation struct {
	Summary               string
	Args                  []string
	Preflight             []string
	Refresh               []string
	Mutating              bool
	Secrets               []string
	Preview               string
	NeedsSojuConfirmation bool
	ConfirmPhrase         string
	ConfirmationImpact    AdminConfirmationImpact
	FollowUpKind          string
	TargetUser            string
	TargetNetwork         string
	TargetChannel         string
	CapabilityUser        string
	FormKind              string
	Quiet                 bool
	CertificateState      string
	CertificateReport     string
	CompatibilityFallback []string
	FallbackPreview       string
}

type AdminConfirmation struct {
	Operation AdminOperation
	Input     []rune
}

type AdminState struct {
	View             AdminView
	Cursor           int
	Output           []string
	OutputScroll     int
	Form             *AdminForm
	Confirm          *AdminConfirmation
	ExitConfirm      bool
	HelpOpen         bool
	HelpScroll       int
	Busy             bool
	LastRefresh      []string
	PendingOperation *AdminOperation
	PendingBatch     []AdminOperation
	BatchFailures    int
	Capabilities     AdminCapabilities
	Users            []string
}

type AdminCapabilities struct {
	Known    bool
	Commands map[string]bool
}

type adminResult struct {
	Operation AdminOperation
	Output    string
	Err       error
}

type App struct {
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	backend  *SojuCtl
	admin    AdminState
	results  chan adminResult
	done     chan struct{}
	closeOne sync.Once
	quit     bool
	status   string
	statusAt time.Time
	// localUsername is an in-memory convenience used to prefer the matching
	// Soju account in the "Change my password" selector. It is not persisted
	// and does not establish the Soju administrator identity.
	localUsername string
}

func newAdminApp(backend *SojuCtl) *App {
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{
		ctx:           ctx,
		cancel:        cancel,
		backend:       backend,
		admin:         AdminState{View: adminOutput},
		results:       make(chan adminResult, 16),
		done:          make(chan struct{}),
		status:        "checking sojuctl admin socket...",
		localUsername: currentLocalUsername(),
	}
	op := makeAdminOperation(backend.Config, "Detect global Soju capabilities", []string{"help"}, nil, false, nil)
	op.FollowUpKind = "startup-global-help"
	op.Quiet = true
	a.requestOperation(op)
	return a
}

func currentLocalUsername() string {
	current, err := user.Current()
	if err != nil {
		return ""
	}
	return current.Username
}

func (a *App) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closeLocked()
}

func (a *App) closeLocked() {
	a.quit = true
	if a.cancel != nil {
		a.cancel()
	}
	a.closeOne.Do(func() { close(a.done) })
}

func (a *App) requestOperation(op AdminOperation) {
	a.mu.Lock()
	if a.admin.Busy {
		a.mu.Unlock()
		return
	}
	if !op.Quiet {
		a.admin.Output = append(a.admin.Output, "> "+op.Preview)
		a.admin.OutputScroll = 0
		a.admin.Output = trimOutput(a.admin.Output)
	}
	a.admin.Busy = true
	a.admin.View = adminOutput
	a.setStatusLocked("running sojuctl...", 0)
	a.mu.Unlock()

	go func() {
		output, err := a.backend.Run(a.ctx, op.Args)
		select {
		case a.results <- adminResult{Operation: op, Output: output, Err: err}:
		case <-a.done:
		}
	}()
}

func (a *App) processResult(result adminResult) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.admin.Busy = false
	if !result.Operation.Quiet {
		a.admin.OutputScroll = 0
	}
	output := redactText(result.Output, result.Operation.Secrets)
	if result.Err == nil && isSASLStatusArgs(result.Operation.Args) {
		output = clarifySASLStatusOutput(output)
	}
	if !result.Operation.Quiet && strings.TrimSpace(output) != "" {
		a.admin.Output = append(a.admin.Output, strings.TrimRight(output, "\n"))
	}
	if result.Operation.FollowUpKind == "startup-global-help" {
		if result.Err == nil {
			a.admin.Capabilities.Commands = parseAdminCommandHelp(output)
			op := makeAdminOperation(a.backend.Config, "Find a Soju user for capability detection", []string{"user", "status"}, nil, false, nil)
			op.FollowUpKind = "startup-user-status"
			op.Quiet = true
			a.continueStartupLocked(op)
			return
		}
		// Capability detection must fail closed: never advertise commands that
		// the running server has not explicitly reported as available.
		a.admin.Capabilities = AdminCapabilities{Known: true, Commands: make(map[string]bool)}
		a.continueStartupLocked(a.serverStatusOperation())
		return
	}
	if result.Operation.FollowUpKind == "startup-user-status" {
		if result.Err == nil {
			a.admin.Users = parseSojuUsernames(output)
			if username := parseFirstSojuUsername(output); username != "" {
				op := makeAdminOperation(a.backend.Config, "Detect per-user Soju capabilities", []string{"user", "run", username, "help"}, nil, false, nil)
				op.FollowUpKind = "startup-user-help"
				op.Quiet = true
				a.continueStartupLocked(op)
				return
			}
		}
		// With no users, expose only global and local actions. Creating the
		// first user triggers another capability query without a restart.
		a.admin.Capabilities.Known = true
		a.continueStartupLocked(a.serverStatusOperation())
		return
	}
	if result.Operation.FollowUpKind == "startup-user-help" {
		if result.Err == nil {
			if a.admin.Capabilities.Commands == nil {
				a.admin.Capabilities.Commands = make(map[string]bool)
			}
			for command := range parseAdminCommandHelp(output) {
				a.admin.Capabilities.Commands[command] = true
			}
			a.admin.Capabilities.Known = true
		} else {
			// Preserve the known global command set. Falling back to an unknown
			// capability set would make every user-scoped action visible.
			a.admin.Capabilities.Known = true
		}
		a.continueStartupLocked(a.serverStatusOperation())
		return
	}
	if result.Operation.FollowUpKind == "post-create-user-help" {
		if result.Err == nil {
			if a.admin.Capabilities.Commands == nil {
				a.admin.Capabilities.Commands = make(map[string]bool)
			}
			for command := range parseAdminCommandHelp(output) {
				a.admin.Capabilities.Commands[command] = true
			}
			a.admin.Capabilities.Known = true
		}
		op := makeAdminOperation(a.backend.Config, "Refresh users", []string{"user", "status"}, []string{"user", "status"}, false, nil)
		a.continueStartupLocked(op)
		return
	}
	if result.Operation.FollowUpKind == "open-user-form" && result.Err == nil {
		users := parseSojuUsernames(output)
		a.admin.Users = users
		if len(users) == 0 {
			a.admin.Output = append(a.admin.Output, "No Soju users exist. Create a user before opening this action.")
			a.admin.View = adminOutput
			a.setStatusLocked("no Soju users available", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		if err := a.adminOpenFormWithUsersLocked(result.Operation.FormKind, users); err != nil {
			a.admin.Output = append(a.admin.Output, "ERROR: "+err.Error())
			a.admin.View = adminOutput
			a.setStatusLocked("could not open user-targeted action", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		return
	}
	if result.Operation.FollowUpKind == "open-network-form" {
		if result.Err != nil {
			a.admin.Output = append(a.admin.Output, "ERROR: could not discover saved networks: "+redactText(result.Err.Error(), result.Operation.Secrets))
			a.admin.View = adminOutput
			a.setStatusLocked("network discovery failed", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		if err := a.adminOpenNetworkFormLocked(result.Operation.FormKind, result.Operation.TargetUser, output); err != nil {
			a.admin.Output = append(a.admin.Output, "ERROR: "+err.Error())
			a.admin.View = adminOutput
			a.setStatusLocked("could not open network-targeted action", 0)
			a.admin.Output = trimOutput(a.admin.Output)
		}
		return
	}
	if result.Operation.FollowUpKind == "open-channel-form" {
		if result.Err != nil {
			a.admin.Output = append(a.admin.Output, "ERROR: could not discover saved channels: "+redactText(result.Err.Error(), result.Operation.Secrets))
			a.admin.View = adminOutput
			a.setStatusLocked("channel discovery failed", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		if err := a.adminOpenChannelFormLocked(result.Operation.FormKind, result.Operation.TargetUser, result.Operation.TargetNetwork, output); err != nil {
			a.admin.Output = append(a.admin.Output, "ERROR: "+err.Error())
			a.admin.View = adminOutput
			a.setStatusLocked("could not open channel-targeted action", 0)
			a.admin.Output = trimOutput(a.admin.Output)
		}
		return
	}
	if result.Operation.FollowUpKind == "channel-update" {
		if result.Err != nil {
			a.admin.Output = append(a.admin.Output, "ERROR: could not load channel state: "+redactText(result.Err.Error(), result.Operation.Secrets))
			a.setStatusLocked("channel state lookup failed", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		channel, err := findChannelStatus(output, result.Operation.TargetChannel)
		if err != nil {
			a.admin.Output = append(a.admin.Output, "ERROR: "+err.Error())
			a.setStatusLocked("could not load channel settings", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		a.admin.Form = newChannelUpdateForm(result.Operation.TargetUser, result.Operation.TargetNetwork, channel)
		a.admin.View = adminForm
		a.setStatusLocked("current detached state loaded; blank undisclosed settings keep their values", 0)
		return
	}
	if result.Operation.FollowUpKind == "cert-fingerprint-batch" {
		a.admin.Output = append(a.admin.Output, "CERTFP NETWORK: "+result.Operation.TargetNetwork)
		switch {
		case result.Err == nil:
			if strings.TrimSpace(output) == "" {
				a.admin.Output = append(a.admin.Output, "  No fingerprint data returned")
			} else {
				a.admin.Output = append(a.admin.Output, strings.TrimSpace(output))
			}
		case isCertFPNotConfigured(output):
			a.admin.Output = append(a.admin.Output, "  Not configured")
		default:
			a.admin.BatchFailures++
			a.admin.Output = append(a.admin.Output, "ERROR: "+redactText(result.Err.Error(), result.Operation.Secrets))
			if strings.TrimSpace(output) != "" {
				a.admin.Output = append(a.admin.Output, strings.TrimSpace(output))
			}
		}
		a.admin.Output = append(a.admin.Output, "────────────────────────────────")
		a.admin.Output = trimOutput(a.admin.Output)
		if len(a.admin.PendingBatch) > 0 {
			next := a.admin.PendingBatch[0]
			a.admin.PendingBatch = a.admin.PendingBatch[1:]
			a.mu.Unlock()
			a.requestOperation(next)
			a.mu.Lock()
			a.setStatusLocked("inspecting upstream CertFP across saved networks...", 0)
			return
		}
		if a.admin.BatchFailures > 0 {
			a.setStatusLocked("CertFP inspection completed with errors", 0)
		} else {
			a.setStatusLocked("CertFP inspection completed for all saved networks", 4*time.Second)
		}
		return
	}
	if result.Operation.FollowUpKind == "cert-generate-preflight" {
		planned := a.admin.PendingOperation
		a.admin.PendingOperation = nil
		if planned == nil {
			a.admin.Output = append(a.admin.Output, "ERROR: certificate generation preflight lost its pending operation")
			a.setStatusLocked("certificate generation cancelled safely", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		if result.Err == nil && isCertificateFingerprintReport(output) {
			planned.CertificateState = "existing"
			planned.ConfirmationImpact = adminConfirmationDestructive
			planned.CertificateReport = normalizeCertificateReport(output)
			a.admin.Output = append(a.admin.Output,
				"EXISTING UPSTREAM SASL CERTIFICATE FOUND",
				strings.TrimSpace(output),
				"The following confirmation replaces only this user's upstream IRC CertFP certificate. The Soju host TLS/Let's Encrypt files are not touched.",
			)
			planned.ConfirmPhrase = "REPLACE EXISTING UPSTREAM CERTIFICATE"
			a.admin.Confirm = &AdminConfirmation{Operation: *planned}
			a.admin.View = adminOutput
			a.setStatusLocked("existing CertFP found; review fingerprints and type the replacement phrase", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		if isCertFPNotConfigured(output) {
			planned.CertificateState = "absent"
			planned.ConfirmationImpact = adminConfirmationAddition
			planned.CertificateReport = ""
			a.admin.Output = append(a.admin.Output,
				"No existing upstream SASL CertFP certificate was found for this user and network.",
				"Generation affects only upstream IRC authentication. The Soju host TLS/Let's Encrypt files are not touched.",
			)
			planned.ConfirmPhrase = "GENERATE UPSTREAM CERTIFICATE"
			a.admin.Confirm = &AdminConfirmation{Operation: *planned}
			a.admin.View = adminOutput
			a.setStatusLocked("no existing CertFP found; type the generation phrase to continue", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		errorText := "unexpected fingerprint response"
		if result.Err != nil {
			errorText = redactText(result.Err.Error(), result.Operation.Secrets)
		}
		a.admin.Output = append(a.admin.Output, "ERROR: could not safely inspect the existing upstream certificate: "+errorText)
		if strings.TrimSpace(output) != "" {
			a.admin.Output = append(a.admin.Output, strings.TrimSpace(output))
		}
		a.setStatusLocked("certificate generation blocked because preflight failed", 0)
		a.admin.Output = trimOutput(a.admin.Output)
		return
	}
	if result.Operation.FollowUpKind == "cert-generate-guard" {
		planned := a.admin.PendingOperation
		a.admin.PendingOperation = nil
		if planned == nil {
			a.admin.Output = append(a.admin.Output, "ERROR: certificate generation safety check lost its pending operation")
			a.setStatusLocked("certificate generation cancelled safely", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		if !certificateStateMatches(*planned, output, result.Err) {
			a.admin.Output = append(a.admin.Output,
				"ERROR: upstream CertFP state changed after it was reviewed; generation was blocked.",
				"Reopen the action to inspect the current certificate state before trying again.",
			)
			if strings.TrimSpace(output) != "" {
				a.admin.Output = append(a.admin.Output, strings.TrimSpace(output))
			}
			a.setStatusLocked("certificate generation blocked because state changed", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		planned.CertificateState = ""
		planned.CertificateReport = ""
		a.mu.Unlock()
		a.requestOperation(*planned)
		a.mu.Lock()
		a.setStatusLocked("certificate state revalidated; running confirmed generation...", 0)
		return
	}
	if result.Err != nil && len(result.Operation.CompatibilityFallback) > 0 && isUnixSchemeCompatibilityError(output) {
		fallback := result.Operation
		fallback.Args = append([]string(nil), result.Operation.CompatibilityFallback...)
		fallback.Preview = result.Operation.FallbackPreview
		fallback.CompatibilityFallback = nil
		fallback.FallbackPreview = ""
		a.admin.Output = append(a.admin.Output,
			"Soju rejected the stable unix:// spelling; retrying the equivalent irc+unix:// spelling advertised by this server.",
		)
		a.mu.Unlock()
		a.requestOperation(fallback)
		a.mu.Lock()
		a.setStatusLocked("retrying with this Soju version's Unix-address spelling...", 0)
		return
	}
	if result.Err != nil {
		a.admin.Output = append(a.admin.Output, "ERROR: "+redactText(result.Err.Error(), result.Operation.Secrets))
		if hint := sojuCtlFailureHint(output); hint != "" {
			a.admin.Output = append(a.admin.Output, hint)
		}
		a.setStatusLocked("sojuctl operation failed", 0)
	} else {
		if isUserStatusArgs(result.Operation.Args) {
			a.admin.Users = parseSojuUsernames(output)
		}
		if result.Operation.CapabilityUser != "" {
			op := makeAdminOperation(a.backend.Config, "Refresh per-user Soju capabilities", []string{"user", "run", result.Operation.CapabilityUser, "help"}, nil, false, nil)
			op.FollowUpKind = "post-create-user-help"
			op.Quiet = true
			a.continueStartupLocked(op)
			return
		}
		if result.Operation.FollowUpKind == "network-update" {
			network, err := findNetworkStatus(output, result.Operation.TargetNetwork)
			if err != nil {
				a.admin.Output = append(a.admin.Output, "ERROR: "+err.Error())
				a.setStatusLocked("could not load network settings", 0)
				a.admin.Output = trimOutput(a.admin.Output)
				return
			}
			a.admin.Form = newNetworkUpdateForm(result.Operation.TargetUser, network)
			a.admin.View = adminForm
			a.setStatusLocked("existing public settings loaded; blank undisclosed fields keep their current values", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		if result.Operation.NeedsSojuConfirmation {
			if args, username, ok := parseUserDeleteConfirmation(output, result.Operation.TargetUser); ok {
				followUp := makeAdminOperation(a.backend.Config, "Confirm deletion of user "+username, args, []string{"user", "status"}, true, nil)
				followUp.ConfirmPhrase = "DELETE USER " + username
				followUp.ConfirmationImpact = adminConfirmationDestructive
				a.admin.Confirm = &AdminConfirmation{Operation: followUp}
				a.admin.View = adminOutput
				a.setStatusLocked("soju requires a second deletion confirmation; type the displayed phrase", 0)
				a.admin.Output = trimOutput(a.admin.Output)
				return
			}
			a.admin.Output = append(a.admin.Output,
				"ERROR: Soju returned success without the expected user-deletion confirmation token.",
				"Deletion was blocked; no follow-up command was sent.",
			)
			a.setStatusLocked("user deletion blocked by an unexpected confirmation response", 0)
			a.admin.Output = trimOutput(a.admin.Output)
			return
		}
		if len(result.Operation.Refresh) > 0 {
			a.admin.LastRefresh = append([]string(nil), result.Operation.Refresh...)
		}
		a.setStatusLocked("operation completed", 4*time.Second)
	}
	a.admin.Output = trimOutput(a.admin.Output)
}

func normalizeCertificateReport(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for index := range lines {
		lines[index] = strings.TrimSpace(lines[index])
	}
	return strings.Join(lines, "\n")
}

func certificateStateMatches(operation AdminOperation, output string, err error) bool {
	switch operation.CertificateState {
	case "existing":
		return err == nil && isCertificateFingerprintReport(output) && normalizeCertificateReport(output) == operation.CertificateReport
	case "absent":
		return err != nil && isCertFPNotConfigured(output)
	default:
		return false
	}
}

func (a *App) serverStatusOperation() AdminOperation {
	return makeAdminOperation(a.backend.Config, "Server status", []string{"server", "status"}, []string{"server", "status"}, false, nil)
}

func (a *App) continueStartupLocked(op AdminOperation) {
	a.setStatusLocked("detecting supported Soju administration commands...", 0)
	a.mu.Unlock()
	a.requestOperation(op)
	a.mu.Lock()
}

func trimOutput(output []string) []string {
	if len(output) <= 800 {
		return output
	}
	return output[len(output)-800:]
}

func (a *App) setStatusLocked(status string, duration time.Duration) {
	a.status = status
	if duration > 0 {
		a.statusAt = time.Now().Add(duration)
	} else {
		a.statusAt = time.Time{}
	}
}

func (a *App) currentStatusLocked() string {
	if !a.statusAt.IsZero() && time.Now().After(a.statusAt) {
		return "ready"
	}
	return a.status
}
