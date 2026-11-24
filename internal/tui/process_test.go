package tui

import (
	"encoding/csv"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestProcessGroupCountsAndOrder(t *testing.T) {
	groups := groupProcesses([]connectionRow{
		{connection: models.Connection{Pid: 12, ProcessName: "Browser", Family: "IPv4", Type: "TCP", Status: "ESTABLISHED"}},
		{connection: models.Connection{Pid: 12, ProcessName: "Browser", Family: "IPv6", Type: "TCP", Status: "LISTEN"}},
		{connection: models.Connection{Pid: 12, Family: "IPv4", Type: "UDP", Status: "NONE"}},
		{connection: models.Connection{Pid: 15, ProcessName: "Browser", Family: "Unix", Type: "TCP", Status: "ESTABLISHED"}},
		{connection: models.Connection{Pid: 14, ProcessName: "Browser"}},
		{connection: models.Connection{Pid: 19, ProcessName: "Agent"}},
		{connection: models.Connection{Pid: 20}},
		{connection: models.Connection{Pid: -1}},
		{connection: models.Connection{}},
		{connection: models.Connection{}},
		{connection: models.Connection{}},
	})
	var order []int32
	for _, group := range groups {
		order = append(order, group.pid)
	}
	if want := []int32{12, 19, 14, 15, 20, 0}; !slices.Equal(order, want) {
		t.Fatalf("group order = %v, want %v", order, want)
	}
	if first := groups[0]; first.label() != "Browser" || len(first.connections) != 3 || first.established != 1 || first.listeners != 1 {
		t.Fatalf("unexpected Browser counts: %+v", first)
	}
	if groups[3].established != 0 || groups[4].label() != "Name unavailable" || groups[4].pidLabel() != "20" {
		t.Fatal("Unix sockets and named PIDs with missing names should stay separately attributable")
	}
	if unknown := groups[len(groups)-1]; unknown.label() != "Unknown owner" || unknown.pidLabel() != "" || len(unknown.connections) != 4 {
		t.Fatalf("unexpected unknown owner group: %+v", unknown)
	}
}

func TestProcessViewSharesFiltersDetailsAndCSV(t *testing.T) {
	t.Chdir(t.TempDir())
	app := NewApp()
	app.view.Init()
	app.setKeybindings()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(160, 32)
	app.view.pages.SetRect(0, 0, 160, 32)
	listener := models.Connection{Pid: 10, ProcessName: "browser", Family: "IPv4", Type: "TCP", Status: "LISTEN", Laddr: "127.0.0.1:8080"}
	client := models.Connection{Pid: 10, ProcessName: "browser", Family: "IPv4", Type: "TCP", Status: "ESTABLISHED", Laddr: "192.0.2.2:8081", Raddr: "192.0.2.3:443"}
	udp := models.Connection{Pid: 10, ProcessName: "browser", Family: "IPv4", Type: "UDP", Status: "NONE", Laddr: "192.0.2.2:8082"}
	vanished := models.Connection{Pid: 20, ProcessName: "old", Family: "IPv4", Type: "TCP", Laddr: "192.0.2.2:9000"}
	applyProcessSnapshot(t, app, []models.Connection{vanished})
	processKey(app, tcell.KeyRune, 'B')
	applyProcessSnapshot(t, app, []models.Connection{listener, client, udp})
	processKey(app, tcell.KeyTab, 0)
	if !app.view.vanishedTable.HasFocus() {
		t.Fatal("expected vanished table to have focus")
	}
	processKey(app, tcell.KeyRune, ' ')
	processKey(app, tcell.KeyRune, 'P')
	p := &app.view.processes
	if !app.paused || !p.table.HasFocus() || len(p.rows) != 1 || p.table.GetCell(1, 2).Text != "3" {
		t.Fatal("Process View should open on the paused snapshot with the group collapsed")
	}
	processKey(app, tcell.KeyRune, 'k')
	if app.view.IsModalActive() || !strings.Contains(app.view.hintView.GetText(true), "select an endpoint") {
		t.Fatal("process group must prompt for an endpoint")
	}
	processKey(app, tcell.KeyRune, 'e')
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
	if len(records) != 4 || records[1][2] != "LISTEN" || records[2][5] != client.Laddr || records[3][4] != "UDP" || !p.table.HasFocus() {
		t.Fatalf("unexpected export or focus: %v", records)
	}
	processKey(app, tcell.KeyRune, 'e')
	processKey(app, tcell.KeyTab, 0)
	processKey(app, tcell.KeyEnter, 0)
	pathField, ok := app.tviewApp.GetFocus().(*tview.InputField)
	if !ok {
		t.Fatal("custom export should focus its path field")
	}
	app.view.pages.Draw(screen)
	for _, r := range "custom.csv" {
		processKey(app, tcell.KeyRune, r)
	}
	if pathField.GetText() != "custom.csv" {
		t.Fatalf("custom path field contains %q", pathField.GetText())
	}
	processKey(app, tcell.KeyTab, 0)
	if button, ok := app.tviewApp.GetFocus().(*tview.Button); !ok || button.GetLabel() != "Save" {
		t.Fatalf("custom export did not move to Save: %T", app.tviewApp.GetFocus())
	}
	processKey(app, tcell.KeyEnter, 0)
	if _, err := os.Stat("custom.csv"); err != nil || !p.table.HasFocus() {
		t.Fatalf("custom export did not save or restore focus: %v", err)
	}

	processKey(app, tcell.KeyRune, '/')
	app.view.filterInput.SetText("pid:10")
	processKey(app, tcell.KeyEnter, 0)
	processKey(app, tcell.KeyRune, 'L')
	if !p.table.HasFocus() || len(p.rows) != 1 || p.table.GetCell(1, 2).Text != "1" || p.table.GetCell(1, 3).Text != "0" || p.table.GetCell(1, 4).Text != "1" {
		t.Fatal("shared filter and listener toggle should narrow the process counts")
	}
	processKey(app, tcell.KeyEnter, 0)
	if len(p.rows) != 2 || p.table.GetCell(2, 5).Text != "LISTEN · Loopback" {
		t.Fatal("Enter should reveal the matching TCP listener with its binding label")
	}
	app.view.pages.Draw(screen)
	if _, _, width := p.table.GetCell(2, 9).GetLastPosition(); width == 0 {
		t.Fatal("the expanded process table should render the remote endpoint column")
	}
	processKey(app, tcell.KeyDown, 0)
	if got := app.view.GetSelectedConnection(); got == nil || *got != listener || !strings.Contains(app.view.detailsView.GetText(true), listener.Laddr) {
		t.Fatal("selecting an endpoint should show its connection details")
	}
	processKey(app, tcell.KeyRune, 'K')
	if app.view.IsModalActive() || !strings.Contains(app.view.hintView.GetText(true), "Resume and wait for a refresh") {
		t.Fatal("termination must still require a fresh snapshot")
	}
	processKey(app, tcell.KeyEnter, 0)
	if page, _ := app.view.pages.GetFrontPage(); page != "details_modal" {
		t.Fatalf("endpoint Enter opened %q", page)
	}
	processKey(app, tcell.KeyEscape, 0)
	if !p.table.HasFocus() {
		t.Fatal("closing endpoint details should restore Process View focus")
	}
	processKey(app, tcell.KeyRune, 'b')
	processKey(app, tcell.KeyRune, 'L')
	app.view.filterInput.SetText("")
	processKey(app, tcell.KeyRune, 'P')
	if !app.view.vanishedTable.HasFocus() || !strings.Contains(app.view.baselineView.GetText(true), "Baseline: Pinned") {
		t.Fatal("returning to Connections should restore the pinned comparison and vanished focus")
	}
}

func TestProcessViewKeepsExpansionAndSelectionAcrossRefreshes(t *testing.T) {
	app := NewApp()
	app.view.Init()
	app.setKeybindings()
	first := models.Connection{Pid: 10, ProcessName: "browser", Family: "IPv4", Type: "TCP", Laddr: "192.0.2.10:8000", Raddr: "192.0.2.3:443"}
	second := first
	second.Laddr = "192.0.2.10:8001"
	other := models.Connection{Pid: 20, ProcessName: "other", Laddr: "192.0.2.20:9000"}
	applyProcessSnapshot(t, app, []models.Connection{first, second, other})
	processKey(app, tcell.KeyRune, 'P')
	processKey(app, tcell.KeyEnter, 0)
	processKey(app, tcell.KeyDown, 0)
	processKey(app, tcell.KeyDown, 0)

	applyProcessSnapshot(t, app, []models.Connection{second, first, other, other, other})
	p := &app.view.processes
	if p.rows[0].group.pid != 20 || !p.expanded[10] || p.selectedRow().connection == nil || *p.selectedRow().connection != second {
		t.Fatal("selected endpoint should follow its PID when both groups and endpoints move")
	}
	app.state.SetSort(6, true)
	processKey(app, tcell.KeyRune, 'S')
	if p.rows[2].connection == nil || *p.rows[2].connection != second || p.selectedRow().connection == nil || *p.selectedRow().connection != second {
		t.Fatal("sorting should reorder endpoints and retain the selection")
	}
	applyProcessSnapshot(t, app, []models.Connection{first, other, other})
	if selected := p.selectedRow(); selected.group == nil || selected.group.pid != 10 || selected.connection != nil {
		t.Fatal("a vanished selected endpoint should fall back to its group")
	}
	app.view.filterInput.SetText("pid:20")
	if p.selectedRow().group.pid != 20 || !p.expanded[10] {
		t.Fatal("a filtered group should give up selection while retaining expansion")
	}
	app.view.filterInput.SetText("")
	processKey(app, tcell.KeyRune, 'P')
	processKey(app, tcell.KeyRune, 'P')
	if !p.expanded[10] || p.selectedRow().group.pid != 20 {
		t.Fatal("switching views should keep the process selection and expansion")
	}
	processKey(app, tcell.KeyRune, 'P')
	applyProcessSnapshot(t, app, []models.Connection{other})
	applyProcessSnapshot(t, app, []models.Connection{first, other})
	processKey(app, tcell.KeyRune, 'P')
	if p.expanded[10] || len(p.rows) != 2 {
		t.Fatal("an absent PID should lose its expansion even while viewing Connections")
	}
}

func TestProcessViewFallsBackToGroupWhenSelectedSocketDisappears(t *testing.T) {
	for _, view := range []string{"Process View", "Connections"} {
		t.Run(view, func(t *testing.T) {
			app := NewApp()
			app.view.Init()
			app.setKeybindings()
			first := models.Connection{Pid: 10, Fd: 8, Family: "IPv4", Type: "UDP", Laddr: "192.0.2.10:8000"}
			second := first
			second.Fd = 9
			applyProcessSnapshot(t, app, []models.Connection{first, second})
			processKey(app, tcell.KeyRune, 'P')
			processKey(app, tcell.KeyEnter, 0)
			processKey(app, tcell.KeyDown, 0)
			processKey(app, tcell.KeyDown, 0)
			if selected := app.view.GetSelectedConnection(); selected == nil || selected.Fd != 9 {
				t.Fatal("expected to select the second socket with the same endpoints")
			}
			applyProcessSnapshot(t, app, []models.Connection{second, first})
			if selected := app.view.GetSelectedConnection(); selected == nil || selected.Fd != 9 {
				t.Fatal("selection should follow the same socket when matching endpoints reorder")
			}
			if view == "Connections" {
				processKey(app, tcell.KeyRune, 'P')
			}
			applyProcessSnapshot(t, app, []models.Connection{first})
			applyProcessSnapshot(t, app, []models.Connection{first, second})
			if view == "Connections" {
				processKey(app, tcell.KeyRune, 'P')
			}
			selected := app.view.processes.selectedRow()
			if selected.group == nil || selected.group.pid != 10 || selected.connection != nil {
				t.Fatal("selection should return to the expanded group when the selected socket disappears")
			}
		})
	}
}

func TestProcessViewCyclesThroughEndpointSortColumns(t *testing.T) {
	app := NewApp()
	app.view.Init()
	app.setKeybindings()
	listener := models.Connection{Pid: 10, Family: "IPv4", Type: "TCP", Status: "LISTEN"}
	client := models.Connection{Pid: 10, Family: "IPv4", Type: "TCP", Status: "ESTABLISHED"}
	applyProcessSnapshot(t, app, []models.Connection{listener, client})
	processKey(app, tcell.KeyRune, 'P')
	processKey(app, tcell.KeyEnter, 0)
	p := &app.view.processes
	processKey(app, tcell.KeyRune, 's')
	if p.rows[1].connection.Status != "ESTABLISHED" || !strings.Contains(p.table.GetCell(0, 5).Text, "▲") {
		t.Fatal("the first sort cycle should sort endpoints by status and mark its header")
	}
	processKey(app, tcell.KeyRune, 'S')
	if p.rows[1].connection.Status != "LISTEN" || !strings.Contains(p.table.GetCell(0, 5).Text, "▼") {
		t.Fatal("reversing sort should reverse endpoint order and update the header")
	}
}

func applyProcessSnapshot(t *testing.T, app *App, conns []models.Connection) {
	t.Helper()
	id, ok := app.beginRefresh()
	if !ok {
		t.Fatal("expected a refresh to start")
	}
	app.applyRefresh(id, conns, time.Unix(int64(id), 0))
}

func processKey(app *App, key tcell.Key, r rune) {
	event := app.tviewApp.GetInputCapture()(tcell.NewEventKey(key, r, tcell.ModNone))
	if event != nil {
		app.view.pages.InputHandler()(event, func(p tview.Primitive) { app.tviewApp.SetFocus(p) })
	}
}
