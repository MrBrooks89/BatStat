package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (v *View) populateTable() {
	connections := v.app.state.GetFilteredConnections()
	v.table.Clear()

	headers := []string{"No", "Process", "PID", "Status", "Family", "Type", "Local Addr", "Remote Addr"}
	for i, h := range headers {
		cell := tview.NewTableCell(h).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetAlign(tview.AlignCenter).
			SetSelectable(false)
		v.table.SetCell(0, i, cell)
	}

	for r, conn := range connections {
		rowItems := []string{
			strconv.Itoa(r + 1),
			conn.ProcessName,
			strconv.Itoa(int(conn.Pid)),
			conn.Status,
			conn.Family,
			conn.Type,
			conn.Laddr,
			conn.Raddr,
		}
		for c, item := range rowItems {
			cell := tview.NewTableCell(truncate(item, 30)).
				SetExpansion(1).
				SetTextColor(getStatusColor(conn.Status))
			v.table.SetCell(r+1, c, cell)
		}
	}
}

func (v *View) updateHeaderIndicator() {
	headers := []string{"No", "Process", "PID", "Status", "Family", "Type", "Local Addr", "Remote Addr"}
	for i, h := range headers {
		indicator := ""
		if i == v.app.state.sortColumn {
			indicator = " [yellow]▲"
			if !v.app.state.sortAsc {
				indicator = " [yellow]▼"
			}
		}
		if cell := v.table.GetCell(0, i); cell != nil {
			cell.SetText(h + indicator)
		}
	}
}

func (v *View) updateDetailsView() {
	c := v.GetSelectedConnection()
	if c == nil {
		v.detailsView.Clear().SetText(" [gray]No connection selected")
		return
	}
	v.detailsView.SetText(v.connectionDetails(*c))
}

func (v *View) connectionDetails(c models.Connection) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("[yellow]Process:[white]    %s\n", c.ProcessName))
	builder.WriteString(fmt.Sprintf("[yellow]PID:[white]        %d\n", c.Pid))
	var details models.DetailedInfo
	if !v.app.awaitingLiveData {
		details = models.GetDetailedInfo(c.Pid)
		builder.WriteString(fmt.Sprintf("[yellow]User:[white]       %s\n\n", details.Username))
	}
	builder.WriteString(fmt.Sprintf("[yellow]Status:[white]     %s\n", c.Status))
	builder.WriteString(fmt.Sprintf("[yellow]Local Addr:[white] %s\n", c.Laddr))
	builder.WriteString(fmt.Sprintf("[yellow]Remote Addr:[white] %s\n\n", c.Raddr))

	if v.app.awaitingLiveData {
		builder.WriteString("[gray]Live user and command details return after a refresh.")
	} else {
		builder.WriteString(fmt.Sprintf("[yellow]Command:[white]\n%s\n", details.Cmdline))
	}
	return builder.String()
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
