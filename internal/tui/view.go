package tui

import (
	"fmt"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	keyHints        = "[yellow]H[white] History [yellow]P[white] Process View [yellow]i[white] Interfaces [yellow]Space[white] Pause/Resume [yellow]/[white] Filter [yellow]L[white] Listening only [yellow]Tab[white] Switch lists [yellow]v[white] Toggle vanished [yellow]B/b[white] Pin/Switch baseline [yellow]e[white] Export [yellow]h[white] Help [yellow]q[white] Quit"
	processKeyHints = "[yellow]H[white] History [yellow]P[white] Connections [yellow]i[white] Interfaces [yellow]Enter[white] Expand/Details [yellow]/[white] Filter [yellow]L[white] Listening only [yellow]Space[white] Pause/Resume [yellow]e[white] Export [yellow]h[white] Help [yellow]q[white] Quit"
	historyKeyHints = "[yellow]H[white] Return [yellow]i[white] Interfaces [yellow]Tab[white] List/Timeline [yellow]/[white] Filter [yellow]L[white] Listening only [yellow]Space[white] Pause/Resume [yellow]h[white] Help [yellow]q[white] Quit"

	minStackedHeight = 15
	footerHeight     = 3 // Status, filter, and key hints.
)

type dashboardLayout struct {
	*tview.Flex
	traffic *trafficView
}

func (layout *dashboardLayout) Draw(screen tcell.Screen) {
	_, _, _, height := layout.GetRect()
	trafficHeight := 1
	if height >= minStackedHeight+footerHeight+2 {
		trafficHeight = 2
	} else if height <= footerHeight+1 {
		trafficHeight = 0
	}
	layout.ResizeItem(layout.traffic, trafficHeight, 0)
	layout.Flex.Draw(screen)
}

type connectionLayout struct {
	*tview.Flex
	listPane   tview.Primitive
	tables     [2]*tview.Table
	tableWidth int
	stacked    bool
}

func (layout *connectionLayout) Draw(screen tcell.Screen) {
	_, _, width, height := layout.GetRect()
	const minDetailsWidth = 36
	// Allow for the extra row used by the compact strip at the stacking
	// threshold so the strip alone cannot force the panes side by side.
	stacked := height >= minStackedHeight-1 && (width < 120 || width < layout.tableWidth+minDetailsWidth)
	if stacked {
		layout.SetDirection(tview.FlexRow)
		layout.ResizeItem(layout.listPane, 0, 1)
		if !layout.stacked {
			// Stacking shrinks the tables vertically. Reapply the selection so
			// tview brings it into the new viewport on the next draw.
			for _, table := range layout.tables {
				if table == nil {
					continue
				}
				if row, column := table.GetSelection(); row > 0 && row < table.GetRowCount() {
					table.Select(row, column)
				}
			}
		}
	} else {
		// Start with an even split and give the tables room for their columns.
		tableWidth := max(width/2, min(layout.tableWidth, width-minDetailsWidth))
		layout.SetDirection(tview.FlexColumn)
		layout.ResizeItem(layout.listPane, tableWidth, 1)
	}
	layout.stacked = stacked
	layout.Flex.Draw(screen)
}

type View struct {
	app                 *App
	table               *tview.Table
	liveRows            []connectionRow
	changesOnly         bool
	vanishedTable       *tview.Table
	connectionLists     *tview.Flex
	listPages           *tview.Pages
	mainLayout          *connectionLayout
	processes           processView
	showProcesses       bool
	wasVanishedFocused  bool
	history             historyView
	showHistory         bool
	historyFromVanished bool
	baselineView        *tview.TextView
	vanishedCollapsed   bool
	vanishedVisible     bool
	detailsView         *tview.TextView
	filterInput         *tview.InputField
	hintView            *tview.TextView
	pages               *tview.Pages
	statusMessageID     uint64
}

