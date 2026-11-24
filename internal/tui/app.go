package tui

import (
	"sync"
	"time"

	"github.com/MrBrooks89/BatStat/internal/conn"
	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/rivo/tview"
)

type App struct {
	tviewApp *tview.Application
	view     *View
	state    *AppState

	refreshMu     sync.Mutex
	nextRefreshID uint64
	paused        bool

	// These fields are only used on the UI thread.
	ignoreRefreshThroughID uint64
	awaitingLiveData       bool
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

	done := make(chan struct{})
	defer close(done)
	go a.refreshDataLoop(done)

	return a.tviewApp.Run()
}

func (a *App) Stop() {
	a.tviewApp.Stop()
}

func (a *App) refreshDataLoop(done <-chan struct{}) {
	a.loadData()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			a.loadData()
		}
	}
}

func (a *App) setPaused(paused bool) {
	a.refreshMu.Lock()
	a.paused = paused
	if paused {
		// Discard every refresh that started before this pause, even if it
		// finishes after monitoring resumes.
		a.ignoreRefreshThroughID = a.nextRefreshID
		a.awaitingLiveData = true
	}
	a.refreshMu.Unlock()

	if paused {
		a.view.detailsView.SetTitle(" Paused | Details ")
		a.view.updateDetailsView()
	} else {
		a.view.detailsView.SetTitle(" Resuming | Details ")
	}
}

func (a *App) beginRefresh() (uint64, bool) {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	if a.paused {
		return 0, false
	}
	a.nextRefreshID++
	return a.nextRefreshID, true
}

func (a *App) loadData() {
	id, ok := a.beginRefresh()
	if !ok {
		return
	}
	conns, err := conn.FetchConnections()
	if err != nil {
		return
	}
	capturedAt := time.Now()
	a.tviewApp.QueueUpdateDraw(func() {
		a.applyRefresh(id, conns, capturedAt)
	})
}

func (a *App) applyRefresh(id uint64, conns []models.Connection, capturedAt time.Time) {
	if id <= a.ignoreRefreshThroughID {
		return
	}
	a.ignoreRefreshThroughID = id
	a.awaitingLiveData = false
	a.view.detailsView.SetTitle(" Details ")

	a.state.SetConnections(conns, capturedAt)
	a.view.processes.reconcileSnapshot(conns)
	a.view.Refresh()
}
