package tui

import (
	"sort"
	"strings"
	"sync"

	"github.com/MrBrooks89/BatStat/internal/models"
)

type AppState struct {
	sync.RWMutex
	connections         []models.Connection // Master list of all connections
	filteredConnections []models.Connection // Connections after filtering
	filterText          string
	sortColumn          int
	sortAsc             bool
}

func NewAppState() *AppState {
	return &AppState{
		sortColumn: 0,
		sortAsc:    true,
	}
}

func (s *AppState) SetConnections(conns []models.Connection) {
	s.Lock()
	defer s.Unlock()
	s.connections = conns
	s.applySort()
	s.applyFilter()
}

func (s *AppState) GetFilteredConnections() []models.Connection {
	s.RLock()
	defer s.RUnlock()
	return s.filteredConnections
}

func (s *AppState) SetSort(column int, asc bool) {
	s.Lock()
	defer s.Unlock()
	s.sortColumn = column
	s.sortAsc = asc
	s.applySort()
	s.applyFilter()
}

func (s *AppState) ToggleSortOrder() {
	s.Lock()
	defer s.Unlock()
	s.sortAsc = !s.sortAsc
	s.applySort()
	s.applyFilter()
}

func (s *AppState) CycleSortColumn() {
	s.Lock()
	defer s.Unlock()
	s.sortColumn = (s.sortColumn % 7) + 1 
	s.sortAsc = true
	s.applySort()
	s.applyFilter()
}

func (s *AppState) SetFilterText(text string) {
	s.Lock()
	defer s.Unlock()
	s.filterText = text
	s.applyFilter()
}

func (s *AppState) applyFilter() {
	s.filteredConnections = nil
	normalizedFilter := strings.ToLower(s.filterText)

	if normalizedFilter == "" {
		s.filteredConnections = s.connections
		return
	}

	for _, c := range s.connections {
		searchable := strings.ToLower(
			c.ProcessName + " " +
				c.Status + " " +
				c.Family + " " +
				c.Laddr + " " +
				c.Raddr,
		)
		if strings.Contains(searchable, normalizedFilter) {
			s.filteredConnections = append(s.filteredConnections, c)
		}
	}
}

func (s *AppState) applySort() {
	sort.SliceStable(s.connections, func(i, j int) bool {
		c1, c2 := s.connections[i], s.connections[j]
		var less bool
		switch s.sortColumn {
		case 1:
			less = strings.ToLower(c1.ProcessName) < strings.ToLower(c2.ProcessName)
		case 2:
			less = c1.Pid < c2.Pid
		case 3:
			less = c1.Status < c2.Status
		case 4:
			less = c1.Family < c2.Family
		case 5:
			less = c1.Type < c2.Type
		case 6:
			less = c1.Laddr < c2.Laddr
		case 7:
			less = c1.Raddr < c2.Raddr
		default:
			return i < j
		}
		if !s.sortAsc {
			return !less
		}
		return less
	})
}
