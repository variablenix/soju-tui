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
		"|  SOJU  |",
	} {
		if !strings.Contains(large, expected) {
			t.Fatalf("large administration screen is missing %q:\n%s", expected, large)
		}
	}
	if strings.Contains(large, "No IRC chat connection") {
		t.Fatalf("obsolete IRC-client status remains visible:\n%s", large)
	}

	compact := renderAdminScreen(t, app, 64, 18)
	if !strings.Contains(compact, "SOJU-TUI") || !strings.Contains(compact, "| SOJU |") {
		t.Fatalf("compact administration brand was not rendered:\n%s", compact)
	}

	short := renderAdminScreen(t, app, 40, 10)
	if strings.Contains(short, "SOJU-TUI") {
		t.Fatalf("brand should be omitted when it would crowd the output:\n%s", short)
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