func NewView(app *App) *View {
	v := &View{app: app}

	v.table = tview.NewTable().
		SetBorders(liveTableBorders).
		SetSelectable(true, false).
		SetFixed(1, 0)
	v.vanishedTable = tview.NewTable().SetBorders(vanishedTableBorders).SetSelectable(true, false).SetFixed(1, 0)
	v.processes = newProcessView()
	v.history.table = tview.NewTable().SetSelectable(true, false).SetFixed(1, 0)
	v.baselineView = tview.NewTextView()

	details := tview.NewTextView()
	details.SetDynamicColors(true)
	details.SetWordWrap(true)
	details.SetBorder(true)
	details.SetTitle(" Details ")
	v.detailsView = details

	v.filterInput = tview.NewInputField().
		SetLabel("Filter: ").
		SetLabelColor(tcell.ColorYellow).
		SetFieldBackgroundColor(tcell.ColorDarkSlateGray)

	hint := tview.NewTextView()
	hint.SetDynamicColors(true)
	hint.SetText(keyHints)
	v.hintView = hint

	v.pages = tview.NewPages()

	return v
}

func (v *View) Init() {
	v.connectionLists = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(v.table, 0, 2, true).
		AddItem(v.vanishedTable, 0, 0, false)
	v.listPages = tview.NewPages().
		AddPage("connections", v.connectionLists, true, true).
		AddPage("processes", v.processes.table, true, false).
		AddPage("history", v.history.table, true, false)
	mainFlex := tview.NewFlex().
		AddItem(v.listPages, 0, 1, true).
		AddItem(v.detailsView, 0, 1, false)
	v.mainLayout = &connectionLayout{
		Flex:     mainFlex,
		listPane: v.listPages,
		tables:   [2]*tview.Table{v.table, v.vanishedTable},
	}

	traffic := &trafficView{Box: tview.NewBox(), traffic: &v.app.traffic}
	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(v.mainLayout, 0, 1, true).
		AddItem(traffic, 2, 0, false).
		AddItem(v.baselineView, 1, 0, false).
		AddItem(v.filterInput, 1, 0, false).
		AddItem(v.hintView, 1, 0, false)

	v.pages.AddPage("main", &dashboardLayout{Flex: layout, traffic: traffic}, true, true)
	v.app.tviewApp.SetRoot(v.pages, true).EnableMouse(true)
	v.Refresh()
}

func (v *View) Refresh() {
	listeningOnly, problem := v.app.state.filterStatus()
	label := "Filter"
	if listeningOnly {
		label = "Listening only | " + label
	}
	v.filterInput.SetLabel(label + ": ")
	if v.showHistory {
		v.populateHistory(problem)
		v.updateDetailsView()
		return
	}
	live, vanished, pinTime := v.app.state.displayedRows()
	if v.showProcesses {
		v.populateProcesses(live, problem)
		v.updateDetailsView()
		return
	}
	pinned := !pinTime.IsZero()
	if pinned && v.changesOnly {
		var newRows []connectionRow
		for _, row := range live {
			if row.isNew {
				newRows = append(newRows, row)
			}
		}
		live = newRows
	}
	v.populateLiveTable(live, pinned)
	v.populateVanishedTable(vanished, pinTime, problem)
	v.mainLayout.tableWidth = max(
		connectionTableWidth(v.table, liveTableBorders),
		connectionTableWidth(v.vanishedTable, vanishedTableBorders),
	)
	v.updateDetailsView()
}

func (v *View) GetSelectedConnection() *models.Connection {
	if v.showHistory {
		return nil
	}
	if v.showProcesses {
		return v.processes.selectedRow().connection
	}
	if v.vanishedTable.HasFocus() {
		return nil
	}
	row, _ := v.table.GetSelection()
	if row < 1 || row > len(v.liveRows) {
		return nil
	}
	return &v.liveRows[row-1].connection
}

func (v *View) toggleProcessView() {
	v.showProcesses = !v.showProcesses
	if v.showProcesses {
		v.wasVanishedFocused = v.vanishedTable.HasFocus()
	}
	v.switchListPage(!v.showProcesses && v.wasVanishedFocused)
}

