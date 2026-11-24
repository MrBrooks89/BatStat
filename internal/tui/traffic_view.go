package tui

import (
	"math"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type trafficView struct {
	*tview.Box
	traffic *interfaceTraffic
}

func (view *trafficView) Draw(screen tcell.Screen) {
	view.Box.DrawForSubclass(screen, view)
	x, y, width, height := view.GetInnerRect()
	if width == 0 || height == 0 {
		return
	}

	color := tview.Styles.PrimaryTextColor
	traffic := view.traffic
	switch {
	case traffic.selected == "":
		status := "Sampling…"
		if traffic.readFailed {
			status = "Unavailable"
		} else if len(traffic.samples) > 0 && len(traffic.interfaces) == 0 {
			status = "No interfaces"
		}
		tview.Print(screen, "Interface traffic  "+status, x, y, width, tview.AlignLeft, color)
		return
	}

	name := tview.Escape(traffic.selected)
	status := ""
	if !traffic.available(traffic.selected) {
		status = "Unavailable"
	}

	history := traffic.history()
	current := history[len(history)-1]
	receive := formatTrafficRate(current.received, current.valid)
	send := formatTrafficRate(current.sent, current.valid)
	if height == 1 {
		drawCompactTraffic(screen, name, status, receive, send, x, y, width)
		return
	}

	rx, tx := "RX "+receive, "TX "+send
	rateWidth := max(tview.TaggedStringWidth(rx), tview.TaggedStringWidth(tx))
	statusWidth := 0
	if status != "" {
		statusWidth = len(status) + 2
	}
	// Give narrow terminals room for the rates and the graph even when an
	// interface has a long name.
	labelWidth := drawTrafficName(screen, name, x, y, width, statusWidth+rateWidth+min(width/3, trafficSlots))
	tview.Print(screen, status, x+labelWidth, y, width-labelWidth, tview.AlignLeft, color)
	rateOffset := labelWidth + statusWidth
	tview.Print(screen, rx, x+rateOffset, y, width-rateOffset, tview.AlignLeft, color)
	tview.Print(screen, tx, x+rateOffset, y+1, width-rateOffset, tview.AlignLeft, color)
	graphOffset := rateOffset + rateWidth + 2
	if graphWidth := min(trafficSlots, width-graphOffset); graphWidth > 0 {
		receive, send := trafficGraphs(history[:], graphWidth)
		tview.Print(screen, receive, x+graphOffset, y, graphWidth, tview.AlignLeft, tcell.ColorTeal)
		tview.Print(screen, send, x+graphOffset, y+1, graphWidth, tview.AlignLeft, tcell.ColorGreen)
	}
}

func drawTrafficName(screen tcell.Screen, name string, x, y, width, reserve int) int {
	nameWidth := min(tview.TaggedStringWidth(name), max(0, width-reserve-2))
	if nameWidth == 0 {
		return 0
	}
	tview.Print(screen, name, x, y, nameWidth, tview.AlignLeft, tview.Styles.PrimaryTextColor)
	return nameWidth + 2
}

func drawCompactTraffic(screen tcell.Screen, name, status, receive, send string, x, y, width int) {
	color := tview.Styles.PrimaryTextColor
	if status != "" {
		offset := drawTrafficName(screen, name, x, y, width, len(status))
		tview.Print(screen, status, x+offset, y, width-offset, tview.AlignLeft, color)
		return
	}
	rx, tx := "RX "+receive, "TX "+send
	spacing := 2
	if tview.TaggedStringWidth(name+"  "+rx+"  "+tx) > width {
		rx, tx = "RX"+strings.ReplaceAll(receive, " ", ""), "TX"+strings.ReplaceAll(send, " ", "")
		spacing = 1
	}
	ratesWidth := tview.TaggedStringWidth(rx+tx) + spacing
	// Reserve space for both direction labels when the rates need clipping.
	offset := drawTrafficName(screen, name, x, y, width, min(ratesWidth, max(5, width-6)))
	remaining := width - offset - spacing
	rxWidth := min(tview.TaggedStringWidth(rx), max(remaining/2, remaining-tview.TaggedStringWidth(tx)))
	tview.Print(screen, rx, x+offset, y, rxWidth, tview.AlignLeft, color)
	tview.Print(screen, tx, x+offset+rxWidth+spacing, y, remaining-rxWidth, tview.AlignLeft, color)
}

func formatTrafficRate(bitsPerSecond float64, valid bool) string {
	if !valid {
		return "—"
	}
	units := [...]string{"b/s", "kb/s", "Mb/s", "Gb/s", "Tb/s", "Pb/s", "Eb/s"}
	unit := 0
	for bitsPerSecond >= 999.95 && unit < len(units)-1 {
		bitsPerSecond /= 1000
		unit++
	}
	rate := strings.TrimSuffix(strconv.FormatFloat(bitsPerSecond, 'f', 1, 64), ".0")
	return rate + " " + units[unit]
}

func trafficGraphs(history []trafficRate, width int) (receive, send string) {
	var peak float64
	for _, rate := range history {
		peak = max(peak, rate.received, rate.sent)
	}
	rx, tx := make([]rune, width), make([]rune, width)
	for column := range width {
		// Keep the latest reading separate. In a narrow graph, earlier
		// columns group adjacent times and retain gaps.
		bucket := history[len(history)-1]
		if column < width-1 {
			start := column * (len(history) - 1) / (width - 1)
			end := (column + 1) * (len(history) - 1) / (width - 1)
			bucket = trafficRate{valid: true}
			for _, rate := range history[start:end] {
				if !rate.valid {
					bucket.valid = false
					break
				}
				bucket.received = max(bucket.received, rate.received)
				bucket.sent = max(bucket.sent, rate.sent)
			}
		}
		rx[column] = trafficBar(bucket.received, peak, bucket.valid)
		tx[column] = trafficBar(bucket.sent, peak, bucket.valid)
	}
	return string(rx), string(tx)
}

func trafficBar(rate, peak float64, valid bool) rune {
	switch {
	case !valid:
		return ' '
	case rate == 0:
		return '▁'
	default:
		return '▁' + rune(math.Ceil(rate/peak*7))
	}
}

func (v *View) showInterfacePicker() {
	traffic := &v.app.traffic
	names := traffic.interfaceNames()
	if len(names) == 0 {
		v.SetStatusMessage("No interfaces available.")
		return
	}

	const page = "interfaces"
	previousFocus := v.app.tviewApp.GetFocus()
	closePicker := func() {
		v.pages.RemovePage(page)
		if previousFocus != nil {
			v.app.tviewApp.SetFocus(previousFocus)
		}
	}

	list := tview.NewList().ShowSecondaryText(false)
	list.SetBorder(true).SetTitle(" Interfaces · Enter / Esc ")
	for index, name := range names {
		label := tview.Escape(name)
		if !traffic.available(name) {
			label += "  (unavailable)"
		}
		list.AddItem(label, "", 0, nil)
		if name == traffic.selected {
			list.SetCurrentItem(index)
		}
	}
	list.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		traffic.selected = names[index]
		closePicker()
	})
	list.SetDoneFunc(closePicker)

	grid := tview.NewGrid().
		SetColumns(0, -3, 0).
		SetRows(0, -3, 0).
		AddItem(list, 1, 1, 1, 1, 0, 0, true)
	v.pages.AddPage(page, grid, true, true)
	v.app.tviewApp.SetFocus(list)
}
