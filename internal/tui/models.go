package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/MrBrooks89/BatStat/internal/actions"
	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (v *View) showHelpModal() {
	var builder strings.Builder
	builder.WriteString("[::b][yellow]BatStat Keybindings[-:-:-]\n\n")
	builder.WriteString("[::u]Navigation[-:-]\n")
	builder.WriteString("[green]↑/↓      [white]Move selection up/down\n")
	builder.WriteString("[green]←/→      [white]Scroll table left/right\n")
	builder.WriteString("[green]Tab      [white]Switch between live and vanished connections\n")
	builder.WriteString("[green]Enter    [white]Show detailed info for selection\n\n")
	builder.WriteString("[::u]Actions[-:-]\n")
	builder.WriteString("[gray]Process termination requires a live connection snapshot.\n")
	builder.WriteString("[green]k        [white]Kill selected process (Graceful)\n")
	builder.WriteString("[green]K        [white]Force Kill selected process (SIGKILL)\n")
	builder.WriteString("[green]p        [white]Ping remote address of selection\n")
	builder.WriteString("[green]n        [white]Nslookup remote address of selection\n")
	builder.WriteString("[green]t        [white]Traceroute to remote address of selection\n	")
	builder.WriteString("[green]/        [white]Filter, e.g. pid:1234 proto:tcp state:listen\n")
	builder.WriteString("[green]L        [white]Toggle Listening only\n")
	builder.WriteString("[green]e        [white]Export filtered live connections to CSV (with path selection)\n\n")
	builder.WriteString("[::u]Sorting[-:-]\n")
	builder.WriteString("[green]s        [white]Cycle through sortable columns\n")
	builder.WriteString("[green]S        [white]Toggle sort order (ASC/DESC)\n\n")
	builder.WriteString("[::u]Application[-:-]\n")
	builder.WriteString("[green]h        [white]Show/Hide this help panel\n")
	builder.WriteString("[green]Space    [white]Pause or resume connection monitoring\n")
	builder.WriteString("[green]v        [white]Collapse or expand vanished connections\n")
	builder.WriteString("[green]B        [white]Pin the current connection snapshot\n")
	builder.WriteString("[green]b        [white]Switch between previous-refresh and pinned comparisons\n")
	builder.WriteString("[green]r        [white]Refresh connections manually while live\n")
	builder.WriteString("[green]q        [white]Quit BatStat\n")

	textView := tview.NewTextView().SetDynamicColors(true).SetText(builder.String())
	textView.SetBorder(true).SetBorderPadding(1, 1, 1, 1)

	frame := tview.NewFrame(textView).
		AddText("Help", true, tview.AlignCenter, tview.Styles.TitleColor).
		AddText("Press 'h' or Esc to close", false, tview.AlignCenter, tview.Styles.SecondaryTextColor)

	frame.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Rune() == 'h' {
			v.pages.RemovePage("help_modal")
			v.app.tviewApp.SetFocus(v.table)
			return nil
		}
		return event
	})

	v.pages.AddPage("help_modal", frame, true, true)
}

func (v *View) showPingModal() {
	c := v.GetSelectedConnection()
	if c == nil || c.Raddr == "" || strings.HasPrefix(c.Raddr, ":") {
		return
	}
	ip := strings.Split(c.Raddr, ":")[0]

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() { v.app.tviewApp.Draw() })

	frame := tview.NewFrame(textView).
		AddText(fmt.Sprintf("Pinging %s...", ip), true, tview.AlignCenter, tview.Styles.TitleColor).
		AddText("Press Esc to close", false, tview.AlignCenter, tview.Styles.SecondaryTextColor)

	ctx, cancel := context.WithCancel(context.Background())

	frame.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			cancel()
			v.pages.RemovePage("ping_modal")
			v.app.tviewApp.SetFocus(v.table)
			return nil
		}
		return event
	})

	outputChan := make(chan string)
	go actions.Ping(ctx, ip, outputChan)

	go func() {
		for line := range outputChan {
			v.app.tviewApp.QueueUpdateDraw(func() {
				currentText := textView.GetText(false)
				textView.SetText(currentText + line + "\n")
				textView.ScrollToEnd()
			})
		}
	}()

	v.pages.AddPage("ping_modal", frame, true, true)
}

