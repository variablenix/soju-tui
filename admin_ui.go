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
	case tcell.KeyCtrlC, tcell.KeyCtrlQ:
		key = "quit"
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
	sidebarWidth := 38
	if sidebarWidth > width/2 {
		sidebarWidth = width / 3
	}
	header := fmt.Sprintf(" %s   ADMINISTRATION   sojuctl: %s", versionHeader(), app.backend.Path)
	putClipped(screen, 0, 0, width, header, styleHeader)
	drawAdminSidebar(screen, app, sidebarWidth, height-3)
	for y := 1; y < height-2; y++ {
		putClipped(screen, sidebarWidth, y, 1, "│", styleDivider)
	}
	contentX := sidebarWidth + 2
	contentWidth := width - contentX - 1
	if app.admin.ExitConfirm {
		drawExitConfirmation(screen, app, contentX, contentWidth, 2, height-4)
	} else if app.admin.Confirm != nil {
		drawAdminConfirmation(screen, app, contentX, contentWidth, 2, height-4)
	} else if app.admin.Form != nil {
		drawAdminForm(screen, app, contentX, contentWidth, 2, height-4)
	} else {
		drawAdminOutput(screen, app, contentX, contentWidth, 2, height-4)
	}
	putClipped(screen, 0, height-2, width, " ↑↓ select  Enter open  r refresh  Esc back  Ctrl-S preview  q quit", styleMuted)
	status := " " + app.currentStatusLocked()
	if app.admin.Busy {
		status += "  [running]"
	}
	putClipped(screen, 0, height-1, width, status, styleInput)
	screen.HideCursor()
	screen.Show()
}

func drawAdminSidebar(screen tcell.Screen, app *App, width, height int) {
	for y := 1; y < height; y++ {
		putClipped(screen, 0, y, width, " ", styleMuted)
	}
	putClipped(screen, 1, 1, width-1, "SOJU ADMINISTRATION", styleAccent)
	putClipped(screen, 1, 2, width-1, "No IRC chat connection", styleMuted)
	items := adminMenuItems()
	start := 0
	visible := height - 4
	if visible < 1 {
		visible = 1
	}
	if app.admin.Cursor >= visible {
		start = app.admin.Cursor - visible + 1
	}
	for i := start; i < len(items); i++ {
		y := i - start + 4
		if y >= height {
			break
		}
		style := styleMuted
		if i == app.admin.Cursor && app.admin.Form == nil && app.admin.Confirm == nil {
			style = styleAccent.Background(tcell.ColorDarkBlue)
		}
		item := items[i]
		putClipped(screen, 0, y, width, fmt.Sprintf(" %02d %s", i+1, item.Label), style)
	}
}

func drawAdminOutput(screen tcell.Screen, app *App, x, width, y, height int) {
	if len(app.admin.Output) == 0 {
		putClipped(screen, x, y, width, "No sojuctl output yet.", styleMuted)
		return
	}
	lines := make([]styledLine, 0, len(app.admin.Output))
	for _, text := range app.admin.Output {
		style := styleNormal
		lower := strings.ToLower(text)
		if strings.HasPrefix(text, "> ") {
			style = styleAccent
		}
		if strings.HasPrefix(text, "ERROR:") || strings.Contains(lower, "permission denied") {
			style = styleError
		}
		for _, rawLine := range strings.Split(text, "\n") {
			for _, piece := range wrapText(rawLine, width) {
				lines = append(lines, styledLine{text: piece, style: style})
			}
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
	form := app.admin.Form
	if form == nil {
		return
	}
	putClipped(screen, x, y, width, form.Title, styleAccent)
	putClipped(screen, x, y+1, width, "Enter advances · Space cycles choices · Ctrl-S previews · Esc cancels", styleMuted)
	row := y + 3
	visible := (height - 3) / 2
	if visible < 1 {
		visible = 1
	}
	start := 0
	if form.Cursor >= visible {
		start = form.Cursor - visible + 1
	}
	for i := start; i < len(form.Fields); i++ {
		field := form.Fields[i]
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
		line := fmt.Sprintf("%-22s %s", field.Label+":", value)
		putClipped(screen, x, row, width, line, style)
		if i == form.Cursor && field.Help != "" {
			putClipped(screen, x+2, row+1, width-2, field.Help, styleMuted)
		}
		row += 2
	}
}

func drawAdminConfirmation(screen tcell.Screen, app *App, x, width, y, height int) {
	confirmation := app.admin.Confirm
	if confirmation == nil {
		return
	}
	boxWidth := width
	if boxWidth < 30 {
		boxWidth = 30
	}
	boxHeight := height
	if boxHeight > 14 {
		boxHeight = 14
	}
	boxY := y + (height-boxHeight)/2
	background := styleError.Background(tcell.ColorDarkRed)
	for row := 0; row < boxHeight; row++ {
		putClipped(screen, x, boxY+row, boxWidth, " ", background)
	}
	putClipped(screen, x+2, boxY+1, boxWidth-4, "CONFIRM ADMINISTRATIVE CHANGE", background.Bold(true))
	putClipped(screen, x+2, boxY+3, boxWidth-4, confirmation.Operation.Summary, styleNormal.Background(tcell.ColorDarkRed))
	row := boxY + 5
	for _, line := range wrapText(confirmation.Operation.Preview, boxWidth-4) {
		if row >= boxY+boxHeight-2 {
			break
		}
		putClipped(screen, x+2, row, boxWidth-4, line, styleMuted.Background(tcell.ColorDarkRed))
		row++
	}
	if confirmation.Operation.ConfirmPhrase == "" {
		putClipped(screen, x+2, boxY+boxHeight-2, boxWidth-4, "Press y to apply · n or Esc to cancel", styleAccent.Background(tcell.ColorDarkRed))
		return
	}
	phraseRow := boxY + boxHeight - 4
	putClipped(screen, x+2, phraseRow, boxWidth-4, "Type exactly: "+confirmation.Operation.ConfirmPhrase, styleAccent.Background(tcell.ColorDarkRed))
	putClipped(screen, x+2, phraseRow+1, boxWidth-4, "> "+string(confirmation.Input), styleInput.Background(tcell.ColorDarkRed))
	putClipped(screen, x+2, boxY+boxHeight-2, boxWidth-4, "Enter confirms · Esc cancels", styleAccent.Background(tcell.ColorDarkRed))
}

func drawExitConfirmation(screen tcell.Screen, app *App, x, width, y, height int) {
	boxHeight := 9
	if boxHeight > height {
		boxHeight = height
	}
	boxY := y + (height-boxHeight)/2
	background := styleError.Background(tcell.ColorDarkRed)
	for row := 0; row < boxHeight; row++ {
		putClipped(screen, x, boxY+row, width, " ", background)
	}
	putClipped(screen, x+2, boxY+1, width-4, "EXIT SOJU ADMINISTRATION?", background.Bold(true))
	message := "No administrative changes will be made by exiting."
	if app.admin.Busy {
		message = "A sojuctl operation is running and will be cancelled."
	}
	putClipped(screen, x+2, boxY+3, width-4, message, styleNormal.Background(tcell.ColorDarkRed))
	putClipped(screen, x+2, boxY+boxHeight-2, width-4, "Press y to exit · n or Esc to stay", styleAccent.Background(tcell.ColorDarkRed))
}
