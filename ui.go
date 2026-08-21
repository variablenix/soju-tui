package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

var (
	styleHeader  = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorLightCyan).Bold(true)
	styleDivider = tcell.StyleDefault.Foreground(tcell.ColorDarkCyan)
	styleNormal  = tcell.StyleDefault.Foreground(tcell.ColorWhite)
	styleMuted   = tcell.StyleDefault.Foreground(tcell.ColorGray)
	styleAccent  = tcell.StyleDefault.Foreground(tcell.ColorLightCyan).Bold(true)
	styleEvent   = tcell.StyleDefault.Foreground(tcell.ColorYellow)
	styleError   = tcell.StyleDefault.Foreground(tcell.ColorLightCoral)
	styleInput   = tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorDarkBlue)
)

func runUI(app *App) error {
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err := screen.Init(); err != nil {
		return err
	}
	defer screen.Fini()
	screen.Clear()

	uiEvents := make(chan tcell.Event, 1)
	go func() {
		for {
			uiEvents <- screen.PollEvent()
		}
	}()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	drawUI(screen, app)
	for {
		select {
		case event := <-app.events:
			app.processEvent(event)
		case event := <-uiEvents:
			if handleUIEvent(screen, app, event) {
				app.close()
				return nil
			}
		case <-ticker.C:
		}
		drawUI(screen, app)
		app.mu.RLock()
		shouldQuit := app.quit
		app.mu.RUnlock()
		if shouldQuit {
			return nil
		}
	}
}

func handleUIEvent(screen tcell.Screen, app *App, event tcell.Event) bool {
	switch event := event.(type) {
	case *tcell.EventResize:
		screen.Sync()
	case *tcell.EventKey:
		if event.Key() == tcell.KeyF2 {
			app.toggleAdmin()
			return false
		}
		app.mu.RLock()
		adminActive := app.admin.active
		app.mu.RUnlock()
		if adminActive {
			if event.Key() == tcell.KeyCtrlC || event.Key() == tcell.KeyCtrlQ {
				return true
			}
			handleAdminKeyEvent(app, event)
			return false
		}
		switch event.Key() {
		case tcell.KeyCtrlC, tcell.KeyCtrlQ:
			return true
		case tcell.KeyEnter:
			app.sendInput()
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			app.mu.Lock()
			if len(app.input) > 0 {
				app.input = app.input[:len(app.input)-1]
			}
			app.mu.Unlock()
		case tcell.KeyEscape:
			app.mu.Lock()
			app.input = nil
			app.mu.Unlock()
		case tcell.KeyTAB:
			app.nextBuffer(1)
		case tcell.KeyCtrlN:
			app.nextBuffer(1)
		case tcell.KeyCtrlP:
			app.nextBuffer(-1)
		case tcell.KeyUp:
			app.historyMove(-1)
		case tcell.KeyDown:
			app.historyMove(1)
		case tcell.KeyPgUp:
			app.scrollMessages(6)
		case tcell.KeyPgDn:
			app.scrollMessages(-6)
		case tcell.KeyCtrlL:
			screen.Sync()
		case tcell.KeyRune:
			if event.Rune() != 0 {
				app.mu.Lock()
				app.input = append(app.input, event.Rune())
				app.mu.Unlock()
			}
		}
	}
	return false
}

func (a *App) historyMove(delta int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.history) == 0 {
		return
	}
	if a.historyPos == len(a.history) && delta < 0 {
		a.historyPos = len(a.history) - 1
	} else {
		a.historyPos += delta
	}
	if a.historyPos < 0 {
		a.historyPos = 0
	}
	if a.historyPos >= len(a.history) {
		a.historyPos = len(a.history)
		a.input = nil
		return
	}
	a.input = []rune(a.history[a.historyPos])
}

func (a *App) scrollMessages(delta int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.scroll += delta
	if a.scroll < 0 {
		a.scroll = 0
	}
	if delta > 0 {
		buffer := a.buffers[a.active]
		if buffer != nil && buffer.Target != "" {
			if session := a.activeSessionLocked(); session != nil && session.EnabledCaps["draft/chathistory"] {
				if buffer.OldestMsgID != "" {
					_ = session.Send("CHATHISTORY", "BEFORE", buffer.Target, buffer.OldestMsgID, "50")
				} else {
					_ = session.Send("CHATHISTORY", "LATEST", buffer.Target, "*", "50")
				}
				// The first message in the next history batch becomes the new
				// cursor. This also keeps the initial live-only case useful.
				buffer.OldestMsgID = ""
			}
		}
	}
}

