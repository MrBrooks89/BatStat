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

func TestPinnedComparisonChangesOnly(t *testing.T) {
	app := NewApp()
	app.view.Init()
	app.setKeybindings()
	press := func(r rune) {
		t.Helper()
		processKey(app, tcell.KeyRune, r)
	}
	listener := models.Connection{ProcessName: "app existing", Family: "IPv4", Type: "TCP", Laddr: "127.0.0.1:8000", Status: "LISTEN"}
	client := models.Connection{ProcessName: "app client", Family: "IPv4", Type: "TCP", Laddr: "127.0.0.1:8001", Status: "ESTABLISHED"}
	oldListener := models.Connection{ProcessName: "app old", Family: "IPv4", Type: "TCP", Laddr: "127.0.0.1:8002", Status: "LISTEN"}
	oldOther := models.Connection{ProcessName: "other old", Family: "IPv4", Type: "TCP", Laddr: "127.0.0.1:8003", Status: "LISTEN"}
	newListener := models.Connection{ProcessName: "app new", Family: "IPv4", Type: "TCP", Laddr: "127.0.0.1:8004", Status: "LISTEN"}
	newOther := models.Connection{ProcessName: "app outbound", Family: "IPv4", Type: "TCP", Laddr: "127.0.0.1:8005", Status: "ESTABLISHED"}

	at := time.Date(2026, 11, 24, 13, 0, 0, 0, time.Local)
	app.applyRefresh(1, []models.Connection{listener, client, oldListener, oldOther}, at)
	press('B')
	client.Status = "CLOSE_WAIT"
	app.applyRefresh(2, []models.Connection{listener, client, newListener, newOther}, at.Add(time.Second))

	assertRows := func(table *tview.Table, want ...string) {
		t.Helper()
		var got []string
		for row := 1; row < table.GetRowCount(); row++ {
			got = append(got, table.GetCell(row, 1).Text)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("process rows = %v, want %v", got, want)
		}
	}
	assertStatus := func(parts ...string) {
		t.Helper()
		status := app.view.baselineView.GetText(true)
		for _, want := range parts {
			if !strings.Contains(status, want) {
				t.Errorf("comparison status = %q, missing %q", status, want)
			}
		}
	}
	assertRows(app.view.table, "app existing", "app client", "app new", "app outbound")
	press('C')
	assertRows(app.view.table, "app new", "app outbound")
	assertRows(app.view.vanishedTable, "app old", "other old")
	assertStatus("Pinned at 2026-11-24 13:00:00", "New since pin (2)", "Vanished since pin (2)")
	if got := app.view.detailsView.GetText(true); !strings.Contains(got, "127.0.0.1:8004") {
		t.Errorf("details should describe the selected new row, got %q", got)
	}

	setFilter := func(query string) {
		t.Helper()
		press('/')
		app.view.filterInput.SetText(query)
		processKey(app, tcell.KeyEnter, 0)
	}
	setFilter("app")
	assertRows(app.view.table, "app new", "app outbound")
	assertRows(app.view.vanishedTable, "app old")
	assertStatus("New since pin (2)", "Vanished since pin (1)")
	press('L')
	assertRows(app.view.table, "app new")
	assertRows(app.view.vanishedTable, "app old")
	assertStatus("New since pin (1)", "Vanished since pin (1)")
	press('v')
	assertRows(app.view.vanishedTable)
	assertStatus("Vanished since pin (1)", "collapsed")
	press('v')

	setFilter("app existing")
	assertRows(app.view.table)
	assertRows(app.view.vanishedTable)
	assertStatus("New since pin (0)", "Vanished since pin (0)")
	if got := app.view.detailsView.GetText(true); !strings.Contains(got, "No matching changes since pin") {
		t.Errorf("empty comparison details = %q", got)
	}
	setFilter("pid:invalid")
	assertStatus("Invalid PID")
	if got := app.view.detailsView.GetText(true); strings.Contains(got, "No matching changes since pin") {
		t.Errorf("invalid filter displayed as an empty comparison: %q", got)
	}

	setFilter("app")
	press('C')
	assertRows(app.view.table, "app existing", "app new")
	assertRows(app.view.vanishedTable, "app old")
}

func TestPinnedComparisonChangesOnlyCSV(t *testing.T) {
	t.Chdir(t.TempDir())
	app := NewApp()
	app.view.Init()
	app.setKeybindings()
	press := func(r rune) { processKey(app, tcell.KeyRune, r) }

	existing := models.Connection{ProcessName: "persistent", Laddr: ":8000"}
	app.applyRefresh(1, []models.Connection{existing, {ProcessName: "vanished", Laddr: ":8001"}}, time.Now())
	press('B')
	app.applyRefresh(2, []models.Connection{
		existing,
		{ProcessName: "zulu", Laddr: ":8002"},
		{ProcessName: "alpha", Laddr: ":8003"},
	}, time.Now())
	press('s')
	press('C')

	assertExport := func(want ...string) {
		t.Helper()
		press('e')
		processKey(app, tcell.KeyEnter, 0)
		file, err := os.Open("batstat_export.csv")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		records, err := csv.NewReader(file).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, record := range records[1:] {
			got = append(got, record[0])
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("exported processes = %v, want %v", got, want)
		}
	}
	assertExport("alpha", "zulu")
	press('P')
	assertExport("alpha", "persistent", "zulu")
	press('P')
	app.view.filterInput.SetText("vanished")
	press('e')
	if got := app.view.hintView.GetText(true); app.view.IsModalActive() || !strings.Contains(got, "No connections to export") {
		t.Fatalf("expected empty-export feedback for vanished-only results, got %q", got)
	}
}

func TestPinnedComparisonChangesOnlyShortcut(t *testing.T) {
	app := NewApp()
	app.view.Init()
	app.setKeybindings()
	press := func(r rune) { processKey(app, tcell.KeyRune, r) }
	assertHint := func(want string) {
		t.Helper()
		if got := app.view.hintView.GetText(true); !strings.Contains(got, want) {
			t.Fatalf("key hints = %q, missing %q", got, want)
		}
	}
	assertLiveCount := func(want int) {
		t.Helper()
		if got := app.view.table.GetRowCount() - 1; got != want {
			t.Fatalf("live rows = %d, want %d", got, want)
		}
	}

	press('C')
	assertHint("Press B to pin")
	existing := models.Connection{ProcessName: "persistent", Laddr: ":8000"}
	app.applyRefresh(1, []models.Connection{existing}, time.Now())
	press('B')
	assertHint("C Changes only")
	press('h')
	_, help := app.view.pages.GetFrontPage()
	helpText := help.(*tview.Frame).GetPrimitive().(*tview.TextView).GetText(true)
	if !strings.Contains(helpText, "Toggle Changes only in the pinned comparison") {
		t.Fatalf("help omits C: %s", helpText)
	}
	processKey(app, tcell.KeyEscape, 0)

	app.applyRefresh(2, []models.Connection{existing, {ProcessName: "new", Laddr: ":8001"}}, time.Now())
	press(' ')
	press('C')
	assertLiveCount(1)
	press('b')
	assertLiveCount(2)
	press('C')
	assertHint("Press b")
	press('b')
	assertHint("C Changes only")
	assertLiveCount(1)

	for _, shortcut := range []rune{'P', 'H'} {
		press(shortcut)
		press('C')
		press(shortcut)
		assertHint("C Changes only")
		assertLiveCount(1)
	}
	press('B')
	assertLiveCount(0)
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
