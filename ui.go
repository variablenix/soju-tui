package main

import (
	"fmt"
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
	styleInfo    = tcell.StyleDefault.Foreground(tcell.ColorLightSkyBlue)
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
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case result := <-app.results:
			app.processResult(result)
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
		width, height := screen.Size()
		_, _, contentWidth, contentHeight := adminContentGeometry(width, height)
		handleAdminKeyEventWithViewport(app, event, contentWidth, contentHeight)
	}
	return false
}

func drawUI(screen tcell.Screen, app *App) {
	app.mu.RLock()
	defer app.mu.RUnlock()
	width, height := screen.Size()
	screen.Clear()
	if width < 32 || height < 8 {
		putClipped(screen, 0, 0, width, "terminal too small for soju administration", styleError)
		screen.Show()
		return
	}
	drawAdminUI(screen, app)
}

type styledLine struct {
	text  string
	style tcell.Style
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
	column := x
	used := 0
	for _, r := range text {
		runeWidth := runewidth.RuneWidth(r)
		if runeWidth <= 0 {
			continue
		}
		if used+runeWidth > width {
			break
		}
		screen.SetContent(column, y, r, nil, style)
		column += runeWidth
		used += runeWidth
	}
}

func versionHeader() string {
	return fmt.Sprintf("soju-tui admin v%s", version)
}
