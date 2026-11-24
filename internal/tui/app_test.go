package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
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

func TestListeningShortcutFiltersPausedTablesAndCSV(t *testing.T) {
	t.Chdir(t.TempDir())
	app := NewApp()
	app.view.Init()
	app.setKeybindings()
	press := func(r rune) {
		t.Helper()
		if event := app.tviewApp.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone)); event != nil {
			t.Fatalf("shortcut %q was not consumed", r)
		}
	}

	oldListener := models.Connection{ProcessName: "old", Pid: 1234, Type: "TCP", Status: "LISTEN", Family: "IPv4", Laddr: "127.0.0.1:80"}
	liveListener := models.Connection{ProcessName: "new", Pid: 1234, Type: "TCP", Status: "LISTEN", Family: "IPv6", Laddr: ":::80"}
	established := models.Connection{ProcessName: "client", Pid: 1234, Type: "TCP", Status: "ESTABLISHED", Family: "IPv4", Laddr: "127.0.0.1:8080"}
	udp := models.Connection{ProcessName: "udp", Pid: 1234, Type: "UDP", Status: "NONE", Family: "IPv4", Laddr: "0.0.0.0:53"}
	at := time.Now()
	app.applyRefresh(1, []models.Connection{oldListener, established}, at)
	app.view.filterInput.SetText("state:established")
	press('B')
	app.applyRefresh(2, []models.Connection{liveListener, udp}, at.Add(time.Second))
	app.setPaused(true)

	filter := app.view.filterInput
	press('/')
	if event := app.tviewApp.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'L', tcell.ModNone)); event == nil {
		t.Fatal("L must pass through to the filter input while it has focus")
	}
	filter.SetText("pid:1234 proto:tcp state:listen")
	if got := app.state.GetFilteredConnections(); len(got) != 1 || got[0] != liveListener {
		t.Fatalf("compound filter returned %v", got)
	}
	filter.SetText("pid:1234")
	filter.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {
		app.tviewApp.SetFocus(p)
	})
	if event := app.tviewApp.GetInputCapture()(tcell.NewEventKey(tcell.KeyRune, 'l', tcell.ModNone)); event == nil {
		t.Fatal("lowercase l must pass through to the table")
	}
	press('L')
	if filter.GetText() != "pid:1234" || !strings.Contains(filter.GetLabel(), "Listening only") {
		t.Fatalf("toggle changed the query or omitted its indicator: %q %q", filter.GetLabel(), filter.GetText())
	}
	for _, view := range []string{"pinned", "previous refresh"} {
		live, vanished, _ := app.state.displayedRows()
		if len(live) != 1 || live[0].connection != liveListener || len(vanished) != 1 || vanished[0] != oldListener {
			t.Fatalf("%s comparison returned live %v, vanished %v", view, live, vanished)
		}
		if liveStatus, vanishedStatus := app.view.table.GetCell(1, 3).Text, app.view.vanishedTable.GetCell(1, 3).Text; liveStatus != "LISTEN · All interfaces" || vanishedStatus != "LISTEN · Loopback" {
			t.Fatalf("%s table status = %q, %q", view, liveStatus, vanishedStatus)
		}
		press('b')
	}
	if got := app.view.detailsView.GetText(true); !strings.Contains(got, "LISTEN · All interfaces") || !strings.Contains(got, "IPv6") || !strings.Contains(got, ":::80") {
		t.Fatalf("listener details = %s", got)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	draw := func(width int) []string {
		screen.SetSize(width, 24)
		app.view.pages.SetRect(0, 0, width, 24)
		screen.Clear()
		app.view.pages.Draw(screen)
		screen.Show()
		cells, columns, rows := screen.GetContents()
		rendered := make([]string, rows)
		for y := range rows {
			var line strings.Builder
			for _, cell := range cells[y*columns : (y+1)*columns] {
				line.WriteString(string(cell.Runes))
			}
			rendered[y] = line.String()
		}
		return rendered
	}
	rendered := draw(80)
	for _, test := range []struct {
		table *tview.Table
		want  []string
	}{
		{app.view.table, []string{"LISTEN · All interfaces", "IPv6", ":::80"}},
		{app.view.vanishedTable, []string{"LISTEN · Loopback", "IPv4", "127.0.0.1:80"}},
	} {
		_, top, _, height := test.table.GetRect()
		rows := strings.Join(rendered[top:top+height], "\n")
		for _, want := range test.want {
			if !strings.Contains(rows, want) {
				t.Errorf("80-column terminal did not show %q in table:\n%s", want, rows)
			}
		}
	}
	filter.SetText("state:established")
	if live, vanished, _ := app.state.displayedRows(); len(live) != 0 || len(vanished) != 0 {
		t.Fatalf("conflicting filters returned live %v, vanished %v", live, vanished)
	}
	filter.SetText("pid:")
	if feedback := app.view.baselineView.GetText(true); len(app.state.GetFilteredConnections()) != 0 || !strings.Contains(feedback, "Invalid PID") {
		t.Fatalf("invalid query left results or had no error: %q", feedback)
	}
	filter.SetText("proto:icmp")
	narrow := strings.Join(draw(40), "\n")
	for _, want := range []string{"Listening only", "proto:icmp", "Expected TCP or UDP"} {
		if !strings.Contains(narrow, want) {
			t.Errorf("40-column terminal did not show %q", want)
		}
	}
	filter.SetText("pid:1234")
	if feedback := app.view.baselineView.GetText(true); strings.Contains(feedback, "Filter error") {
		t.Fatalf("corrected query retained error: %q", feedback)
	}
	press('e')
	app.view.pages.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) {
		app.tviewApp.SetFocus(p)
	})
	export, err := os.ReadFile("batstat_export.csv")
	if err != nil {
		t.Fatal(err)
	}
	want := "ProcessName,PID,Status,Family,Type,LocalAddr,RemoteAddr\nnew,1234,LISTEN,IPv6,TCP,:::80,\n"
	if string(export) != want {
		t.Fatalf("CSV export = %q, want %q", export, want)
	}
	press('L')
	if got := app.state.GetFilteredConnections(); len(got) != 2 || filter.GetText() != "pid:1234" || strings.Contains(filter.GetLabel(), "Listening only") {
		t.Fatalf("turning the toggle off returned %v with filter %q %q", got, filter.GetLabel(), filter.GetText())
	}
}
