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
	Busy          bool
	LastRefresh   []string
	LastOperation *AdminOperation
}

type adminResult struct {
	Operation AdminOperation
	Output    string
	Err       error
}

type App struct {
	mu       sync.RWMutex
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
	a := &App{
		backend: backend,
		admin:   AdminState{View: adminOutput},
		results: make(chan adminResult, 16),
		done:    make(chan struct{}),
		status:  "checking sojuctl admin socket...",
	}
	a.requestOperation(AdminOperation{
		Summary: "Server status",
		Args:    []string{"server", "status"},
		Refresh: []string{"server", "status"},
	})
	return a
}

func (a *App) close() {
	a.closeOne.Do(func() {
		a.mu.Lock()
		a.quit = true
		a.mu.Unlock()
		close(a.done)
	})
}

func (a *App) requestOperation(op AdminOperation) {
	a.mu.Lock()
	if a.admin.Busy {
		a.mu.Unlock()
		return
	}
	a.admin.Output = append(a.admin.Output, "> "+op.Preview)
	a.admin.Output = trimOutput(a.admin.Output)
	a.admin.Busy = true
	a.admin.View = adminOutput
	a.admin.LastOperation = &op
	a.setStatusLocked("running sojuctl...", 0)
	a.mu.Unlock()

	go func() {
		output, err := a.backend.Run(context.Background(), op.Args)
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
	if strings.TrimSpace(output) != "" {
		a.admin.Output = append(a.admin.Output, strings.TrimRight(output, "\n"))
	}
	if result.Err != nil {
		a.admin.Output = append(a.admin.Output, "ERROR: "+redactText(result.Err.Error(), result.Operation.Secrets))
		a.setStatusLocked("sojuctl operation failed", 0)
	} else {
		if result.Operation.NeedsSojuConfirmation {
			if args, username, ok := parseUserDeleteConfirmation(output); ok {
				followUp := makeAdminOperation(a.backend.Config, "Confirm deletion of user "+username, args, []string{"user", "status"}, true, nil)
				a.admin.Confirm = &AdminConfirmation{Operation: followUp}
				a.admin.View = adminOutput
				a.setStatusLocked("soju requires a second deletion confirmation; review it and press y", 0)
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
