package tui

import (
	"strings"

	"github.com/MrBrooks89/BatStat/internal/actions"
	"github.com/gdamore/tcell/v2"
)

func (a *App) setKeybindings() {
	a.tviewApp.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
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

	filename, err := actions.ExportToCSV(conns)
	if err != nil {
		a.view.SetStatusMessage("Error exporting to CSV: " + err.Error())
	} else {
		a.view.SetStatusMessage("Exported " + strings.TrimSpace(filename) + "")
	}
}