func (v *View) showKillConfirmationModal(force bool) {
	if v.app.awaitingLiveData {
		v.SetStatusMessage("Resume and wait for a refresh before killing a process.")
		return
	}
	c := v.GetSelectedConnection()
	if c == nil || c.Pid == 0 {
		return
	}

	actionText := "kill"
	actionFunc := func() error { return actions.KillProcess(c.Pid) }
	if force {
		actionText = "forcefully kill"
		actionFunc = func() error { return actions.ForceKillProcess(c.Pid) }
	}

	modal := tview.NewModal().
		SetText(fmt.Sprintf("Are you sure you want to %s process '%s' (PID: %d)?", actionText, c.ProcessName, c.Pid)).
		AddButtons([]string{"Confirm", "Cancel"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			if buttonLabel == "Confirm" {
				if err := actionFunc(); err == nil {
					go v.app.loadData()
				}
			}
			v.pages.RemovePage("kill_confirm").ShowPage("main")
			v.app.tviewApp.SetFocus(v.table)
		})
	v.pages.AddPage("kill_confirm", modal, true, true)
}

func (v *View) showDetailsModal(c models.Connection, vanished bool) {
	textView := tview.NewTextView().SetDynamicColors(true).SetText(v.connectionDetails(c, vanished))
	textView.SetBorder(true).SetBorderPadding(1, 1, 1, 1)

	frame := tview.NewFrame(textView).
		AddText("Connection Details", true, tview.AlignCenter, tview.Styles.TitleColor).
		AddText("Press Enter or Esc to close", false, tview.AlignCenter, tview.Styles.SecondaryTextColor)

	frame.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter || event.Key() == tcell.KeyEscape {
			v.pages.RemovePage("details_modal")
			if vanished && v.vanishedVisible {
				v.app.tviewApp.SetFocus(v.vanishedTable)
			} else {
				v.app.tviewApp.SetFocus(v.table)
			}
			return nil
		}
		return event
	})

	v.pages.AddPage("details_modal", frame, true, true)
}

func (v *View) showNslookupModal() {
	c := v.GetSelectedConnection()
	if c == nil || c.Raddr == "" || strings.HasPrefix(c.Raddr, ":") {
		return
	}
	host := strings.Split(c.Raddr, ":")[0]

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() { v.app.tviewApp.Draw() })

	frame := tview.NewFrame(textView).
		AddText(fmt.Sprintf("Nslookup for %s...", host), true, tview.AlignCenter, tview.Styles.TitleColor).
		AddText("Press Esc to close", false, tview.AlignCenter, tview.Styles.SecondaryTextColor)

	ctx, cancel := context.WithCancel(context.Background())

	frame.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			cancel()
			v.pages.RemovePage("nslookup_modal")
			v.app.tviewApp.SetFocus(v.table)
			return nil
		}
		return event
	})

	outputChan := make(chan string)
	go actions.Nslookup(ctx, host, outputChan)

	go func() {
		for line := range outputChan {
			v.app.tviewApp.QueueUpdateDraw(func() {
				currentText := textView.GetText(false)
				textView.SetText(currentText + line + "\n")
				textView.ScrollToEnd()
			})
		}
	}()

	v.pages.AddPage("nslookup_modal", frame, true, true)
}

func (v *View) showTracerouteModal() {
	c := v.GetSelectedConnection()
	if c == nil || c.Raddr == "" || strings.HasPrefix(c.Raddr, ":") {
		return
	}
	host := strings.Split(c.Raddr, ":")[0]

	textView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() { v.app.tviewApp.Draw() })

	frame := tview.NewFrame(textView).
		AddText(fmt.Sprintf("Traceroute to %s...", host), true, tview.AlignCenter, tview.Styles.TitleColor).
		AddText("Press Esc to close", false, tview.AlignCenter, tview.Styles.SecondaryTextColor)

	ctx, cancel := context.WithCancel(context.Background())

	frame.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			cancel()
			v.pages.RemovePage("traceroute_modal")
			v.app.tviewApp.SetFocus(v.table)
			return nil
		}
		return event
	})

	outputChan := make(chan string)
	go actions.Traceroute(ctx, host, outputChan)

	go func() {
		for line := range outputChan {
			v.app.tviewApp.QueueUpdateDraw(func() {
				currentText := textView.GetText(false)
				textView.SetText(currentText + line + "\n")
				textView.ScrollToEnd()
			})
		}
	}()

	v.pages.AddPage("traceroute_modal", frame, true, true)
}
