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
		if event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab {
			if a.view.showHistory {
				if a.view.detailsView.HasFocus() {
					a.view.focusActiveTable()
				} else {
					a.tviewApp.SetFocus(a.view.detailsView)
				}
				return nil
			}
			if a.view.showProcesses {
				return nil
			}
			if a.view.vanishedVisible && !a.view.vanishedTable.HasFocus() {
				a.tviewApp.SetFocus(a.view.vanishedTable)
			} else {
				a.tviewApp.SetFocus(a.view.table)
			}
			return nil
		}
		if a.view.showHistory {
			switch event.Rune() {
			case 'P', 'B', 'b', 'v', 's', 'S', 'k', 'K', 'p', 'n', 't', 'e':
				a.view.SetStatusMessage("Press H to return to the previous view for this action.")
				return nil
			}
		}
		if a.view.showProcesses {
			switch event.Rune() {
			case 'B', 'b', 'v':
				a.view.SetStatusMessage("Press P for snapshot comparison controls.")
				return nil
			case 'k', 'K', 'p', 'n', 't':
				if row := a.view.processes.selectedRow(); row.group != nil && row.connection == nil {
					a.view.SetStatusMessage("Expand the group and select an endpoint.")
					return nil
				}
			}
		}

		switch event.Rune() {
		case 'H':
			a.view.toggleHistoryView()
			return nil
		case 'q':
			a.Stop()
			return nil
		case ' ':
			resume := a.paused
			a.setPaused(!resume)
			if resume {
				go a.loadData()
			}
			return nil
		case 's':
			firstColumn := 1
			if a.view.showProcesses {
				firstColumn = firstEndpointColumn
			}
			a.state.CycleSortColumn(firstColumn)
			a.view.Refresh()
			return nil
		case 'S':
			a.state.ToggleSortOrder()
			a.view.Refresh()
			return nil
		case 'r':
			if a.paused {
				a.view.SetStatusMessage("Press Space to resume before refreshing.")
			} else {
				go a.loadData()
			}
			return nil
		case 'v':
			a.view.vanishedCollapsed = !a.view.vanishedCollapsed
			a.view.Refresh()
			return nil
		case 'L':
			a.state.toggleListeningOnly()
			a.view.Refresh()
			return nil
		case 'P':
			a.view.toggleProcessView()
			return nil
		case 'B':
			if a.state.pinSnapshot() {
				a.view.Refresh()
			} else {
				a.view.SetStatusMessage("No snapshot available to pin.")
			}
			return nil
		case 'b':
			if a.state.togglePinned() {
				a.view.Refresh()
			} else {
				a.view.SetStatusMessage("Press B to pin the current snapshot first.")
			}
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

	for _, table := range []*tview.Table{a.view.table, a.view.vanishedTable, a.view.processes.table, a.view.history.table} {
		table.SetFocusFunc(a.view.updateDetailsView)
		table.SetSelectionChangedFunc(func(_, _ int) {
			a.view.updateDetailsView()
		})
	}
	for _, table := range []*tview.Table{a.view.table, a.view.vanishedTable} {
		table.SetSelectedFunc(func(_, _ int) {
			if conn, vanished := a.view.selectedConnectionDetails(); conn != nil {
				a.view.showDetailsModal(*conn, vanished)
			}
		})
	}
	a.view.processes.table.SetSelectedFunc(func(_, _ int) {
		a.view.activateProcessRow()
	})

	a.view.filterInput.SetChangedFunc(func(text string) {
		a.state.SetFilterText(text)
		a.view.Refresh()
	})

	a.view.filterInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter || key == tcell.KeyEscape {
			a.view.focusActiveTable()
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
				return
			}
			a.view.focusActiveTable()
		})

	a.view.pages.AddPage("export_modal", modal, true, true)
}
