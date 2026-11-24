package tui

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type processGroup struct {
	pid         int32
	name        string
	established int
	listeners   int
	connections []models.Connection
}

func processGroupPID(c models.Connection) int32 {
	return max(c.Pid, 0)
}

func groupProcesses(live []connectionRow) []*processGroup {
	byPID := make(map[int32]*processGroup)
	for _, row := range live {
		c := row.connection
		pid := processGroupPID(c)
		group := byPID[pid]
		if group == nil {
			group = &processGroup{pid: pid}
			byPID[pid] = group
		}
		if c.ProcessName != "" && (group.name == "" || c.ProcessName < group.name) {
			group.name = c.ProcessName
		}
		group.connections = append(group.connections, c)
		if matchesIPProtocol(c, "TCP") && strings.EqualFold(c.Status, "ESTABLISHED") {
			group.established++
		}
		if isTCPListener(c) {
			group.listeners++
		}
	}

	return slices.SortedFunc(maps.Values(byPID), func(a, b *processGroup) int {
		if a.pid == 0 || b.pid == 0 {
			return cmp.Compare(b.pid, a.pid) // Unknown owners follow known PIDs.
		}
		return cmp.Or(
			cmp.Compare(len(b.connections), len(a.connections)),
			strings.Compare(strings.ToLower(a.label()), strings.ToLower(b.label())),
			cmp.Compare(a.pid, b.pid),
		)
	})
}

func (g *processGroup) label() string {
	if g.pid == 0 {
		return "Unknown owner"
	}
	if g.name == "" {
		return "Name unavailable"
	}
	return g.name
}

func (g *processGroup) pidLabel() string {
	if g.pid > 0 {
		return strconv.FormatInt(int64(g.pid), 10)
	}
	return ""
}

type processRow struct {
	group      *processGroup
	connection *models.Connection
}

type processView struct {
	table    *tview.Table
	rows     []processRow
	expanded map[int32]bool
}

func newProcessView() processView {
	return processView{
		table:    tview.NewTable().SetSelectable(true, false).SetFixed(1, 0),
		expanded: make(map[int32]bool),
	}
}

func (p *processView) selectedRow() processRow {
	row, _ := p.table.GetSelection()
	if row < 1 || row > len(p.rows) {
		return processRow{}
	}
	return p.rows[row-1]
}

func sameProcessEndpoint(selected, candidate models.Connection) bool {
	return endpoints(selected) == endpoints(candidate) && (selected.Fd == 0 || selected.Fd == candidate.Fd)
}

func (p *processView) reconcileSnapshot(current []models.Connection) {
	selected := p.selectedRow()
	if len(p.expanded) == 0 && selected.group == nil {
		return
	}
	retainedExpansions := make(map[int32]bool)
	groupPresent, endpointPresent := false, false
	for _, c := range current {
		pid := processGroupPID(c)
		if selected.group != nil && selected.group.pid == pid {
			groupPresent = true
			if selected.connection != nil && sameProcessEndpoint(*selected.connection, c) {
				endpointPresent = true
			}
		}
		if p.expanded[pid] {
			retainedExpansions[pid] = true
		}
	}
	p.expanded = retainedExpansions
	if selected.group != nil && !groupPresent {
		p.table.Select(0, 0)
	} else if selected.connection != nil && !endpointPresent {
		row, _ := p.table.GetSelection()
		for p.rows[row-1].connection != nil {
			row--
		}
		p.table.Select(row, 0)
	}
}

func (p *processView) populate(groups []*processGroup, sortColumn int, ascending bool) {
	previous := p.selectedRow()
	clear(p.rows)
	p.rows = p.rows[:0]
	p.table.Clear()
	headers := []string{"Process", "PID", "Total", "Established", "Listeners"}
	endpointOffset := len(headers)
	for _, group := range groups {
		if p.expanded[group.pid] {
			headers = append(headers, connectionHeaders[firstEndpointColumn:]...)
			break
		}
	}
	for column, header := range headers {
		if column >= endpointOffset && column-endpointOffset+firstEndpointColumn == sortColumn {
			header = sortHeader(header, ascending)
		}
		p.table.SetCell(0, column, tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetSelectable(false))
	}

	var groupRow, endpointRow int
	for _, group := range groups {
		p.rows = append(p.rows, processRow{group: group})
		row := len(p.rows)
		selectedGroup := previous.group != nil && previous.group.pid == group.pid
		if selectedGroup {
			groupRow = row
		}
		indicator := "▸ "
		if p.expanded[group.pid] {
			indicator = "▾ "
		}
		p.addRow(row, tview.Styles.PrimaryTextColor, []string{
			indicator + tview.Escape(truncate(group.label(), 30)), group.pidLabel(), strconv.Itoa(len(group.connections)),
			strconv.Itoa(group.established), strconv.Itoa(group.listeners),
		})
		if !p.expanded[group.pid] {
			continue
		}
		for i := range group.connections {
			c := &group.connections[i]
			p.rows = append(p.rows, processRow{group: group, connection: c})
			row := len(p.rows)
			if selectedGroup && endpointRow == 0 && previous.connection != nil && sameProcessEndpoint(*previous.connection, *c) {
				endpointRow = row
			}
			fields := endpointFields(*c)
			p.addRow(row, getStatusColor(c.Status), append([]string{"  ↳", "", "", "", ""}, fields[:]...))
		}
	}

	selected := cmp.Or(endpointRow, groupRow)
	if selected == 0 && len(p.rows) > 0 {
		selected = 1
	}
	if row, _ := p.table.GetSelection(); selected != row {
		p.table.Select(selected, 0)
	}
}

func (p *processView) addRow(row int, color tcell.Color, values []string) {
	for column, value := range values {
		p.table.SetCell(row, column, tview.NewTableCell(value).SetTextColor(color))
	}
}

func (v *View) activateProcessRow() {
	row := v.processes.selectedRow()
	if row.connection != nil {
		v.showDetailsModal(*row.connection, false)
	} else if row.group != nil {
		pid := row.group.pid
		if v.processes.expanded[pid] {
			delete(v.processes.expanded, pid)
		} else {
			v.processes.expanded[pid] = true
		}
		v.Refresh()
	}
}

func (v *View) populateProcesses(live []connectionRow, problem string) {
	groups := groupProcesses(live)
	v.processes.populate(groups, v.app.state.sortColumn, v.app.state.sortAsc)
	status := fmt.Sprintf("Process View  |  Groups: %d", len(groups))
	if len(groups) == 0 {
		status = "Process View  |  No matching connections"
	}
	v.setListStatus(status, problem)
	v.mainLayout.tableWidth = connectionTableWidth(v.processes.table, false)
}
