package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestConnectionTablesShowRemoteAddresses(t *testing.T) {
	app := NewApp()
	app.view.Init()
	app.awaitingLiveData = true
	current := models.Connection{
		ProcessName: "long-running-process --argument for the connection",
		Pid:         10101,
		Type:        "TCP",
		Status:      "ESTABLISHED",
		Family:      "IPv4",
		Laddr:       "192.0.2.100:54321",
		Raddr:       "198.51.100.200:443",
	}
	listener := models.Connection{Type: "TCP", Status: "LISTEN", Family: "IPv6", Laddr: ":::5355"}
	vanished := current
	vanished.Raddr = "203.0.113.200:443"
	start := time.Unix(0, 0)
	app.state.SetConnections([]models.Connection{vanished, listener}, start)
	app.state.SetConnections([]models.Connection{current, listener}, start.Add(time.Second))
	app.view.Refresh()

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	checkTables := func(width, height int, live, vanished models.Connection) {
		t.Helper()
		screen.SetSize(width, height)
		app.view.pages.SetRect(0, 0, width, height)
		screen.Clear()
		app.view.pages.Draw(screen)
		screen.Show()
		cells, columns, _ := screen.GetContents()
		for _, check := range []struct {
			name  string
			table *tview.Table
			row   models.Connection
		}{
			{"live", app.view.table, live},
			{"vanished", app.view.vanishedTable, vanished},
		} {
			x, y, tableWidth, tableHeight := check.table.GetRect()
			var rendered strings.Builder
			for row := y; row < y+tableHeight; row++ {
				for _, cell := range cells[row*columns+x : row*columns+x+tableWidth] {
					rendered.WriteString(string(cell.Runes))
				}
				rendered.WriteByte('\n')
			}
			for _, field := range []string{"Remote Addr", check.row.Laddr, check.row.Raddr} {
				if !strings.Contains(rendered.String(), field) {
					t.Errorf("%d×%d terminal hid %q from the %s table", width, height, field, check.name)
				}
			}
		}
	}
	checkTables(194, 47, current, vanished)
	checkTables(128, 24, current, vanished)

	ipv6 := current
	ipv6.Family = "IPv6"
	ipv6.Laddr = "2001:db8:1234:5678:abcd:ef01:6789:abcd:54321"
	ipv6.Raddr = "2001:db8:1234:5678:abcd:ef01:6789:ef01:443"
	app.state.SetConnections([]models.Connection{ipv6, listener}, start.Add(2*time.Second))
	app.view.Refresh()
	checkTables(194, 47, ipv6, current)
}

func TestStackingConnectionPanesKeepsSelectionVisible(t *testing.T) {
	app := NewApp()
	app.view.Init()
	app.awaitingLiveData = true
	base := models.Connection{ProcessName: "process", Type: "TCP", Family: "IPv4", Laddr: "192.0.2.100:443"}
	connections := make([]models.Connection, 40)
	for i := range connections {
		connections[i] = base
		connections[i].Raddr = "198.51.100.200:" + strconv.Itoa(10000+i)
	}
	start := time.Unix(0, 0)
	app.state.SetConnections(connections, start)
	app.view.Refresh()
	table := app.view.table
	table.Select(18, 0)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(155, 47)
	app.view.pages.SetRect(0, 0, 155, 47)
	app.view.pages.Draw(screen)
	_, _, _, initialHeight := table.GetRect()
	if _, _, width := table.GetCell(18, 1).GetLastPosition(); width == 0 {
		t.Fatal("selected row was hidden before stacking")
	}

	ipv6 := base
	ipv6.Family = "IPv6"
	ipv6.Laddr = "2001:db8:1234:5678:abcd:ef01:6789:abcd:54321"
	ipv6.Raddr = "2001:db8:1234:5678:abcd:ef01:6789:ef01:443"
	app.state.SetConnections(append(connections, ipv6), start.Add(time.Second))
	app.view.Refresh()
	app.view.pages.Draw(screen)
	if _, _, _, height := table.GetRect(); height >= initialHeight {
		t.Fatal("adding the IPv6 row did not stack the panes")
	}
	row, _ := table.GetSelection()
	if _, _, width := table.GetCell(18, 1).GetLastPosition(); row != 18 || width == 0 {
		t.Fatalf("stacking hid or changed the selected row (selected row %d, cell width %d)", row, width)
	}
}
