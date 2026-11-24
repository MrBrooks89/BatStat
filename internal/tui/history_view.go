package tui

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rivo/tview"
)

type historyView struct {
	table           *tview.Table
	rows            []historyRecord
	detailsID       uint64
	lastSelectionID uint64
}

func (h *historyView) selectedRecord() *historyRecord {
	row, _ := h.table.GetSelection()
	if row < 1 || row > len(h.rows) {
		return nil
	}
	return &h.rows[row-1]
}

func (v *View) populateHistory(problem string) {
	h := &v.history
	selectedID := h.lastSelectionID
	h.lastSelectionID = 0
	if record := h.selectedRecord(); record != nil {
		selectedID = record.id
	}
	rows, untracked := v.app.state.displayedHistory()
	h.rows = rows
	h.table.Clear()
	headers := [...]string{"Record", "Process", "PID", "Last seen", "Local Addr", "Remote Addr"}
	for column, header := range headers {
		h.table.SetCell(0, column, tview.NewTableCell(header).
			SetTextColor(tview.Styles.SecondaryTextColor).
			SetSelectable(false))
	}
	selectedRow := 0
	if len(rows) > 0 {
		selectedRow = 1
	}
	for i, record := range rows {
		c := record.latest
		pid := ""
		if c.Pid > 0 {
			pid = strconv.FormatInt(int64(c.Pid), 10)
		}
		fields := [...]string{
			record.label(), tview.Escape(c.ProcessName), pid, record.lastSeen.Local().Format(time.TimeOnly),
			tview.Escape(c.Laddr), tview.Escape(c.Raddr),
		}
		for column, field := range fields {
			h.table.SetCell(i+1, column, tview.NewTableCell(field))
		}
		if record.id == selectedID {
			selectedRow = i + 1
		}
	}
	if current, _ := h.table.GetSelection(); current != selectedRow {
		h.table.Select(selectedRow, 0)
	}

	status := fmt.Sprintf("Connection History  |  Records: %d", len(rows))
	if untracked > 0 {
		status += fmt.Sprintf("  |  %d live outside history (limit reached)", untracked)
	}
	v.setListStatus(status, problem)
	v.mainLayout.tableWidth = connectionTableWidth(h.table, false)
}

func (h *historyView) releaseRows() {
	if record := h.selectedRecord(); record != nil {
		h.lastSelectionID = record.id
	}
	h.rows = nil
	h.table.Clear()
	h.detailsID = 0
}

func (record historyRecord) label() string {
	if record.firstMissing.IsZero() {
		return "Active"
	}
	return "Vanished"
}

func (v *View) updateHistoryDetails() {
	record := v.history.selectedRecord()
	if record == nil {
		v.history.detailsID = 0
		v.detailsView.SetText(" [gray]No matching history records").ScrollToBeginning()
		return
	}
	v.detailsView.SetText(record.details())
	if record.id != v.history.detailsID {
		v.history.detailsID = record.id
		v.detailsView.ScrollToBeginning()
	}
}

func (record historyRecord) details() string {
	c := record.latest
	var details strings.Builder
	fmt.Fprintf(&details, "[yellow]%s[white]\n", record.label())
	if record.identityUncertain {
		details.WriteString("[yellow]Identity uncertain[white]\n")
	}
	owner := "Unknown owner"
	if c.Pid > 0 {
		owner = fmt.Sprintf("%s (PID %d)", tview.Escape(cmp.Or(c.ProcessName, "Name unavailable")), c.Pid)
	}
	fmt.Fprintf(&details, "%s · %s %s\nLocal: %s\nRemote: %s\nLast state: %s\n\n",
		owner, c.Family, c.Type, tview.Escape(c.Laddr), tview.Escape(c.Raddr),
		tview.Escape(cmp.Or(displayStatus(c), "Unavailable")))
	fmt.Fprintf(&details, "First seen: %s\nLast seen: %s\n",
		record.firstSeen.Local().Format(time.DateTime), record.lastSeen.Local().Format(time.DateTime))
	if !record.firstMissing.IsZero() {
		fmt.Fprintf(&details, "First missing: %s\n", record.firstMissing.Local().Format(time.DateTime))
	}
	details.WriteString("\n[yellow]Observed state changes[white]\n")
	if record.trimmedChanges {
		details.WriteString("[gray]Older changes were trimmed.[white]\n")
	}
	if len(record.changes) == 0 {
		details.WriteString("[gray]None observed.[white]\n")
	}
	for _, change := range record.changes {
		fmt.Fprintf(&details, "%s  %s → %s\n", change.at.Local().Format(time.DateTime),
			tview.Escape(cmp.Or(change.from, "Unavailable")), tview.Escape(cmp.Or(change.to, "Unavailable")))
	}
	return details.String()
}
