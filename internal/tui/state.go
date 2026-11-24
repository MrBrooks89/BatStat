package tui

import (
	"cmp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
)

type connectionRow struct {
	connection models.Connection
	isNew      bool
}

type connectionSnapshot struct {
	connections []models.Connection
	capturedAt  time.Time
}

type AppState struct {
	mu               sync.RWMutex
	current          *connectionSnapshot
	previousMatches  []bool
	recentlyVanished []recentlyVanished
	pinned           *connectionSnapshot
	showPinned       bool

	filteredLive     []connectionRow
	filteredVanished []models.Connection
	filterText       string
	sortColumn       int
	sortAsc          bool
}

func NewAppState() *AppState {
	return &AppState{sortAsc: true}
}

func (s *AppState) SetConnections(conns []models.Connection, capturedAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != nil {
		before := s.current.connections
		beforeMatched, afterMatched := matchSnapshots(before, conns)
		s.previousMatches = afterMatched
		vanished := unmatchedConnections(before, beforeMatched)
		added := unmatchedConnections(conns, afterMatched)
		s.recentlyVanished = updateRecentlyVanished(s.recentlyVanished, vanished, added)
	}
	s.current = &connectionSnapshot{connections: slices.Clone(conns), capturedAt: capturedAt}
	s.updateDisplay()
}

func (s *AppState) pinSnapshot() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == nil {
		return false
	}
	s.pinned = s.current
	s.showPinned = true
	s.updateDisplay()
	return true
}

func (s *AppState) togglePinned() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pinned == nil {
		return false
	}
	s.showPinned = !s.showPinned
	s.updateDisplay()
	return true
}

func (s *AppState) displayedRows() (live []connectionRow, vanished []models.Connection, pinTime time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.showPinned {
		pinTime = s.pinned.capturedAt
	}
	return s.filteredLive, s.filteredVanished, pinTime
}

// GetFilteredConnections returns the filtered live rows in display order.
func (s *AppState) GetFilteredConnections() []models.Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()
	conns := make([]models.Connection, len(s.filteredLive))
	for i, row := range s.filteredLive {
		conns[i] = row.connection
	}
	return conns
}

func (s *AppState) SetSort(column int, asc bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sortColumn = column
	s.sortAsc = asc
	s.updateDisplay()
}

func (s *AppState) ToggleSortOrder() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sortAsc = !s.sortAsc
	s.updateDisplay()
}

func (s *AppState) CycleSortColumn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sortColumn = (s.sortColumn % 7) + 1
	s.sortAsc = true
	s.updateDisplay()
}

func (s *AppState) SetFilterText(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.filterText = strings.ToLower(text)
	s.updateDisplay()
}

func (s *AppState) updateDisplay() {
	if s.current == nil {
		return
	}

	current := s.current.connections
	matches := s.previousMatches
	var vanished []models.Connection
	if s.showPinned {
		before := s.pinned.connections
		beforeMatched, currentMatched := matchSnapshots(before, current)
		matches = currentMatched
		for i, c := range before {
			if !beforeMatched[i] && s.matchesFilter(c) {
				vanished = append(vanished, c)
			}
		}
	} else {
		for _, entry := range s.recentlyVanished {
			if s.matchesFilter(entry.connection) {
				vanished = append(vanished, entry.connection)
			}
		}
	}

	var live []connectionRow
	for i, c := range current {
		if s.matchesFilter(c) {
			live = append(live, connectionRow{c, matches != nil && !matches[i]})
		}
	}
	slices.SortStableFunc(live, func(a, b connectionRow) int {
		order := compareConnections(a.connection, b.connection, s.sortColumn)
		if s.sortAsc {
			return order
		}
		return -order
	})
	s.filteredLive, s.filteredVanished = live, vanished
}

func (s *AppState) matchesFilter(c models.Connection) bool {
	if s.filterText == "" {
		return true
	}
	searchable := strings.Join([]string{c.ProcessName, c.Status, c.Family, c.Laddr, c.Raddr}, " ")
	return strings.Contains(strings.ToLower(searchable), s.filterText)
}

func compareConnections(a, b models.Connection, column int) int {
	switch column {
	case 1:
		return strings.Compare(strings.ToLower(a.ProcessName), strings.ToLower(b.ProcessName))
	case 2:
		return cmp.Compare(a.Pid, b.Pid)
	case 3:
		return strings.Compare(a.Status, b.Status)
	case 4:
		return strings.Compare(a.Family, b.Family)
	case 5:
		return strings.Compare(a.Type, b.Type)
	case 6:
		return strings.Compare(a.Laddr, b.Laddr)
	case 7:
		return strings.Compare(a.Raddr, b.Raddr)
	default:
		return 0
	}
}
