package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
)

func handleAdminKeyEvent(app *App, event *tcell.EventKey) {
	key := ""
	switch event.Key() {
	case tcell.KeyUp:
		key = "up"
	case tcell.KeyDown:
		key = "down"
	case tcell.KeyTAB:
		key = "tab"
	case tcell.KeyBacktab:
		key = "backtab"
	case tcell.KeyEnter:
		key = "enter"
	case tcell.KeyCtrlS:
		key = "submit"
	case tcell.KeyEscape:
		key = "esc"
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		key = "backspace"
	case tcell.KeyRune:
		if event.Rune() == ' ' {
			key = "space"
		} else {
			key = string(event.Rune())
		}
	}
	app.adminHandleKey(key, event.Rune())
}

func drawAdminUI(screen tcell.Screen, app *App) {
	width, height := screen.Size()
	if width < 32 || height < 8 {
		putClipped(screen, 0, 0, width, "terminal too small for admin view", styleError)
		screen.Show()
		return
	}
	sidebarWidth := 38
	if sidebarWidth > width/2 {
		sidebarWidth = width / 3
	}
	header := fmt.Sprintf(" soju-tui  ADMIN  %s  F2 chat", app.cfg.Server)
	putClipped(screen, 0, 0, width, header, styleHeader)
	drawAdminSidebar(screen, app, sidebarWidth, height-3)
	for y := 1; y < height-2; y++ {
		putClipped(screen, sidebarWidth, y, 1, "│", styleDivider)
	}
	contentX := sidebarWidth + 2
	contentWidth := width - contentX - 1
	if app.admin.confirm != nil {
		drawAdminConfirmation(screen, app, contentX, contentWidth, 2, height-4)
	} else if app.admin.form != nil {
		drawAdminForm(screen, app, contentX, contentWidth, 2, height-4)
	} else {
		drawAdminOutput(screen, app, contentX, contentWidth, 2, height-4)
	}
	putClipped(screen, 0, height-2, width, " F2 chat  ↑↓ select  Enter open  r refresh  Esc back  Ctrl-S submit", styleMuted)
	putClipped(screen, 0, height-1, width, " "+app.currentStatusLocked(), styleInput)
	screen.HideCursor()
	screen.Show()
}

func drawAdminSidebar(screen tcell.Screen, app *App, width, height int) {
	for y := 1; y < height; y++ {
		putClipped(screen, 0, y, width, " ", styleMuted)
	}
	putClipped(screen, 1, 1, width-1, "ADMINISTRATION", styleAccent)
	putClipped(screen, 1, 2, width-1, "Read-only by default", styleMuted)
	menu := app.adminMenu()
	for i, item := range menu {
		y := i + 4
		if y >= height {
			break
		}
		style := styleMuted
		if i == app.admin.cursor && app.admin.form == nil && app.admin.confirm == nil {
			style = styleAccent.Background(tcell.ColorDarkBlue)
		}
		putClipped(screen, 0, y, width, fmt.Sprintf(" %02d %s", i+1, item), style)
	}
}

func drawAdminOutput(screen tcell.Screen, app *App, x, width, y, height int) {
	if len(app.admin.output) == 0 {
		putClipped(screen, x, y, width, "No administrative request yet.", styleMuted)
		putClipped(screen, x, y+2, width, "Choose an operation on the left. Changes always stop for confirmation.", styleNormal)
		return
	}
	lines := make([]styledLine, 0, len(app.admin.output))
	for _, text := range app.admin.output {
		style := styleNormal
		if strings.HasPrefix(text, "> ") || strings.HasPrefix(text, "confirmed:") {
			style = styleAccent
		}
		if strings.Contains(strings.ToLower(text), "failed") || strings.Contains(strings.ToLower(text), "error") {
			style = styleError
		}
		for _, piece := range wrapText(text, width) {
			lines = append(lines, styledLine{text: piece, style: style})
		}
	}
	start := len(lines) - height
	if start < 0 {
		start = 0
	}
	row := y
	for _, line := range lines[start:] {
		if row >= y+height {
			break
		}
		putClipped(screen, x, row, width, line.text, line.style)
		row++
	}
}

func drawAdminForm(screen tcell.Screen, app *App, x, width, y, height int) {
	form := app.admin.form
	if form == nil {
		return
	}
	putClipped(screen, x, y, width, form.Title, styleAccent)
	putClipped(screen, x, y+1, width, "Enter advances · Space cycles choices · Ctrl-S previews · Esc cancels", styleMuted)
	row := y + 3
	for i, field := range form.Fields {
		if row >= y+height {
			break
		}
		style := styleNormal
		if i == form.Cursor {
			style = styleAccent.Background(tcell.ColorDarkBlue)
		}
		value := field.Value
		if field.Secret && value != "" {
			value = strings.Repeat("•", len([]rune(value)))
		}
		line := fmt.Sprintf("%-20s %s", field.Label+":", value)
		putClipped(screen, x, row, width, line, style)
		if i == form.Cursor && field.Help != "" {
			putClipped(screen, x+2, row+1, width-2, field.Help, styleMuted)
		}
		row += 2
	}
}

func drawAdminConfirmation(screen tcell.Screen, app *App, x, width, y, height int) {
	confirm := app.admin.confirm
	if confirm == nil {
		return
	}
	boxWidth := width
	if boxWidth < 30 {
		boxWidth = 30
	}
	boxHeight := 9
	if boxHeight > height {
		boxHeight = height
	}
	boxY := y + (height-boxHeight)/2
	putClipped(screen, x, boxY, boxWidth, " ", styleError.Background(tcell.ColorDarkRed))
	for row := 1; row < boxHeight; row++ {
		putClipped(screen, x, boxY+row, boxWidth, " ", styleError.Background(tcell.ColorDarkRed))
	}
	putClipped(screen, x+2, boxY+1, boxWidth-4, "CONFIRM ADMINISTRATIVE CHANGE", styleError.Background(tcell.ColorDarkRed).Bold(true))
	putClipped(screen, x+2, boxY+3, boxWidth-4, confirm.Operation.Summary, styleNormal.Background(tcell.ColorDarkRed))
	for _, line := range wrapText(confirm.Operation.Preview, boxWidth-4) {
		boxY++
		if boxY >= y+height-2 {
			break
		}
		putClipped(screen, x+2, boxY+3, boxWidth-4, line, styleMuted.Background(tcell.ColorDarkRed))
	}
	putClipped(screen, x+2, y+height-2, boxWidth-4, "Press y to apply · n or Esc to cancel", styleAccent)
}
