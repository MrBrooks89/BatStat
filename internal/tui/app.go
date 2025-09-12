package tui

import (
	"time"

	"github.com/MrBrooks89/BatStat/internal/conn"
	"github.com/rivo/tview"
)

type App struct {
	tviewApp *tview.Application
	view     *View
	state    *AppState
}

func NewApp() *App {
	a := &App{
		tviewApp: tview.NewApplication(),
		state:    NewAppState(),
	}
	a.view = NewView(a) 
	return a
}

func (a *App) Run() error {
	a.view.Init()
	a.setKeybindings()

	go a.refreshDataLoop()

	return a.tviewApp.Run()
}

func (a *App) Stop() {
	a.tviewApp.Stop()
}

func (a *App) refreshDataLoop() {
	a.loadData()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		a.loadData()
	}
}

func (a *App) loadData() {
	selectedRow, _ := a.view.table.GetSelection()

	conns, err := conn.FetchConnections()
	if err != nil {
		return
	}

	a.state.SetConnections(conns)

	a.tviewApp.QueueUpdateDraw(func() {
		a.view.Refresh()
		if selectedRow < a.view.table.GetRowCount() {
			a.view.table.Select(selectedRow, 0)
		} else if a.view.table.GetRowCount() > 1 {
			a.view.table.Select(1, 0) 
		}
	})
}
