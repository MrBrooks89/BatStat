package tui

import (
	"encoding/csv"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestSnapshotMatchingPrioritizesKnownOwnersOneToOne(t *testing.T) {
	unknown := models.Connection{Family: "IPv6", Type: "TCP", Laddr: "[::1]:8000", Raddr: "[::1]:443"}
	first := unknown
	first.Pid = 10
	second := unknown
	second.Pid = 20
	changed := first
	changed.Status = "TIME_WAIT"

	before, after := matchSnapshots(
		[]models.Connection{unknown, first, second},
		[]models.Connection{first, changed, first},
	)
	if !reflect.DeepEqual(before, []bool{true, true, false}) ||
		!reflect.DeepEqual(after, []bool{true, true, false}) {
		t.Fatalf("matches before and after are %v, %v", before, after)
	}

	before, after = matchSnapshots(
		[]models.Connection{unknown, first},
		[]models.Connection{unknown, second},
	)
	if !reflect.DeepEqual(before, []bool{true, true}) ||
		!reflect.DeepEqual(after, []bool{true, true}) {
		t.Fatalf("unknown owners should allow both matches, got %v, %v", before, after)
	}

	_, after = matchSnapshots([]models.Connection{first}, []models.Connection{changed})
	if !after[0] {
		t.Fatal("a TCP state change alone should still match")
	}
}

func TestRecentlyVanishedRetentionAndReconnection(t *testing.T) {
	state := NewAppState()
	at := time.Date(2026, 11, 24, 13, 0, 0, 0, time.Local)
	conn := models.Connection{Type: "UDP", Laddr: ":5353", Pid: 12}
	state.SetConnections([]models.Connection{conn, conn}, at)
	live, vanished, _ := state.displayedRows()
	if live[0].isNew || live[1].isNew || len(vanished) != 0 {
		t.Fatalf("first snapshot showed changes: %v, %v", live, vanished)
	}

	for refresh := 1; refresh <= 4; refresh++ {
		state.SetConnections([]models.Connection{conn}, at.Add(time.Duration(refresh)*time.Second))
		live, vanished, _ = state.displayedRows()
		wantVanished := 1
		if refresh == 4 {
			wantVanished = 0
		}
		if live[0].isNew || len(vanished) != wantVanished {
			t.Fatalf("refresh %d showed %v, %v, want %d vanished", refresh, live, vanished, wantVanished)
		}
	}

	state.SetConnections(nil, at)
	state.SetConnections([]models.Connection{conn}, at)
	live, vanished, _ = state.displayedRows()
	if len(live) != 1 || !live[0].isNew || len(vanished) != 0 {
		t.Fatalf("returning connection showed %v, %v", live, vanished)
	}
}

func TestPinnedComparisonUsesFullSnapshotAndKeepsRollingHistory(t *testing.T) {
	state := NewAppState()
	at := time.Date(2026, 11, 24, 13, 0, 0, 0, time.Local)
	original := make([]models.Connection, 25)
	for i := range original {
		original[i] = models.Connection{ProcessName: "original", Laddr: fmt.Sprintf(":%d", i)}
	}
	state.SetConnections(original, at)
	state.SetFilterText("hidden by filter")
	if !state.pinSnapshot() {
		t.Fatal("pin should work while a filter hides the entire snapshot")
	}
	state.SetFilterText("")

	newConnection := models.Connection{ProcessName: "replacement", Laddr: ":80"}
	for i := 1; i <= 5; i++ {
		state.SetConnections([]models.Connection{newConnection}, at.Add(time.Duration(i)*time.Second))
		live, vanished, pinTime := state.displayedRows()
		if len(live) != 1 || !live[0].isNew || len(vanished) != len(original) || !pinTime.Equal(at) {
			t.Fatalf("pinned refresh %d showed %v, %d vanished, pin time %v", i, live, len(vanished), pinTime)
		}
		if i == 1 {
			state.togglePinned()
			_, recent, _ := state.displayedRows()
			if len(recent) != maxRecentlyVanished {
				t.Fatalf("rolling list has %d entries, want %d", len(recent), maxRecentlyVanished)
			}
			state.togglePinned()
		}
	}
	state.togglePinned()
	live, vanished, pinTime := state.displayedRows()
	if live[0].isNew || len(vanished) != 0 || !pinTime.IsZero() {
		t.Fatalf("rolling comparison showed %v, %v, pin time %v", live, vanished, pinTime)
	}
}

func TestSnapshotDiffControlsAndCSVExport(t *testing.T) {
	t.Chdir(t.TempDir())
	app := NewApp()
	app.view.Init()
	app.setKeybindings()
	at := time.Date(2026, 11, 24, 13, 0, 0, 0, time.Local)
	before := models.Connection{ProcessName: "firefox old", Laddr: ":1000", Pid: 11}
	after := models.Connection{ProcessName: "firefox new", Laddr: ":1001", Pid: 12}
	hidden := models.Connection{ProcessName: "other", Laddr: ":1002"}
	apply := func(conns []models.Connection, capturedAt time.Time) {
		t.Helper()
		id, ok := app.beginRefresh()
		if !ok {
			t.Fatal("expected a refresh to start")
		}
		app.applyRefresh(id, conns, capturedAt)
	}

	apply([]models.Connection{before}, at)
	press := func(key tcell.Key, r rune) {
		t.Helper()
		if event := app.tviewApp.GetInputCapture()(tcell.NewEventKey(key, r, tcell.ModNone)); event != nil {
			t.Fatalf("snapshot control %q was not consumed", r)
		}
	}
	app.setPaused(true)
	press(tcell.KeyRune, 'B')
	app.setPaused(false)
	apply([]models.Connection{after, hidden}, at.Add(time.Second))
	app.view.filterInput.SetText("firefox")
	if got := app.view.table.GetCell(1, 0).Text; got != "New since pin" {
		t.Fatalf("live marker = %q", got)
	}
	if got := app.view.vanishedTable.GetCell(1, 1).Text; got != before.ProcessName {
		t.Fatalf("vanished row = %q, want %q", got, before.ProcessName)
	}

	press(tcell.KeyTab, 0)
	if !app.view.vanishedTable.HasFocus() || app.view.GetSelectedConnection() != nil {
		t.Fatal("vanished row must be selectable for inspection only")
	}
	if got := app.view.detailsView.GetText(true); !strings.Contains(got, "firefox old") ||
		strings.Contains(got, "Command:") || strings.Contains(got, "User:") {
		t.Fatalf("unexpected vanished details: %s", got)
	}
	press(tcell.KeyRune, 'v')
	if app.view.vanishedVisible || !app.view.table.HasFocus() {
		t.Fatal("collapsing the vanished table should return focus to live connections")
	}
	press(tcell.KeyRune, 'v')
	press(tcell.KeyRune, 'b')
	if got := app.view.table.GetCell(1, 0).Text; got != "NEW" {
		t.Fatalf("previous-refresh marker = %q", got)
	}

	press(tcell.KeyTab, 0)
	press(tcell.KeyRune, 'e')
	if name, _ := app.view.pages.GetFrontPage(); name != "export_modal" {
		t.Fatalf("expected export modal, got %q", name)
	}
	app.view.pages.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {
		app.tviewApp.SetFocus(p)
	})
	file, err := os.Open("batstat_export.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"ProcessName", "PID", "Status", "Family", "Type", "LocalAddr", "RemoteAddr"},
		{after.ProcessName, "12", "", "", "", after.Laddr, ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CSV = %v, want %v", got, want)
	}
}

func TestPinnedVanishedListScrollsInShortTerminal(t *testing.T) {
	app := NewApp()
	app.view.Init()
	at := time.Date(2026, 11, 24, 13, 0, 0, 0, time.Local)
	before := make([]models.Connection, 25)
	for i := range before {
		before[i] = models.Connection{
			ProcessName: fmt.Sprintf("vanished-%02d", i),
			Laddr:       fmt.Sprintf(":%d", 8000+i),
		}
	}
	app.state.SetConnections(before, at)
	app.state.pinSnapshot()
	app.state.SetConnections(nil, at.Add(time.Second))
	app.view.Refresh()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(100, 12)
	app.view.pages.SetRect(0, 0, 100, 12)
	draw := func() string {
		t.Helper()
		screen.Clear()
		app.view.pages.Draw(screen)
		screen.Show()
		cells, _, _ := screen.GetContents()
		var rendered strings.Builder
		for _, cell := range cells {
			rendered.WriteString(string(cell.Runes))
		}
		return rendered.String()
	}
	if !strings.Contains(draw(), "vanished-00") {
		t.Fatal("short terminal should display the first vanished connection")
	}

	table := app.view.vanishedTable
	app.tviewApp.SetFocus(table)
	table.InputHandler()(tcell.NewEventKey(tcell.KeyEnd, 0, tcell.ModNone), func(p tview.Primitive) {
		app.tviewApp.SetFocus(p)
	})
	if !strings.Contains(draw(), "vanished-24") {
		t.Fatal("short terminal should scroll to the final pinned difference")
	}
}