func drawUI(screen tcell.Screen, app *App) {
	app.mu.RLock()
	defer app.mu.RUnlock()
	width, height := screen.Size()
	if width < 20 || height < 5 {
		screen.Clear()
		putClipped(screen, 0, 0, width, "terminal too small", styleError)
		screen.Show()
		return
	}
	screen.Clear()
	if app.admin.active {
		drawAdminUI(screen, app)
		return
	}

	sidebarWidth := width / 4
	if sidebarWidth < 22 {
		sidebarWidth = 22
	}
	if sidebarWidth > 34 {
		sidebarWidth = 34
	}
	if sidebarWidth >= width-10 {
		sidebarWidth = width / 3
	}

	header := fmt.Sprintf(" soju-tui  %s  %s", app.cfg.Server, app.currentBufferTitleLocked())
	putClipped(screen, 0, 0, width, header, styleHeader)
	drawSidebar(screen, app, sidebarWidth, height-3)
	for y := 1; y < height-2; y++ {
		putClipped(screen, sidebarWidth, y, 1, "│", styleDivider)
	}
	drawMessages(screen, app, sidebarWidth+1, width-sidebarWidth-1, 1, height-3)

	status := " " + app.currentStatusLocked()
	putClipped(screen, 0, height-2, width, status, styleMuted)
	input := " > " + string(app.input)
	putClipped(screen, 0, height-1, width, input, styleInput)
	cursorX := 3 + runewidth.StringWidth(string(app.input))
	if cursorX >= width {
		cursorX = width - 1
	}
	screen.ShowCursor(cursorX, height-1)
	screen.Show()
}

func drawSidebar(screen tcell.Screen, app *App, width, height int) {
	for y := 1; y < height; y++ {
		putClipped(screen, 0, y, width, " ", styleMuted)
	}
	y := 1
	drawBufferRow(screen, app, "root", y, width, "BouncerServ")
	y++
	ids := make([]string, 0, len(app.networks))
	for id := range app.networks {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return app.networks[ids[i]].displayName() < app.networks[ids[j]].displayName()
	})
	for _, id := range ids {
		if y >= height {
			break
		}
		network := app.networks[id]
		state := "·"
		if network.State == "connected" {
			state = "●"
		} else if network.State == "error" {
			state = "!"
		}
		putClipped(screen, 0, y, width, fmt.Sprintf(" %s %s", state, network.displayName()), styleAccent)
		y++
		for _, key := range app.order {
			if y >= height {
				break
			}
			buffer := app.buffers[key]
			if buffer == nil || buffer.NetworkID != id || buffer.Target == "" {
				continue
			}
			name := "  " + buffer.Title
			if buffer.Unread > 0 {
				name += fmt.Sprintf(" [%d]", buffer.Unread)
			}
			drawBufferRow(screen, app, key, y, width, name)
			y++
		}
	}
}

func drawBufferRow(screen tcell.Screen, app *App, key string, y, width int, title string) {
	style := styleMuted
	if key == app.active {
		style = styleAccent.Background(tcell.ColorDarkBlue)
	}
	putClipped(screen, 0, y, width, " "+title, style)
}

func drawMessages(screen tcell.Screen, app *App, x, width, y, height int) {
	buffer := app.buffers[app.active]
	if buffer == nil {
		return
	}
	wrapped := make([]styledLine, 0, len(buffer.Lines))
	for _, line := range buffer.Lines {
		style := styleNormal
		if line.Kind == "event" || line.Kind == "topic" {
			style = styleEvent
		} else if line.Kind == "error" {
			style = styleError
		} else if line.Kind == "help" || line.Kind == "server" || line.Kind == "notice" {
			style = styleMuted
		}
		text := formatChatLine(line)
		for _, piece := range wrapText(text, width) {
			wrapped = append(wrapped, styledLine{text: piece, style: style})
		}
	}
	start := len(wrapped) - height - app.scroll
	if start < 0 {
		start = 0
	}
	end := start + height
	if end > len(wrapped) {
		end = len(wrapped)
	}
	row := y
	for _, line := range wrapped[start:end] {
		putClipped(screen, x, row, width, line.text, line.style)
		row++
	}
}

type styledLine struct {
	text  string
	style tcell.Style
}

func formatChatLine(line ChatLine) string {
	timestamp := line.When.Format("15:04")
	if line.When.IsZero() {
		timestamp = "     "
	}
	if line.From != "" && line.Text != "" {
		return fmt.Sprintf("%s <%s> %s", timestamp, line.From, line.Text)
	}
	if line.Text != "" {
		return fmt.Sprintf("%s %s", timestamp, line.Text)
	}
	return timestamp
}

func wrapText(text string, width int) []string {
	if width <= 1 {
		return []string{text}
	}
	if text == "" {
		return []string{""}
	}
	result := make([]string, 0, 2)
	var current []rune
	currentWidth := 0
	for _, r := range text {
		runeWidth := runewidth.RuneWidth(r)
		if runeWidth == 0 {
			current = append(current, r)
			continue
		}
		if currentWidth+runeWidth > width && len(current) > 0 {
			result = append(result, string(current))
			current = nil
			currentWidth = 0
		}
		current = append(current, r)
		currentWidth += runeWidth
	}
	if len(current) > 0 || len(result) == 0 {
		result = append(result, string(current))
	}
	return result
}

func putClipped(screen tcell.Screen, x, y, width int, text string, style tcell.Style) {
	if width <= 0 || y < 0 {
		return
	}
	rowWidth := 0
	for _, r := range text {
		runeWidth := runewidth.RuneWidth(r)
		if runeWidth <= 0 {
			continue
		}
		if rowWidth+runeWidth > width {
			break
		}
		screen.SetContent(x+rowWidth, y, r, nil, style)
		rowWidth += runeWidth
	}
	for rowWidth < width {
		screen.SetContent(x+rowWidth, y, ' ', nil, style)
		rowWidth++
	}
}

func (a *App) currentBufferTitleLocked() string {
	if buffer := a.buffers[a.active]; buffer != nil {
		return buffer.Title
	}
	return "BouncerServ"
}
