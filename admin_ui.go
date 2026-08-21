package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

const adminSidebarSubtitle = "Managing Soju via sojuctl"

type adminBrandLine struct {
	text  string
	style tcell.Style
}

var fullAdminBrand = []adminBrandLine{
	{text: adminBrandWithBottle(`  ____    ____   _____   _   _`, `____`, 35), style: styleAccent},
	{text: adminBrandWithBottle(` / ___|  / __ \ |  ___| | | | |`, `|    |`, 34), style: styleAccent},
	{text: adminBrandWithBottle(` \___ \ | |  | || |___  | | | |`, `|____|`, 34), style: styleAccent},
	{text: adminBrandWithBottle(`  ___) || |__| ||  ___| | |_| |`, "/      \\", 33), style: styleAccent},
	{text: adminBrandWithBottle(` |____/  \____/ |_|      \___/`, "/          \\", 31), style: styleAccent},
	{text: adminBrandWithBottle(`    \____\____\____\____\____`, "/            \\", 30), style: styleInfo},
	{text: adminBrandWithBottle(``, `|              |`, 29), style: styleInfo},
	{text: adminBrandWithBottle(` _____  _   _  ___`, `|     SOJU     |`, 29), style: styleInfo},
	{text: adminBrandWithBottle(`|_   _|| | | | |_ |`, `|      TUI     |`, 29), style: styleInfo},
	{text: adminBrandWithBottle(`  | |  | | | |  | |`, `|              |`, 29), style: styleInfo},
	{text: adminBrandWithBottle(`  | |  | |_| |  | |`, `|              |`, 29), style: styleInfo},
	{text: adminBrandWithBottle(`  |_|   \___/  |___|`, `|______________|`, 29), style: styleInfo},
	{text: adminBrandWithBottle(`    \____\____\____\____`, `\____________/`, 30), style: styleInfo},
	{text: "", style: styleMuted},
	{text: `       SOJU-TUI ADMINISTRATION CONSOLE`, style: styleMuted},
}

// adminBrandWithBottle keeps the bottle on a fixed column even when a wordmark
// row has a different width. This prevents small spacing edits from skewing its
// center axis on terminals with different pane widths.
func adminBrandWithBottle(left, bottle string, bottleColumn int) string {
	padding := bottleColumn - runewidth.StringWidth(left)
	if padding < 1 {
		padding = 1
	}
	return left + strings.Repeat(" ", padding) + bottle
}

