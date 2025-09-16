package tui

import (
	"fmt"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/MrBrooks89/BatStat/internal/models"
)

type View struct {
	app         *App
	table       *tview.Table
	detailsView *tview.TextView
	filterInput *tview.InputField
	hintView    *tview.TextView
	pages       *tview.Pages
}

func NewView(app *App) *View {
	v := &View{app: app}

	v.table = tview.NewTable().
		SetBorders(true).
		SetSelectable(true, false).
		SetFixed(1, 0)

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
	hint.SetText("[::b]Keys:[-:-] [yellow]/[white]Filter [yellow]s/S[white]Sort [yellow]k/K[white]Kill [yellow]p[white]Ping [yellow]t[white]Traceroute [yellow]n[white]Nslookup [yellow]e[white]Export [yellow]h[white]Help [yellow]q[white]Quit")
	v.hintView = hint

	v.pages = tview.NewPages()

	return v
}

func (v *View) Init() {
	mainFlex := tview.NewFlex().
		AddItem(v.table, 0, 1, true).
		AddItem(v.detailsView, 0, 1, false)

	layout := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(mainFlex, 0, 1, true).
		AddItem(v.filterInput, 1, 0, false).
		AddItem(v.hintView, 1, 0, false)

	v.pages.AddPage("main", layout, true, true)
	v.app.tviewApp.SetRoot(v.pages, true).EnableMouse(true)
}

func (v *View) Refresh() {
	v.populateTable()
	v.updateHeaderIndicator()
	selectedRow, _ := v.table.GetSelection()
	v.updateDetailsView(selectedRow)
}

func (v *View) GetSelectedConnection() *models.Connection {
	row, _ := v.table.GetSelection()
	if row < 1 {
		return nil
	}
	conns := v.app.state.GetFilteredConnections()
	if row > len(conns) {
		return nil
	}
	return &conns[row-1]
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
	originalText := v.hintView.GetText(false)
	v.hintView.SetText(fmt.Sprintf("[yellow]Status: [white]%s", message))

	go func() {
		<-time.After(3 * time.Second) 
		v.app.tviewApp.QueueUpdateDraw(func() {
			v.hintView.SetText(originalText)
		})
	}()
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
