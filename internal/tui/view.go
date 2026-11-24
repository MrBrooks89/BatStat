package tui

import (
	"fmt"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const keyHints = "[yellow]Space[white] Pause/Resume [yellow]Tab[white] Switch lists [yellow]v[white] Toggle vanished [yellow]B/b[white] Pin/Switch baseline [yellow]/[white] Filter [yellow]e[white] Export [yellow]h[white] Help [yellow]q[white] Quit"

type View struct {
	app               *App
	table             *tview.Table
	vanishedTable     *tview.Table
	connectionLists   *tview.Flex
	baselineView      *tview.TextView
	vanishedCollapsed bool
	vanishedVisible   bool
	detailsView       *tview.TextView
	filterInput       *tview.InputField
	hintView          *tview.TextView
	pages             *tview.Pages
	statusMessageID   uint64
}

func NewView(app *App) *View {
	v := &View{app: app}

	v.table = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false).
		SetFixed(1, 0)
	v.vanishedTable = tview.NewTable().SetSelectable(true, false).SetFixed(1, 0)
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
	mainFlex := tview.NewFlex().
		AddItem(v.connectionLists, 0, 1, true).
		AddItem(v.detailsView, 0, 1, false)

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(mainFlex, 0, 1, true).
		AddItem(v.baselineView, 1, 0, false).
		AddItem(v.filterInput, 1, 0, false).
		AddItem(v.hintView, 1, 0, false)

	v.pages.AddPage("main", layout, true, true)
	v.app.tviewApp.SetRoot(v.pages, true).EnableMouse(true)
	v.Refresh()
}

func (v *View) Refresh() {
	live, vanished, pinTime := v.app.state.displayedRows()
	v.populateLiveTable(live, !pinTime.IsZero())
	v.populateVanishedTable(vanished, pinTime)
	v.updateDetailsView()
}

func (v *View) GetSelectedConnection() *models.Connection {
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
				v.hintView.SetText(keyHints)
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
			v.app.tviewApp.SetFocus(v.table)
		}).
		AddButton("Cancel", func() {
			v.pages.RemovePage("input_modal")
			v.app.tviewApp.SetFocus(v.table)
		})

	form.SetBorder(true).SetTitle(title)

	grid := tview.NewGrid().
		SetColumns(0, 50, 0).
		SetRows(0, 0, 0).
		AddItem(form, 1, 1, 1, 1, 0, 0, true)

	grid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			v.pages.RemovePage("input_modal")
			v.app.tviewApp.SetFocus(v.table)
			return nil
		}
		return event
	})

	v.pages.AddPage("input_modal", grid, true, true)

	v.app.tviewApp.SetFocus(inputField)
}
