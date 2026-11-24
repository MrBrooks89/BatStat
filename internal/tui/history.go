package tui

import (
	"slices"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
)

const (
	maxActiveHistory   = 2048
	maxVanishedHistory = 2048
	maxHistoryChanges  = 16
)

type observedStateChange struct {
	at       time.Time
	from, to string
}

type historyRecord struct {
	id                uint64
	latest            models.Connection
	firstSeen         time.Time
	lastSeen          time.Time
	firstMissing      time.Time
	changes           []observedStateChange
	trimmedChanges    bool
	identityUncertain bool
}

func (record *historyRecord) observe(c models.Connection, at time.Time, uncertain bool) {
	if !uncertain && record.latest.Status != c.Status {
		// Displayed history rows share the existing changes, so copy before writing.
		changes := record.changes
		if len(changes) == maxHistoryChanges {
			changes = changes[1:]
			record.trimmedChanges = true
		}
		record.changes = append(slices.Clone(changes), observedStateChange{at, record.latest.Status, c.Status})
	}
	record.latest = c
	record.lastSeen = at
	record.identityUncertain = record.identityUncertain || uncertain
}

type activeHistoryRecord struct {
	historyRecord
	snapshotIndex int
	knownPID      int32
}

type connectionHistory struct {
	active       []activeHistoryRecord
	vanished     []historyRecord
	nextVanished int
	nextID       uint64
	untracked    int
}

func (h *connectionHistory) observe(before, after []models.Connection, at time.Time) {
	// Include untracked sockets when matching so the active limit can't hide
	// alternative matches. Retain a known owner across missing PID observations.
	identities := before
	copied := false
	for _, record := range h.active {
		c := record.latest
		if c.Pid > 0 || record.knownPID <= 0 {
			continue
		}
		if !copied {
			identities = slices.Clone(identities)
			copied = true
		}
		c.Pid, c.Fd, c.FDKnown = record.knownPID, 0, false
		identities[record.snapshotIndex] = c
	}
	oldMatches, newMatches := matchHistorySnapshots(identities, after)
	tracked := make([]bool, len(after))
	active := h.active[:0]
	for _, record := range h.active {
		match := oldMatches[record.snapshotIndex]
		if match.pairedIndexPlusOne == 0 {
			record.firstMissing = at
			record.identityUncertain = record.identityUncertain || match.uncertain
			h.archive(record.historyRecord)
			continue
		}
		index := match.pairedIndexPlusOne - 1
		c := after[index]
		uncertain := match.uncertain || newMatches[index].uncertain
		record.observe(c, at, uncertain)
		record.snapshotIndex = index
		if c.Pid > 0 || uncertain {
			// An ambiguous match cannot carry the previous record's owner forward.
			record.knownPID = c.Pid
		}
		tracked[index] = true
		active = append(active, record)
	}
	clear(h.active[len(active):])
	for i, c := range after {
		if len(active) == maxActiveHistory {
			break
		}
		if tracked[i] {
			continue
		}
		h.nextID++
		active = append(active, activeHistoryRecord{
			historyRecord: historyRecord{
				id:                h.nextID,
				latest:            c,
				firstSeen:         at,
				lastSeen:          at,
				identityUncertain: newMatches[i].uncertain,
			},
			snapshotIndex: i,
			knownPID:      c.Pid,
		})
	}
	h.active = active
	h.untracked = len(after) - len(active)
}

func (h *connectionHistory) archive(record historyRecord) {
	if len(h.vanished) < maxVanishedHistory {
		h.vanished = append(h.vanished, record)
	} else {
		h.vanished[h.nextVanished] = record
	}
	h.nextVanished = (h.nextVanished + 1) % maxVanishedHistory
}

func (s *AppState) displayedHistory() (rows []historyRecord, untracked int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	h := &s.history
	rows = make([]historyRecord, 0, len(h.vanished)+len(h.active))
	for offset := range h.vanished {
		index := (h.nextVanished - 1 - offset + len(h.vanished)) % len(h.vanished)
		record := h.vanished[index]
		if s.matchesFilter(record.latest) {
			rows = append(rows, record)
		}
	}
	for _, record := range slices.Backward(h.active) {
		if s.matchesFilter(record.latest) {
			rows = append(rows, record.historyRecord)
		}
	}
	return rows, h.untracked
}
