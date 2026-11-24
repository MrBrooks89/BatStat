package tui

import (
	"context"
	"maps"
	"net"
	"slices"
	"time"

	"github.com/gdamore/tcell/v2"
	gopsnet "github.com/shirou/gopsutil/v3/net"
)

const (
	trafficWindow = time.Minute
	trafficSlots  = int(trafficWindow / refreshInterval)
	// tview redraws after an input capture consumes a key event. This key is
	// reserved for the sampler and is outside the terminal key range.
	trafficUpdateKey = tcell.Key(-1)
)

type interfaceCounters struct {
	received, sent uint64
	index          int
	flags          net.Flags
}

func (counters interfaceCounters) available() bool {
	const active = net.FlagUp | net.FlagRunning
	return counters.index == 0 || counters.flags&active == active
}

type interfaceReading struct {
	at       time.Time
	counters map[string]interfaceCounters
}

type trafficRate struct {
	received, sent float64
	valid          bool
}

type trafficSample struct {
	at    time.Time
	rates map[string]trafficRate
}

type interfaceTraffic struct {
	selected   string
	interfaces map[string]interfaceCounters
	samples    []trafficSample
	readFailed bool
}

// sampleInterfaceTraffic runs independently of connection refreshes so paused
// monitoring and slow connection lookups leave interface traffic live.
func sampleInterfaceTraffic(ctx context.Context, screen tcell.Screen, updates chan<- interfaceReading) {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		readContext, cancel := context.WithTimeout(ctx, refreshInterval)
		counters, err := gopsnet.IOCountersWithContext(readContext, true)
		at := time.Now()
		cancel()

		var reading map[string]interfaceCounters
		if err == nil {
			reading = make(map[string]interfaceCounters, len(counters))
			for _, counter := range counters {
				if counter.Name != "" {
					reading[counter.Name] = interfaceCounters{received: counter.BytesRecv, sent: counter.BytesSent}
				}
			}
			// Metadata is optional. The byte counters are still useful if
			// the OS cannot enumerate interface flags or indices.
			if interfaces, err := net.Interfaces(); err == nil {
				for _, iface := range interfaces {
					if counter, ok := reading[iface.Name]; ok {
						counter.index, counter.flags = iface.Index, iface.Flags
						reading[iface.Name] = counter
					}
				}
			}
		}

		select {
		case updates <- interfaceReading{at: at, counters: reading}:
			// If the event queue is full, the next draw will still drain
			// the pending sample. Posting never waits for the UI to exit.
			_ = screen.PostEvent(tcell.NewEventKey(trafficUpdateKey, 0, tcell.ModNone))
		case <-ctx.Done():
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// record accepts a nil reading for an unsuccessful sample. A successful empty
// reading means the interfaces are gone.
func (traffic *interfaceTraffic) record(at time.Time, reading map[string]interfaceCounters) {
	sample := trafficSample{at: at}
	if reading != nil {
		if !traffic.readFailed && len(traffic.samples) > 0 {
			elapsed := at.Sub(traffic.samples[len(traffic.samples)-1].at).Seconds()
			if elapsed > 0 {
				sample.rates = make(map[string]trafficRate, len(reading))
				for name, current := range reading {
					previous, ok := traffic.interfaces[name]
					if !ok || !current.available() || !previous.available() ||
						current.received < previous.received || current.sent < previous.sent ||
						(current.index != 0 && previous.index != 0 && current.index != previous.index) {
						continue
					}
					sample.rates[name] = trafficRate{
						received: float64(current.received-previous.received) * 8 / elapsed,
						sent:     float64(current.sent-previous.sent) * 8 / elapsed,
						valid:    true,
					}
				}
			}
		}
		traffic.interfaces = reading
	}
	traffic.readFailed = reading == nil
	cutoff := at.Add(-trafficWindow)
	traffic.samples = slices.DeleteFunc(traffic.samples, func(old trafficSample) bool {
		return !old.at.After(cutoff)
	})
	traffic.samples = append(traffic.samples, sample)
	if traffic.selected == "" && len(sample.rates) > 0 {
		traffic.selected = traffic.initialInterface(sample.rates)
	}
}

func (traffic *interfaceTraffic) interfaceNames() []string {
	names := slices.Sorted(maps.Keys(traffic.interfaces))
	if index, found := slices.BinarySearch(names, traffic.selected); !found && traffic.selected != "" {
		names = slices.Insert(names, index, traffic.selected)
	}
	return names
}

func (traffic *interfaceTraffic) available(name string) bool {
	counters, ok := traffic.interfaces[name]
	return ok && counters.available()
}

func (traffic *interfaceTraffic) initialInterface(rates map[string]trafficRate) string {
	names := traffic.interfaceNames()
	var busiest, firstUp, loopback string
	var peak float64
	for _, name := range names {
		flags := traffic.interfaces[name].flags
		if flags&net.FlagLoopback != 0 {
			if loopback == "" {
				loopback = name
			}
			continue
		}
		if firstUp == "" && flags&net.FlagUp != 0 {
			firstUp = name
		}
		rate := rates[name]
		if total := rate.received + rate.sent; total > peak {
			busiest, peak = name, total
		}
	}
	for _, name := range []string{busiest, firstUp, loopback} {
		if name != "" {
			return name
		}
	}
	return names[0]
}

func (traffic *interfaceTraffic) history() [trafficSlots]trafficRate {
	var history [trafficSlots]trafficRate
	if len(traffic.samples) == 0 {
		return history
	}
	latest := traffic.samples[len(traffic.samples)-1].at
	for _, sample := range traffic.samples {
		slotsAgo := int(latest.Sub(sample.at).Round(refreshInterval) / refreshInterval)
		if slotsAgo < len(history) {
			history[len(history)-1-slotsAgo] = sample.rates[traffic.selected]
		}
	}
	return history
}