func (v *View) toggleHistoryView() {
	v.showHistory = !v.showHistory
	if v.showHistory {
		v.historyFromVanished = v.vanishedTable.HasFocus()
	} else {
		v.history.releaseRows()
		v.detailsView.ScrollToBeginning()
	}
	v.switchListPage(!v.showHistory && v.historyFromVanished)
}

func (v *View) activeList() (page string, tables [2]*tview.Table, hints string) {
	switch {
	case v.showHistory:
		return "history", [2]*tview.Table{v.history.table}, historyKeyHints
	case v.showProcesses:
		return "processes", [2]*tview.Table{v.processes.table}, processKeyHints
	default:
		hints := keyHints
		if _, pinned := v.app.state.pinnedComparison(); pinned {
			hints = "[yellow]C[white] Changes only " + hints
		}
		return "connections", [2]*tview.Table{v.table, v.vanishedTable}, hints
	}
}

func (v *View) refreshComparison() {
	_, _, hints := v.activeList()
	v.statusMessageID++
	v.hintView.SetText(hints)
	v.Refresh()
}

func (v *View) switchListPage(preferVanished bool) {
	page, tables, hints := v.activeList()
	v.mainLayout.tables = tables
	v.listPages.SwitchToPage(page)
	v.statusMessageID++
	v.hintView.SetText(hints)
	v.Refresh()
	focus := tables[0]
	if preferVanished && tables[1] == v.vanishedTable && v.vanishedVisible {
		focus = v.vanishedTable
	}
	v.app.tviewApp.SetFocus(focus)
}

func (v *View) focusActiveTable() {
	_, tables, _ := v.activeList()
	v.app.tviewApp.SetFocus(tables[0])
}

func (v *View) setListStatus(status, filterProblem string) {
	if filterProblem != "" {
		status = "Filter error: " + filterProblem + "  |  " + status
	}
	v.baselineView.SetText(status)
}

func (v *View) selectedVanishedConnection() *models.Connection {
	row, _ := v.vanishedTable.GetSelection()
	_, vanished, _ := v.app.state.displayedRows()
	if row < 1 || row > len(vanished) {
		return nil
	}
	return &vanished[row-1]
}

func (v *View) ShowInfoModal(message string, duration int) {
	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"OK"})

	go func() {
		<-time.After(time.Duration(duration) * time.Second)
		v.app.tviewApp.QueueUpdateDraw(func() {
			v.pages.RemovePage("info_modal")
		})
	}()

	modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
		v.pages.RemovePage("info_modal")
	})

	v.pages.AddPage("info_modal", modal, true, true)
}

func (v *View) SetStatusMessage(message string) {
	v.statusMessageID++
	id := v.statusMessageID
	v.hintView.SetText(fmt.Sprintf("[yellow]Status: [white]%s", message))

	time.AfterFunc(3*time.Second, func() {
		v.app.tviewApp.QueueUpdateDraw(func() {
			if id == v.statusMessageID {
				_, _, hints := v.activeList()
				v.hintView.SetText(hints)
			}
		})
	})
}

func (v *View) showInputModal(title, label string, callback func(string)) {
	inputField := tview.NewInputField().
		SetLabel(label).
		SetFieldWidth(40)

	form := tview.NewForm().
		AddFormItem(inputField).
		AddButton("Save", func() {
			callback(inputField.GetText())
			v.pages.RemovePage("input_modal")
			v.focusActiveTable()
		}).
		AddButton("Cancel", func() {
			v.pages.RemovePage("input_modal")
			v.focusActiveTable()
		})

	form.SetBorder(true).SetTitle(title)

	grid := tview.NewGrid().
		SetColumns(0, 50, 0).
		SetRows(0, 0, 0).
		AddItem(form, 1, 1, 1, 1, 0, 0, true)

	grid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			v.pages.RemovePage("input_modal")
			v.focusActiveTable()
			return nil
		}
		return event
	})

	v.pages.AddPage("input_modal", grid, true, true)

	v.app.tviewApp.SetFocus(form)
}
