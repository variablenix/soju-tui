package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestAdminConfirmationPresentationUsesImpactColors(t *testing.T) {
	tests := []struct {
		name       string
		impact     AdminConfirmationImpact
		title      string
		background tcell.Color
	}{
		{name: "addition", impact: adminConfirmationAddition, title: "CONFIRM ADDITION", background: tcell.ColorDarkGreen},
		{name: "change", impact: adminConfirmationChange, title: "CONFIRM ADMINISTRATIVE CHANGE", background: tcell.ColorDarkBlue},
		{name: "destructive", impact: adminConfirmationDestructive, title: "CONFIRM DESTRUCTIVE ACTION", background: tcell.ColorDarkRed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			presentation := adminConfirmationPresentationFor(test.impact)
			if presentation.title != test.title || presentation.background != test.background {
				t.Fatalf("presentation = %#v, want title %q and background %v", presentation, test.title, test.background)
			}
			_, background, _ := presentation.heading.Decompose()
			if background != test.background {
				t.Fatalf("heading background = %v, want %v", background, test.background)
			}
		})
	}
}

func TestAdminUIUsesAdministrationIdentityAndResponsiveBrand(t *testing.T) {
	app := newTestApp()
	defer app.close()
	app.backend = &SojuCtl{Path: "/usr/bin/sojuctl", Config: "/etc/soju/config"}
	app.admin.Output = []string{"2/2 users, 5 networks, 25 channels"}
	app.status = "ready"

	large := renderAdminScreen(t, app, 120, 40)
	for _, expected := range []string{
		adminSidebarSubtitle,
		"JK/WS",
		"SOJU-TUI ADMINISTRATION CONSOLE",
		"  ______     _____      ___    ___  __",
		"/\\_____ \\_ /\\  \\/\\ ,\\  \\ \\ \\\\ \\ \\  \\ \\ ,\\",
		"\\/____/> ,\\\\ \\_ \\_> \\\\ _\\_>  \\ \\ \\_ \\_> \\\\",
		"/\\__    __\\/\\  \\/\\ \\_  /\\  \\",
		"|     SOJU     |",
	} {
		if !strings.Contains(large, expected) {
			t.Fatalf("large administration screen is missing %q:\n%s", expected, large)
		}
	}
	if strings.Contains(large, "No IRC chat connection") {
		t.Fatalf("obsolete IRC-client status remains visible:\n%s", large)
	}

	compact := renderAdminScreen(t, app, 64, 18)
	if !strings.Contains(compact, "SOJU-TUI") || !strings.Contains(compact, "|   SOJU   |") {
		t.Fatalf("compact administration brand was not rendered:\n%s", compact)
	}

	short := renderAdminScreen(t, app, 40, 10)
	if strings.Contains(short, "SOJU-TUI") {
		t.Fatalf("brand should be omitted when it would crowd the output:\n%s", short)
	}

	app.admin.HelpOpen = true
	app.admin.HelpScroll = 0
	help := renderAdminScreen(t, app, 80, 24)
	if !strings.Contains(help, "SOJU-TUI HELP & DOCUMENTATION") || !strings.Contains(help, "Vim H/J/K/L") {
		t.Fatalf("help view did not render from the beginning:\n%s", help)
	}
	app.admin.HelpScroll = len(adminHelpLines(51)) + 100
	help = renderAdminScreen(t, app, 80, 24)
	if !strings.Contains(help, "UPSTREAM SOJU DOCUMENTATION") ||
		!strings.Contains(help, "https://soju.im/") ||
		!strings.Contains(help, "does not open") {
		t.Fatalf("help documentation links were not reachable by scrolling:\n%s", help)
	}
}

func TestFullAdminBrandBottleStaysOnOneAxis(t *testing.T) {
	patterns := []string{
		"____",
		"|    |",
		"|____|",
		`/      \`,
		`/          \`,
		`/            \`,
		"|              |",
		"|     SOJU     |",
		"|      TUI     |",
		"|              |",
		"|              |",
		"|______________|",
		`\____________/`,
	}
	wantCenter := -1
	for index, pattern := range patterns {
		start := strings.LastIndex(fullAdminBrand[index].text, pattern)
		if start < 0 {
			t.Fatalf("brand row %d is missing %q", index, pattern)
		}
		center := 2*start + len(pattern) - 1
		if wantCenter < 0 {
			wantCenter = center
		}
		if center != wantCenter {
			t.Fatalf("bottle row %d center = %d, want %d", index, center, wantCenter)
		}
		left := fullAdminBrand[index].text[:start]
		if strings.TrimSpace(left) != "" {
			trimmed := strings.TrimRight(left, " ")
			if gap := len(left) - len(trimmed); gap < 6 {
				t.Fatalf("brand row %d gap = %d, want at least 6 columns", index, gap)
			}
		}
	}
	if width := adminBrandWidth(fullAdminBrand); width > 62 {
		t.Fatalf("full brand width = %d, expected it to fit a 62-column pane", width)
	}
}

func TestAdminOutputScrollShowsOlderLines(t *testing.T) {
	app := newTestApp()
	defer app.close()
	app.backend = &SojuCtl{Path: "/usr/bin/sojuctl", Config: "/etc/soju/config"}
	app.admin.View = adminOutput
	for i := 1; i <= 30; i++ {
		app.admin.Output = append(app.admin.Output, fmt.Sprintf("output line %02d", i))
	}

	latest := renderAdminScreen(t, app, 100, 14)
	if !strings.Contains(latest, "output line 30") {
		t.Fatalf("latest output was not rendered:\n%s", latest)
	}

	app.admin.OutputScroll = adminOutputPageSize
	older := renderAdminScreen(t, app, 100, 14)
	if !strings.Contains(older, "output line 22") || strings.Contains(older, "output line 30") {
		t.Fatalf("output paging did not show the older window:\n%s", older)
	}
}

func renderAdminScreen(t *testing.T, app *App, width, height int) string {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	screen.Clear()
	drawAdminUI(screen, app)
	cells, renderedWidth, renderedHeight := screen.GetContents()
	var output strings.Builder
	for y := 0; y < renderedHeight; y++ {
		for x := 0; x < renderedWidth; x++ {
			cell := cells[y*renderedWidth+x]
			if len(cell.Runes) == 0 {
				output.WriteByte(' ')
				continue
			}
			output.WriteRune(cell.Runes[0])
		}
		output.WriteByte('\n')
	}
	return output.String()
}
