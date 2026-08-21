package main

import (
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestAdminUIUsesAdministrationIdentityAndResponsiveBrand(t *testing.T) {
	app := newTestApp()
	defer app.close()
	app.backend = &SojuCtl{Path: "/usr/bin/sojuctl", Config: "/etc/soju/config"}
	app.admin.Output = []string{"2/2 users, 5 networks, 25 channels"}
	app.status = "ready"

	large := renderAdminScreen(t, app, 120, 40)
	for _, expected := range []string{
		adminSidebarSubtitle,
		"SOJU-TUI ADMINISTRATION CONSOLE",
		"____    ___        _    _   _",
		"| |_| |  | |_| |  | |_| |",
		`\_____\_____\_____\_____/`,
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
	if !strings.Contains(help, "SOJU-TUI HELP & DOCUMENTATION") || !strings.Contains(help, "Up/Down") {
		t.Fatalf("help view did not render from the beginning:\n%s", help)
	}
	app.admin.HelpScroll = len(sojuTUIHelp()) - 5
	help = renderAdminScreen(t, app, 80, 24)
	if !strings.Contains(help, "https://soju.im/") || !strings.Contains(help, "does not open") {
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
			if gap := len(left) - len(trimmed); gap < 5 {
				t.Fatalf("brand row %d gap = %d, want at least 5 columns", index, gap)
			}
		}
	}
	if width := adminBrandWidth(fullAdminBrand); width > 57 {
		t.Fatalf("full brand width = %d, expected it to fit a 57-column pane", width)
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
