package tui

import (

	"github.com/MrBrooks89/BatStat/internal/actions"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (v *View) IsModalActive() bool {
	currentPage, _ := v.pages.GetFrontPage()
	return currentPage != "main"
}

func (a *App) setKeybindings() {
	a.tviewApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if a.view.IsModalActive() {
			return event
		}
		if a.view.filterInput.HasFocus() {
			return event
		}

		switch event.Rune() {
		case 'q':
			a.Stop()
			return nil
		case 's':
			a.state.CycleSortColumn()
			a.view.Refresh()
			return nil
		case 'S':
			a.state.ToggleSortOrder()
			a.view.Refresh()
			return nil
		case 'r':
			go a.loadData() 
			return nil
		case 'k':
			a.view.showKillConfirmationModal(false) 
			return nil
		case 'K':
			a.view.showKillConfirmationModal(true) 
			return nil
		case 'p':
			a.view.showPingModal()
			return nil
		case 'n':
			a.view.showNslookupModal()
			return nil
		case 't':
			a.view.showTracerouteModal()
			return nil
		case 'h':
			a.view.showHelpModal()
			return nil
		case 'e':
			a.handleExport()
			return nil
		case '/':
			a.tviewApp.SetFocus(a.view.filterInput)
			return nil
		}
		return event
	})

	a.view.table.SetSelectionChangedFunc(func(row, column int) {
		a.view.updateDetailsView(row)
	})

	a.view.table.SetSelectedFunc(func(row, column int) {
		if conn := a.view.GetSelectedConnection(); conn != nil {
			a.view.showDetailsModal(*conn)
		}
	})

	a.view.filterInput.SetChangedFunc(func(text string) {
		a.state.SetFilterText(text)
		a.view.Refresh()
	})

	a.view.filterInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter || key == tcell.KeyEscape {
			a.tviewApp.SetFocus(a.view.table)
		}
	})
}
func (a *App) handleExport() {
	conns := a.state.GetFilteredConnections()
	if len(conns) == 0 {
		a.view.SetStatusMessage("No connections to export.")
		return
	}

	modal := tview.NewModal().
		SetText("Export to CSV").
		AddButtons([]string{"Default", "Custom", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			a.view.pages.RemovePage("export_modal")
			switch buttonLabel {
			case "Default":
				filename, err := actions.ExportToCSV(conns, "batstat_export.csv")
				if err != nil {
					a.view.SetStatusMessage("Error exporting to CSV: " + err.Error())
				} else {
					a.view.SetStatusMessage("Exported to " + filename)
				}
			case "Custom":
				a.view.showInputModal("Export to CSV", "File path: ", func(path string) {
					if path == "" {
						a.view.SetStatusMessage("Export canceled.")
						return
					}
					filename, err := actions.ExportToCSV(conns, path)
					if err != nil {
						a.view.SetStatusMessage("Error exporting to CSV: " + err.Error())
					} else {
						a.view.SetStatusMessage("Exported to " + filename)
					}
				})
			}
			a.tviewApp.SetFocus(a.view.table)
		})

	a.view.pages.AddPage("export_modal", modal, true, true)
}
