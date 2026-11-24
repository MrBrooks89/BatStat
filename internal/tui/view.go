package tui

import (
	"fmt"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	keyHints        = "[yellow]P[white] Process View [yellow]Space[white] Pause/Resume [yellow]/[white] Filter [yellow]L[white] Listening only [yellow]Tab[white] Switch lists [yellow]v[white] Toggle vanished [yellow]B/b[white] Pin/Switch baseline [yellow]e[white] Export [yellow]h[white] Help [yellow]q[white] Quit"
	processKeyHints = "[yellow]P[white] Connections [yellow]Enter[white] Expand/Details [yellow]/[white] Filter [yellow]L[white] Listening only [yellow]Space[white] Pause/Resume [yellow]e[white] Export [yellow]h[white] Help [yellow]q[white] Quit"
)

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
	stacked := height >= 15 && (width < 120 || width < layout.tableWidth+minDetailsWidth)
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
	app                *App
	table              *tview.Table
	vanishedTable      *tview.Table
	connectionLists    *tview.Flex
	listPages          *tview.Pages
	mainLayout         *connectionLayout
	processes          processView
	showProcesses      bool
	wasVanishedFocused bool
	baselineView       *tview.TextView
	vanishedCollapsed  bool
	vanishedVisible    bool
	detailsView        *tview.TextView
	filterInput        *tview.InputField
	hintView           *tview.TextView
	pages              *tview.Pages
	statusMessageID    uint64
}

func NewView(app *App) *View {
	v := &View{app: app}

	v.table = tview.NewTable().
		SetBorders(liveTableBorders).
		SetSelectable(true, false).
		SetFixed(1, 0)
	v.vanishedTable = tview.NewTable().SetBorders(vanishedTableBorders).SetSelectable(true, false).SetFixed(1, 0)
	v.processes = newProcessView()
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
		AddPage("processes", v.processes.table, true, false)
	mainFlex := tview.NewFlex().
		AddItem(v.listPages, 0, 1, true).
		AddItem(v.detailsView, 0, 1, false)
	v.mainLayout = &connectionLayout{
		Flex:     mainFlex,
		listPane: v.listPages,
		tables:   [2]*tview.Table{v.table, v.vanishedTable},
	}

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(v.mainLayout, 0, 1, true).
		AddItem(v.baselineView, 1, 0, false).
		AddItem(v.filterInput, 1, 0, false).
		AddItem(v.hintView, 1, 0, false)

	v.pages.AddPage("main", layout, true, true)
	v.app.tviewApp.SetRoot(v.pages, true).EnableMouse(true)
	v.Refresh()
}

func (v *View) Refresh() {
	live, vanished, pinTime := v.app.state.displayedRows()
	listeningOnly, problem := v.app.state.filterStatus()
	label := "Filter"
	if listeningOnly {
		label = "Listening only | " + label
	}
	v.filterInput.SetLabel(label + ": ")
	if v.showProcesses {
		v.populateProcesses(live, problem)
		v.updateDetailsView()
		return
	}
	v.populateLiveTable(live, !pinTime.IsZero())
	v.populateVanishedTable(vanished, pinTime, problem)
	v.mainLayout.tableWidth = max(
		connectionTableWidth(v.table, liveTableBorders),
		connectionTableWidth(v.vanishedTable, vanishedTableBorders),
	)
	v.updateDetailsView()
}

func (v *View) GetSelectedConnection() *models.Connection {
	if v.showProcesses {
		return v.processes.selectedRow().connection
	}
	if v.vanishedTable.HasFocus() {
		return nil
	}
	row, _ := v.table.GetSelection()
	live, _, _ := v.app.state.displayedRows()
	if row < 1 || row > len(live) {
		return nil
	}
	return &live[row-1].connection
}

func (v *View) toggleProcessView() {
	v.showProcesses = !v.showProcesses
	if v.showProcesses {
		v.wasVanishedFocused = v.vanishedTable.HasFocus()
		v.mainLayout.tables = [2]*tview.Table{v.processes.table}
		v.listPages.SwitchToPage("processes")
	} else {
		v.mainLayout.tables = [2]*tview.Table{v.table, v.vanishedTable}
		v.listPages.SwitchToPage("connections")
	}
	v.statusMessageID++
	v.hintView.SetText(v.currentKeyHints())
	v.Refresh()
	if !v.showProcesses && v.wasVanishedFocused && v.vanishedVisible {
		v.app.tviewApp.SetFocus(v.vanishedTable)
	} else {
		v.focusActiveTable()
	}
}

func (v *View) focusActiveTable() {
	if v.showProcesses {
		v.app.tviewApp.SetFocus(v.processes.table)
	} else {
		v.app.tviewApp.SetFocus(v.table)
	}
}

func (v *View) currentKeyHints() string {
	if v.showProcesses {
		return processKeyHints
	}
	return keyHints
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
				v.hintView.SetText(v.currentKeyHints())
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