var compactAdminBrand = []adminBrandLine{
	{text: `   SOJU-TUI`, style: styleAccent},
	{text: `      ____`, style: styleInfo},
	{text: `     |    |`, style: styleInfo},
	{text: `     |____|`, style: styleInfo},
	{text: `    /      \`, style: styleInfo},
	{text: `   /        \`, style: styleInfo},
	{text: `  |   SOJU   |`, style: styleInfo},
	{text: `  |   TUI    |`, style: styleInfo},
	{text: `  |          |`, style: styleInfo},
	{text: `  |__________|`, style: styleInfo},
	{text: ` ADMINISTRATION`, style: styleMuted},
}

func handleAdminKeyEvent(app *App, event *tcell.EventKey) {
	key := ""
	switch event.Key() {
	case tcell.KeyUp:
		key = "up"
	case tcell.KeyDown:
		key = "down"
	case tcell.KeyHome:
		key = "home"
	case tcell.KeyEnd:
		key = "end"
	case tcell.KeyPgUp:
		key = "pageup"
	case tcell.KeyPgDn:
		key = "pagedown"
	case tcell.KeyTAB:
		key = "tab"
	case tcell.KeyBacktab:
		key = "backtab"
	case tcell.KeyEnter:
		key = "enter"
	case tcell.KeyCtrlS:
		key = "submit"
	case tcell.KeyF1:
		key = "help"
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
	} else if app.admin.HelpOpen {
		drawAdminHelp(screen, app, contentX, contentWidth, 2, height-4)
	} else {
		drawAdminOutput(screen, app, contentX, contentWidth, 2, height-4)
	}
	footer := " ↑↓ select  Enter open  ? help  r refresh  Esc back  Ctrl-S preview  q quit"
	if app.admin.HelpOpen && !app.admin.ExitConfirm {
		footer = " ↑↓ scroll  PgUp/PgDn page  Home/End jump  Esc or ? close  q quit"
	}
	putClipped(screen, 0, height-2, width, footer, styleMuted)
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
	putClipped(screen, 1, 2, width-1, adminSidebarSubtitle, styleMuted)
	items := app.adminMenuItemsLocked()
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
		item := items[i]
		if strings.Contains(item.Kind, "cert") {
			style = styleInfo
		}
		if i == app.admin.Cursor && app.admin.Form == nil && app.admin.Confirm == nil {
			style = styleAccent.Background(tcell.ColorDarkBlue)
		}
		putClipped(screen, 0, y, width, fmt.Sprintf(" %02d %s", i+1, item.Label), style)
	}
}

func drawAdminOutput(screen tcell.Screen, app *App, x, width, y, height int) {
	if len(app.admin.Output) == 0 {
		putClipped(screen, x, y, width, "No sojuctl output yet.", styleMuted)
		drawAdminBrand(screen, x, width, y+2, y+height)
		return
	}
	lines := make([]styledLine, 0, len(app.admin.Output))
	for _, text := range app.admin.Output {
		style := styleNormal
		lower := strings.ToLower(text)
		if strings.HasPrefix(text, "> ") {
			style = styleAccent
		}
		if strings.Contains(strings.ToUpper(text), "CERTIFICATE") && !strings.HasPrefix(text, "ERROR:") {
			style = styleInfo
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
	drawAdminBrand(screen, x, width, row+1, y+height)
}

func drawAdminBrand(screen tcell.Screen, x, width, top, bottom int) {
	availableHeight := bottom - top
	brand := fullAdminBrand
	brandWidth := adminBrandWidth(brand)
	if width < brandWidth+2 || availableHeight < len(fullAdminBrand)+2 {
		brand = compactAdminBrand
		brandWidth = adminBrandWidth(brand)
	}
	if width < brandWidth || availableHeight < len(brand)+1 {
		return
	}
	brandTop := top + (availableHeight-len(brand))/2
	brandX := x + (width-brandWidth)/2
	for index, line := range brand {
		putClipped(screen, brandX, brandTop+index, brandWidth, line.text, line.style)
	}
}

func adminBrandWidth(brand []adminBrandLine) int {
	width := 0
	for _, line := range brand {
		if lineWidth := runewidth.StringWidth(line.text); lineWidth > width {
			width = lineWidth
		}
	}
	return width
}

func drawAdminHelp(screen tcell.Screen, app *App, x, width, y, height int) {
	lines := sojuTUIHelp()
	start := app.admin.HelpScroll
	if start < 0 {
		start = 0
	}
	if start >= len(lines) {
		start = len(lines) - 1
	}
	row := y
	for index := start; index < len(lines) && row < y+height; index++ {
		style := styleNormal
		line := lines[index]
		if index == 0 || strings.ToUpper(line) == line && line != "" && !strings.Contains(line, "HTTPS://") {
			style = styleAccent
		}
		if strings.Contains(line, "https://") {
			style = styleInfo
		}
		for _, wrapped := range wrapText(line, width) {
			if row >= y+height {
				break
			}
			putClipped(screen, x, row, width, wrapped, style)
			row++
		}
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
		if field.Kind == "user" && len(field.Options) > 0 {
			choice := "custom"
			for index, option := range field.Options {
				if value == option {
					choice = fmt.Sprintf("%d/%d", index+1, len(field.Options))
					break
				}
			}
			value += "  [" + choice + "]"
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
