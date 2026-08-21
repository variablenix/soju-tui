package main

import (
	"context"
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
	Kind   string
	Title  string
	Fields []AdminField
	Cursor int
}

type AdminOperation struct {
	Summary               string
	Args                  []string
	Refresh               []string
	Mutating              bool
	Secrets               []string
	Preview               string
	NeedsSojuConfirmation bool
	ConfirmPhrase         string
	FollowUpKind          string
	TargetUser            string
	TargetNetwork         string
	CapabilityUser        string
	Quiet                 bool
}

type AdminConfirmation struct {
	Operation AdminOperation
	Input     []rune
}

type AdminState struct {
	View          AdminView
	Cursor        int
	Output        []string
	Form          *AdminForm
	Confirm       *AdminConfirmation
	ExitConfirm   bool
	Busy          bool
	LastRefresh   []string
	LastOperation *AdminOperation
	Capabilities  AdminCapabilities
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
}

func newAdminApp(backend *SojuCtl) *App {
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{
		ctx:     ctx,
		cancel:  cancel,
		backend: backend,
		admin:   AdminState{View: adminOutput},
		results: make(chan adminResult, 16),
		done:    make(chan struct{}),
		status:  "checking sojuctl admin socket...",
	}
	op := makeAdminOperation(backend.Config, "Detect global Soju capabilities", []string{"help"}, nil, false, nil)
	op.FollowUpKind = "startup-global-help"
	op.Quiet = true
	a.requestOperation(op)
	return a
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
		a.admin.Output = trimOutput(a.admin.Output)
	}
	a.admin.Busy = true
	a.admin.View = adminOutput
	a.admin.LastOperation = &op
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
	output := redactText(result.Output, result.Operation.Secrets)
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
	if result.Err != nil {
		a.admin.Output = append(a.admin.Output, "ERROR: "+redactText(result.Err.Error(), result.Operation.Secrets))
		if hint := sojuCtlFailureHint(output); hint != "" {
			a.admin.Output = append(a.admin.Output, hint)
		}
		a.setStatusLocked("sojuctl operation failed", 0)
	} else {
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
			if args, username, ok := parseUserDeleteConfirmation(output); ok {
				followUp := makeAdminOperation(a.backend.Config, "Confirm deletion of user "+username, args, []string{"user", "status"}, true, nil)
				followUp.ConfirmPhrase = "DELETE USER " + username
				a.admin.Confirm = &AdminConfirmation{Operation: followUp}
				a.admin.View = adminOutput
				a.setStatusLocked("soju requires a second deletion confirmation; type the displayed phrase", 0)
				a.admin.Output = trimOutput(a.admin.Output)
				return
			}
		}
		if len(result.Operation.Refresh) > 0 {
			a.admin.LastRefresh = append([]string(nil), result.Operation.Refresh...)
		}
		a.setStatusLocked("operation completed", 4*time.Second)
	}
	a.admin.Output = trimOutput(a.admin.Output)
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
