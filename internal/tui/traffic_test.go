package tui

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestInterfaceTrafficRatesAndGaps(t *testing.T) {
	var traffic interfaceTraffic
	at := time.Unix(100, 0)
	record := func(delay time.Duration, reading map[string]interfaceCounters) {
		at = at.Add(delay)
		traffic.record(at, reading)
	}
	requireRate := func(want trafficRate) {
		t.Helper()
		if got := traffic.history()[trafficSlots-1]; got != want {
			t.Fatalf("current rate at %v = %+v, want %+v", at, got, want)
		}
	}
	report := func(received, sent uint64, index int) map[string]interfaceCounters {
		return map[string]interfaceCounters{"eth0": {
			received: received, sent: sent, index: index, flags: net.FlagUp | net.FlagRunning,
		}}
	}

	record(0, report(1000, 2000, 1))
	requireRate(trafficRate{})
	record(4*time.Second, report(3000, 2500, 1))
	requireRate(trafficRate{received: 4000, sent: 1000, valid: true})
	record(refreshInterval, nil)
	requireRate(trafficRate{})
	if _, ok := traffic.interfaces["eth0"]; !ok {
		t.Fatal("failed read discarded the last known list of interfaces")
	}
	record(refreshInterval, report(9000, 6000, 1))
	requireRate(trafficRate{})
	record(refreshInterval, report(9000, 6000, 1))
	requireRate(trafficRate{valid: true})
	record(refreshInterval, report(3, 6200, 1))
	requireRate(trafficRate{})
	record(refreshInterval, report(300, 6400, 2))
	requireRate(trafficRate{})
	record(refreshInterval, map[string]interfaceCounters{})
	requireRate(trafficRate{})
	if traffic.selected != "eth0" || len(traffic.interfaces) != 0 {
		t.Fatalf("disappeared interface was not kept as the selection: %+v", traffic)
	}
	record(refreshInterval, report(600, 6800, 2))
	requireRate(trafficRate{})
	record(refreshInterval, report(900, 7100, 2))
	requireRate(trafficRate{received: 800, sent: 800, valid: true})
	if first := traffic.history()[trafficSlots-9]; !first.valid || first.received != 4000 {
		t.Fatalf("earlier traffic disappeared from the minute of history: %+v", first)
	}
	record(trafficWindow, report(900, 7100, 2))
	if len(traffic.samples) != 1 || traffic.history()[trafficSlots-2].valid {
		t.Fatal("expired rates remained in the minute of history")
	}
}

