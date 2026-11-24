package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
)

func TestPauseKeepsLastDisplayedSnapshot(t *testing.T) {
	app := NewApp()
	app.view.Init()
	app.setKeybindings()

	beginRefresh := func() uint64 {
		t.Helper()
		id, ok := app.beginRefresh()
		if !ok {
			t.Fatal("expected a refresh to start")
		}
		return id
	}
	requireDisplayedProcess := func(want string) {
		t.Helper()
		if got := app.view.table.GetCell(1, 1).Text; got != want {
			t.Fatalf("displayed process = %q, want %q", got, want)
		}
		if got := app.state.GetFilteredConnections(); len(got) != 1 || got[0].ProcessName != want {
			t.Fatalf("connections available for export = %v, want %q", got, want)
		}
	}

	app.applyRefresh(beginRefresh(), []models.Connection{{ProcessName: "displayed", Laddr: ":8000"}}, time.Now())
	beforePause := beginRefresh()
	alsoBeforePause := beginRefresh()

	space := tcell.NewEventKey(tcell.KeyRune, ' ', tcell.ModNone)
	if event := app.tviewApp.GetInputCapture()(space); event != nil {
		t.Fatal("expected Space to be consumed")
	}
	if got := app.view.detailsView.GetTitle(); !strings.Contains(got, "Paused") {
		t.Fatalf("details title = %q, want Paused", got)
	}
	if _, ok := app.beginRefresh(); ok {
		t.Fatal("started a refresh while paused")
	}
	app.applyRefresh(beforePause, []models.Connection{{ProcessName: "stale"}}, time.Now())
	requireDisplayedProcess("displayed")

	app.setPaused(false)
	app.applyRefresh(alsoBeforePause, []models.Connection{{ProcessName: "also stale"}}, time.Now())
	requireDisplayedProcess("displayed")
	if _, vanished, _ := app.state.displayedRows(); len(vanished) != 0 {
		t.Fatalf("discarded refresh changed the vanished list: %v", vanished)
	}

	app.applyRefresh(beginRefresh(), []models.Connection{{ProcessName: "fresh", Laddr: ":8001"}}, time.Now())
	requireDisplayedProcess("fresh")
	if live, vanished, _ := app.state.displayedRows(); !live[0].isNew || len(vanished) != 1 || vanished[0].ProcessName != "displayed" {
		t.Fatalf("resume should compare with the paused snapshot: live = %v, vanished = %v", live, vanished)
	}
	if got := app.view.detailsView.GetTitle(); got != " Details " {
		t.Fatalf("details title = %q after fresh data arrived", got)
	}
}
