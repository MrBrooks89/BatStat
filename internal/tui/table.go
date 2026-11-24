package tui

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	liveTableBorders     = true
	vanishedTableBorders = false
	firstEndpointColumn  = 3
)

var connectionHeaders = [...]string{"No", "Process", "PID", "Status", "Family", "Type", "Local Addr", "Remote Addr"}

func (v *View) populateLiveTable(live []connectionRow, pinned bool) {
	table := v.table
	table.Clear()
	setConnectionHeaders(table, v.app.state.sortColumn, v.app.state.sortAsc)
	for i, row := range live {
		number := strconv.Itoa(i + 1)
		if row.isNew {
			if pinned {
				number = "New since pin"
			} else {
				number = "NEW"
			}
		}
		addConnectionRow(table, i+1, row.connection, number)
	}
	clampConnectionSelection(table)
}

func (v *View) populateVanishedTable(vanished []models.Connection, pinTime time.Time, filterProblem string) {
	title := "Recently vanished"
	baseline := "Baseline: Previous refresh"
	if !pinTime.IsZero() {
		title = "Vanished since pin"
		baseline = "Baseline: Pinned at " + pinTime.Format("2006-01-02 15:04:05")
	}
	label := fmt.Sprintf("%s (%d)", title, len(vanished))
	if v.vanishedCollapsed && len(vanished) > 0 {
		label += " · collapsed"
	}
	v.setListStatus(baseline+"  |  "+label, filterProblem)

	v.vanishedVisible = len(vanished) > 0 && !v.vanishedCollapsed
	table := v.vanishedTable
	table.Clear()
	if !v.vanishedVisible {
		v.connectionLists.ResizeItem(table, 0, 0)
		if table.HasFocus() {
			v.app.tviewApp.SetFocus(v.table)
		}
		return
	}

	v.connectionLists.ResizeItem(table, 0, 1)
	setConnectionHeaders(table, -1, true)
	table.GetCell(0, 0).SetText("Vanished")
	for i, c := range vanished {
		addConnectionRow(table, i+1, c, strconv.Itoa(i+1))
	}
	clampConnectionSelection(table)
}

func setConnectionHeaders(table *tview.Table, sortColumn int, ascending bool) {
	for i, header := range connectionHeaders {
		if i == sortColumn {
			header = sortHeader(header, ascending)
		}
		table.SetCell(0, i, tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetAlign(tview.AlignCenter).
			SetSelectable(false))
	}
}

func sortHeader(header string, ascending bool) string {
	if ascending {
		return header + " [yellow]▲"
	}
	return header + " [yellow]▼"
}

func endpointFields(c models.Connection) [5]string {
	return [5]string{displayStatus(c), c.Family, c.Type, tview.Escape(c.Laddr), tview.Escape(c.Raddr)}
}

func addConnectionRow(table *tview.Table, row int, c models.Connection, number string) {
	fields := endpointFields(c)
	items := append([]string{number, truncate(c.ProcessName, 30), strconv.FormatInt(int64(c.Pid), 10)}, fields[:]...)
	for column, item := range items {
		table.SetCell(row, column, tview.NewTableCell(item).
			SetExpansion(1).
			SetTextColor(getStatusColor(c.Status)))
	}
}

// Measure every row so scrolling through a snapshot keeps the same pane widths.
func connectionTableWidth(table *tview.Table, borders bool) int {
	columns := table.GetColumnCount()
	if columns == 0 {
		return 0
	}
	width := columns - 1 // One separator between adjacent columns.
	if borders {
		width += 2
	}
	rows := table.GetRowCount()
	for column := range columns {
		var widest int
		for row := range rows {
			widest = max(widest, tview.TaggedStringWidth(table.GetCell(row, column).Text))
		}
		width += widest
	}
	return width
}

func clampConnectionSelection(table *tview.Table) {
	row, _ := table.GetSelection()
	last := table.GetRowCount() - 1
	if last > 0 && (row < 1 || row > last) {
		table.Select(1, 0)
	}
}

func (v *View) selectedConnectionDetails() (*models.Connection, bool) {
	if !v.showProcesses && v.vanishedTable.HasFocus() {
		return v.selectedVanishedConnection(), true
	}
	return v.GetSelectedConnection(), false
}

func (v *View) updateDetailsView() {
	c, vanished := v.selectedConnectionDetails()
	if c == nil {
		if v.showProcesses {
			if group := v.processes.selectedRow().group; group != nil {
				v.detailsView.SetText(fmt.Sprintf("[yellow]%s[white]\nPID: %s\nTotal: %d\nEstablished: %d\nListeners: %d\n\nEnter to expand or collapse.",
					tview.Escape(group.label()), cmp.Or(group.pidLabel(), "Unavailable"), len(group.connections), group.established, group.listeners))
				return
			}
		}
		v.detailsView.Clear().SetText(" [gray]No connection selected")
		return
	}
	v.detailsView.SetText(v.connectionDetails(*c, vanished))
}

func (v *View) connectionDetails(c models.Connection, vanished bool) string {
	var builder strings.Builder
	if vanished {
		builder.WriteString("[gray]Vanished connection · captured details[white]\n\n")
	}
	builder.WriteString(fmt.Sprintf("[yellow]Process:[white]    %s\n", c.ProcessName))
	builder.WriteString(fmt.Sprintf("[yellow]PID:[white]        %d\n", c.Pid))
	live := !vanished && !v.app.awaitingLiveData
	builder.WriteString(fmt.Sprintf("[yellow]Status:[white]     %s\n", displayStatus(c)))
	builder.WriteString(fmt.Sprintf("[yellow]Family:[white]     %s\n", c.Family))
	builder.WriteString(fmt.Sprintf("[yellow]Local Addr:[white] %s\n", c.Laddr))
	builder.WriteString(fmt.Sprintf("[yellow]Remote Addr:[white] %s\n\n", c.Raddr))

	if live {
		details := models.GetDetailedInfo(c.Pid)
		builder.WriteString(fmt.Sprintf("[yellow]User:[white]       %s\n\n", details.Username))
		builder.WriteString(fmt.Sprintf("[yellow]Command:[white]\n%s\n", details.Cmdline))
	} else if !vanished {
		builder.WriteString("[gray]Live user and command details return after a refresh.")
	}
	return builder.String()
}

func displayStatus(c models.Connection) string {
	if !isTCPListener(c) {
		return c.Status
	}
	addr := c.LocalIP()
	switch {
	case addr.IsLoopback():
		return c.Status + " · Loopback"
	case addr.IsUnspecified():
		return c.Status + " · All interfaces"
	default:
		return c.Status
	}
}

func getStatusColor(status string) tcell.Color {
	switch status {
	case "ESTABLISHED":
		return tcell.ColorGreen
	case "LISTEN":
		return tcell.ColorYellow
	case "CLOSE_WAIT", "TIME_WAIT":
		return tcell.ColorOrangeRed
	case "NONE", "":
		return tview.Styles.PrimaryTextColor
	default:
		return tcell.ColorIndianRed
	}
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