func TestInterfaceTrafficInitialSelection(t *testing.T) {
	interfaces := map[string]interfaceCounters{
		"aaa":  {},
		"eth0": {flags: net.FlagUp},
		"eth1": {flags: net.FlagUp},
		"lo":   {flags: net.FlagUp | net.FlagLoopback},
	}
	for _, test := range []struct {
		name, want string
		interfaces map[string]interfaceCounters
		rates      map[string]trafficRate
	}{
		{"busiest", "eth1", interfaces, map[string]trafficRate{"eth0": {received: 10}, "eth1": {sent: 20}, "lo": {received: 100}}},
		{"quiet", "eth0", interfaces, nil},
		{"loopback only", "lo", map[string]interfaceCounters{"lo": interfaces["lo"]}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			traffic := interfaceTraffic{interfaces: test.interfaces}
			if got := traffic.initialInterface(test.rates); got != test.want {
				t.Fatalf("initial interface = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInterfaceTrafficLinkLoss(t *testing.T) {
	var traffic interfaceTraffic
	at := time.Unix(100, 0)
	record := func(step int, received uint64, flags net.Flags) {
		traffic.record(at.Add(time.Duration(step)*refreshInterval), map[string]interfaceCounters{
			"eth0": {received: received, index: 1, flags: flags},
		})
	}
	const active = net.FlagUp | net.FlagRunning
	record(0, 0, active)
	record(1, 300, active)
	record(2, 300, net.FlagUp)
	if traffic.available("eth0") || traffic.history()[trafficSlots-1].valid {
		t.Fatal("a disconnected interface with counters appeared active")
	}
	record(3, 600, active)
	if !traffic.available("eth0") || traffic.history()[trafficSlots-1].valid {
		t.Fatal("first reading after reconnect should establish a new baseline")
	}
	record(4, 900, active)
	if current := traffic.history()[trafficSlots-1]; !current.valid || current.received != 800 {
		t.Fatalf("second reading after reconnect = %+v, want 800 b/s", current)
	}
}

func TestInterfaceTrafficCompressedGraphKeepsGaps(t *testing.T) {
	active := trafficRate{received: 100, valid: true}
	receive, send := trafficGraphs([]trafficRate{active, {}, active, active, {}}, 3)
	if receive != " █ " || send != " ▁ " {
		t.Fatalf("narrow graph shows RX %q and TX %q", receive, send)
	}
}

func TestInterfaceTrafficStripAndPicker(t *testing.T) {
	app := NewApp()
	app.view.Init()
	app.setKeybindings()
	at := time.Unix(100, 0)
	app.applyRefresh(1, []models.Connection{{ProcessName: "example", Laddr: ":8000"}}, at)
	app.traffic.record(at, map[string]interfaceCounters{"eth0": {}, "eth1": {}})
	app.traffic.record(at.Add(refreshInterval), map[string]interfaceCounters{
		"eth0": {received: 375000}, "eth1": {received: 375, sent: 3750},
	})
	app.setPaused(true)
	app.traffic.record(at.Add(2*refreshInterval), map[string]interfaceCounters{
		"eth0": {received: 1125000}, "eth1": {received: 750, sent: 7500},
	})
	if live, _, _ := app.state.displayedRows(); len(live) != 1 || live[0].connection.ProcessName != "example" {
		t.Fatalf("sampling traffic changed the paused connection snapshot: %v", live)
	}

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	draw := func(width, height int) (string, int) {
		screen.SetSize(width, height)
		app.view.pages.SetRect(0, 0, width, height)
		screen.Clear()
		app.view.pages.Draw(screen)
		screen.Show()
		_, _, _, paneHeight := app.view.mainLayout.GetRect()
		cells, _, _ := screen.GetContents()
		var strip strings.Builder
		for row := paneHeight; row < height-footerHeight; row++ {
			for _, cell := range cells[row*width : (row+1)*width] {
				strip.WriteString(string(cell.Runes))
			}
			strip.WriteByte('\n')
		}
		return strip.String(), paneHeight
	}
	if strip, height := draw(75, 20); height != 15 || !strings.Contains(strip, "eth0  RX 2 Mb/s") ||
		!strings.Contains(strip, "TX 0 b/s") || !strings.Contains(strip, "▅█") || !strings.Contains(strip, "▁▁") {
		t.Fatalf("two-line strip at pane height %d:\n%s", height, strip)
	}

	press := func(key tcell.Key, r rune) {
		event := tcell.NewEventKey(key, r, tcell.ModNone)
		if event = app.tviewApp.GetInputCapture()(event); event != nil {
			app.view.pages.InputHandler()(event, func(p tview.Primitive) { app.tviewApp.SetFocus(p) })
		}
	}
	press(tcell.KeyRune, 'P')
	press(tcell.KeyRune, 'H')
	press(tcell.KeyTab, 0)
	press(tcell.KeyRune, 'i')
	draw(75, 20)
	list, ok := app.tviewApp.GetFocus().(*tview.List)
	if !ok || list.GetCurrentItem() != 0 {
		t.Fatalf("picker did not focus the selected interface: %T", app.tviewApp.GetFocus())
	}
	press(tcell.KeyDown, 0)
	press(tcell.KeyEnter, 0)
	if app.traffic.selected != "eth1" || app.tviewApp.GetFocus() != app.view.detailsView {
		t.Fatal("selecting eth1 did not restore focus to the History timeline")
	}
	app.view.filterInput.SetText("no match")
	if strip, height := draw(75, 19); height != 15 || !strings.Contains(strip, "eth1  RX 1 kb/s  TX 10 kb/s") ||
		strings.ContainsAny(strip, "▁█") {
		t.Fatalf("compact strip at pane height %d:\n%s", height, strip)
	}
	if strip, _ := draw(75, 20); !strings.Contains(strip, "▂▂") || !strings.Contains(strip, "██") {
		t.Fatalf("interface picker lost background history or the shared scale:\n%s", strip)
	}
	if strip, _ := draw(24, 18); !strings.Contains(strip, "eth1") || !strings.Contains(strip, "RX1kb/s TX10kb/s") ||
		!app.view.mainLayout.stacked {
		t.Fatalf("compact strip clipped a rate or displaced a pane:\n%s", strip)
	}
	if strip, height := draw(24, 4); height != 1 || strings.Contains(strip, "eth1") {
		t.Fatalf("strip occupied the last connection row:\n%s", strip)
	}
	app.traffic.record(at.Add(3*refreshInterval), map[string]interfaceCounters{"eth0": {}})
	if strip, _ := draw(75, 20); !strings.Contains(strip, "eth1  Unavailable") || !strings.Contains(strip, "██") {
		t.Fatalf("disconnected interface strip:\n%s", strip)
	}
	press(tcell.KeyRune, 'i')
	draw(75, 20)
	list, ok = app.tviewApp.GetFocus().(*tview.List)
	if !ok || list.GetCurrentItem() != 1 {
		t.Fatal("picker did not highlight the disconnected selection")
	}
	name, _ := list.GetItemText(1)
	if !strings.Contains(name, "eth1") || !strings.Contains(name, "(unavailable)") {
		t.Fatalf("picker displayed disconnected interface as %q", name)
	}
}
