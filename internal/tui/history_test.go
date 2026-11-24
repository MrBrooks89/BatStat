package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MrBrooks89/BatStat/internal/models"
	"github.com/gdamore/tcell/v2"
)

func TestConnectionHistoryRecordsObservationsAndStartsOverAfterAbsence(t *testing.T) {
	state := NewAppState()
	start := time.Unix(100, 0)
	socket := models.Connection{Pid: 10, Fd: 9, Type: "TCP", Status: "ESTABLISHED"}
	state.SetConnections([]models.Connection{socket}, start)
	rows, _ := state.displayedHistory()
	id := rows[0].id

	socket.Status = "CLOSE_WAIT"
	changed := start.Add(3 * time.Second)
	lastSeen := start.Add(6 * time.Second)
	firstMissing := start.Add(time.Minute)
	state.SetConnections([]models.Connection{socket}, changed)
	state.SetConnections([]models.Connection{socket}, lastSeen)
	state.SetConnections(nil, firstMissing)
	state.SetConnections([]models.Connection{socket}, firstMissing.Add(3*time.Second))

	rows, _ = state.displayedHistory()
	if len(rows) != 2 {
		t.Fatalf("history has %d records, want one vanished and one active", len(rows))
	}
	vanished, active := rows[0], rows[1]
	if vanished.id != id || vanished.firstSeen != start || vanished.lastSeen != lastSeen || vanished.firstMissing != firstMissing {
		t.Fatalf("unexpected vanished record: %+v", vanished)
	}
	if want := (observedStateChange{changed, "ESTABLISHED", "CLOSE_WAIT"}); len(vanished.changes) != 1 || vanished.changes[0] != want {
		t.Fatalf("observed state changes = %+v, want %+v", vanished.changes, want)
	}
	if active.id == id || !active.firstMissing.IsZero() || active.firstSeen != firstMissing.Add(3*time.Second) || len(active.changes) != 0 {
		t.Fatalf("unexpected new active record: %+v", active)
	}
}

func TestConnectionHistoryMatchesSharedEndpointsByDescriptor(t *testing.T) {
	state := NewAppState()
	start := time.Unix(100, 0)
	first := models.Connection{Pid: 10, Fd: 8, Type: "UDP", Laddr: "192.0.2.2:8080", Status: "NONE"}
	second := first
	second.Fd = 9
	state.SetConnections([]models.Connection{first, second}, start)
	state.SetConnections([]models.Connection{second, first}, start.Add(time.Second))
	state.SetConnections([]models.Connection{second}, start.Add(2*time.Second))
	rows, _ := state.displayedHistory()
	if len(rows) != 2 || rows[0].latest.Fd != first.Fd || rows[0].firstMissing.IsZero() ||
		rows[1].latest.Fd != second.Fd || rows[1].firstSeen != start || rows[1].identityUncertain {
		t.Fatalf("history did not identify the vanished socket by descriptor: %+v", rows)
	}
}

func TestConnectionHistoryDistinguishesDescriptorZeroFromMissingDescriptor(t *testing.T) {
	state := NewAppState()
	start := time.Unix(100, 0)
	socket := models.Connection{Pid: 10, Fd: 0, FDKnown: true, Type: "TCP", Status: "ESTABLISHED"}
	state.SetConnections([]models.Connection{socket}, start)
	socket.Fd = 8
	state.SetConnections([]models.Connection{socket}, start.Add(3*time.Second))
	rows, _ := state.displayedHistory()
	if len(rows) != 2 || rows[0].firstMissing.IsZero() || rows[0].latest.Fd != 0 || rows[1].latest.Fd != 8 {
		t.Fatalf("descriptor zero and descriptor eight should identify separate sockets: %+v", rows)
	}
}

func TestConnectionHistoryFlagsUncertainIdentityWithoutObservedStateChanges(t *testing.T) {
	state := NewAppState()
	start := time.Unix(100, 0)
	first := models.Connection{Pid: 10, Type: "TCP", Status: "ESTABLISHED"}
	second := first
	second.Status = "CLOSE_WAIT"
	state.SetConnections([]models.Connection{first, second}, start)
	second.Status = "TIME_WAIT"
	state.SetConnections([]models.Connection{second}, start.Add(3*time.Second))
	rows, _ := state.displayedHistory()
	if len(rows) != 2 || rows[0].firstMissing.IsZero() || !rows[1].firstMissing.IsZero() {
		t.Fatalf("history should show that one socket vanished: %+v", rows)
	}
	for _, record := range rows {
		if !record.identityUncertain || len(record.changes) > 0 || !strings.Contains(record.details(), "Identity uncertain") {
			t.Fatalf("ambiguous record should be flagged without a state change: %+v", record)
		}
	}
}

func TestConnectionHistoryFlagsUncertainIdentityAcrossMissingIdentityGroups(t *testing.T) {
	for _, test := range []struct {
		name           string
		known, unknown models.Connection
	}{
		{
			name:    "file descriptor",
			known:   models.Connection{Pid: 10, Fd: 8, Type: "TCP", Status: "ESTABLISHED"},
			unknown: models.Connection{Pid: 10, Type: "TCP", Status: "CLOSE_WAIT"},
		},
		{
			name:    "owner",
			known:   models.Connection{Pid: 10, Type: "TCP", Status: "ESTABLISHED"},
			unknown: models.Connection{Type: "TCP", Status: "CLOSE_WAIT"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := NewAppState()
			start := time.Unix(100, 0)
			state.SetConnections([]models.Connection{test.known, test.unknown}, start)
			remaining := test.unknown
			remaining.Status = "TIME_WAIT"
			state.SetConnections([]models.Connection{remaining}, start.Add(3*time.Second))
			rows, _ := state.displayedHistory()
			if len(rows) != 2 || rows[0].firstMissing.IsZero() || !rows[1].firstMissing.IsZero() {
				t.Fatalf("history should show one vanished and one active socket: %+v", rows)
			}
			for _, record := range rows {
				if !record.identityUncertain || len(record.changes) != 0 {
					t.Fatalf("either old observation could match the remaining socket: %+v", record)
				}
			}
		})
	}
}

func TestConnectionHistoryKeepsKnownOwnersSeparateAcrossMissingPID(t *testing.T) {
	state := NewAppState()
	start := time.Unix(100, 0)
	socket := models.Connection{Pid: 10, Fd: 8, Type: "TCP", Status: "ESTABLISHED"}
	state.SetConnections([]models.Connection{socket}, start)
	socket.Pid, socket.Fd = 0, 0
	state.SetConnections([]models.Connection{socket}, start.Add(3*time.Second))
	rows, _ := state.displayedHistory()
	if len(rows) != 1 || rows[0].firstSeen != start || rows[0].latest.Pid != 0 {
		t.Fatalf("history should keep continuity and show the latest reported owner: %+v", rows)
	}
	socket.Pid, socket.Fd = 11, 8
	state.SetConnections([]models.Connection{socket}, start.Add(6*time.Second))
	rows, _ = state.displayedHistory()
	if len(rows) != 2 || rows[0].firstMissing.IsZero() || rows[1].firstSeen == start {
		t.Fatalf("history joined observations with different known owners: %+v", rows)
	}
}

func TestConnectionHistoryDropsGuessedOwnerAfterUncertainIdentity(t *testing.T) {
	state := NewAppState()
	start := time.Unix(100, 0)
	first := models.Connection{Pid: 10, Type: "TCP", Status: "ESTABLISHED"}
	second := first
	second.Pid = 11
	state.SetConnections([]models.Connection{first, second}, start)
	ownerless := first
	ownerless.Pid = 0
	state.SetConnections([]models.Connection{ownerless}, start.Add(3*time.Second))
	rows, _ := state.displayedHistory()
	if len(rows) != 2 || !rows[1].identityUncertain {
		t.Fatalf("the identity of the surviving record should be uncertain: %+v", rows)
	}
	survivor := rows[1].id

	ownerKnown := first
	ownerKnown.Pid = rows[0].latest.Pid
	at := start.Add(6 * time.Second)
	state.SetConnections([]models.Connection{ownerKnown}, at)
	rows, _ = state.displayedHistory()
	if len(rows) != 2 || rows[1].id != survivor || rows[1].lastSeen != at || rows[1].latest.Pid != ownerKnown.Pid {
		t.Fatalf("history split an uninterrupted observation when its owner became available: %+v", rows)
	}
}

func TestConnectionHistoryCapsActiveAndVanishedHistoryRecordsIndependently(t *testing.T) {
	state := NewAppState()
	start := time.Unix(100, 0)
	snapshot := make([]models.Connection, maxActiveHistory)
	for i := range snapshot {
		snapshot[i].Pid = int32(i + 1)
	}
	state.SetConnections(snapshot, start)

	newcomer := models.Connection{Pid: int32(len(snapshot) + 1)}
	overflow := append([]models.Connection{newcomer}, snapshot...)
	state.SetConnections(overflow, start.Add(time.Second))
	rows, untracked := state.displayedHistory()
	if len(rows) != maxActiveHistory || untracked != 1 {
		t.Fatalf("history retained %d records and reported %d live outside history", len(rows), untracked)
	}
	for _, record := range rows {
		if record.latest.Pid == newcomer.Pid || record.firstSeen != start {
			t.Fatal("an earlier-listed newcomer displaced a tracked active record")
		}
	}

	// Free a slot, then archive the remaining sockets in a later snapshot.
	overflow = append(overflow[:1], overflow[2:]...)
	state.SetConnections(overflow, start.Add(2*time.Second))
	rows, untracked = state.displayedHistory()
	if untracked != 0 || len(rows) != maxActiveHistory+1 || rows[0].latest.Pid != 1 ||
		rows[1].latest.Pid != newcomer.Pid || rows[1].firstSeen != start.Add(2*time.Second) {
		t.Fatal("history should archive the vanished socket and admit the newcomer")
	}
	state.SetConnections(nil, start.Add(3*time.Second))
	rows, _ = state.displayedHistory()
	if len(rows) != maxVanishedHistory {
		t.Fatalf("history holds %d vanished records, want %d", len(rows), maxVanishedHistory)
	}
	for _, record := range rows {
		if record.firstMissing != start.Add(3*time.Second) || record.latest.Pid == 1 {
			t.Fatal("the oldest vanished record should be evicted first")
		}
	}
}

func TestConnectionHistoryRetainsOnlyLatestObservedStateChanges(t *testing.T) {
	state := NewAppState()
	start := time.Unix(100, 0)
	socket := models.Connection{Pid: 10, Status: "0"}
	state.SetConnections([]models.Connection{socket}, start)
	for i := 1; i <= maxHistoryChanges+4; i++ {
		socket.Status = strconv.Itoa(i)
		state.SetConnections([]models.Connection{socket}, start.Add(time.Duration(i)*time.Second))
	}
	rows, _ := state.displayedHistory()
	record := rows[0]
	if record.firstSeen != start || record.lastSeen != start.Add(time.Duration(maxHistoryChanges+4)*time.Second) ||
		!record.trimmedChanges || len(record.changes) != maxHistoryChanges ||
		record.changes[0].from != "4" || record.changes[maxHistoryChanges-1].to != socket.Status ||
		!strings.Contains(record.details(), "Older changes were trimmed") {
		t.Fatalf("unexpected record after trimming state changes: %+v", record)
	}
}

func TestConnectionHistoryViewNavigationFiltersAndPausedMonitoring(t *testing.T) {
	app := NewApp()
	app.view.Init()
	app.setKeybindings()
	const addr = "127.0.0.1:8080"
	listener := models.Connection{Pid: 10, Family: "IPv4", Type: "TCP", Status: "LISTEN", Laddr: addr}
	client := models.Connection{Pid: 20, Family: "IPv4", Type: "TCP", Status: "ESTABLISHED"}
	start := time.Unix(100, 0)
	firstID, _ := app.beginRefresh()
	app.applyRefresh(firstID, []models.Connection{listener, client}, start)
	inFlight, _ := app.beginRefresh()
	app.setPaused(true)
	app.applyRefresh(inFlight, []models.Connection{client}, start.Add(time.Second))

	processKey(app, tcell.KeyRune, 'P')
	processKey(app, tcell.KeyRune, 'H')
	history := &app.view.history
	if !history.table.HasFocus() || len(history.rows) != 2 || strings.Contains(app.view.detailsView.GetText(true), "First missing") {
		t.Fatal("the in-flight refresh discarded by Pause should leave History unchanged")
	}

	app.setPaused(false)
	// A failed fetch never reaches applyRefresh.
	if _, ok := app.beginRefresh(); !ok {
		t.Fatal("expected a refresh to start after resume")
	}
	if len(history.rows) != 2 || strings.Contains(app.view.detailsView.GetText(true), "First missing") {
		t.Fatal("starting a refresh should leave History unchanged")
	}
	resumed, _ := app.beginRefresh()
	app.applyRefresh(resumed, []models.Connection{client}, start.Add(time.Minute))
	if history.rows[0].latest.Laddr != addr || history.rows[0].firstMissing != start.Add(time.Minute) ||
		history.rows[0].lastSeen != start || history.selectedRecord().latest.Pid != client.Pid {
		t.Fatal("the vanished record should move first while the selected active record keeps focus")
	}
	processKey(app, tcell.KeyRune, 'L')
	processKey(app, tcell.KeyRune, '/')
	app.view.filterInput.SetText("pid:10")
	processKey(app, tcell.KeyEnter, 0)
	if !history.table.HasFocus() || len(history.rows) != 1 || !strings.Contains(app.view.detailsView.GetText(true), "First missing") {
		t.Fatal("History filters should match the vanished listener's latest observation")
	}
	processKey(app, tcell.KeyTab, 0)
	if !app.view.detailsView.HasFocus() {
		t.Fatal("Tab should focus the timeline for scrolling")
	}
	for _, key := range "kKpnteBbvsSP" {
		processKey(app, tcell.KeyRune, key)
		if app.view.IsModalActive() || !app.view.showHistory {
			t.Fatalf("%q should leave the inspection view open", key)
		}
	}
	processKey(app, tcell.KeyRune, 'H')
	if !app.view.processes.table.HasFocus() || len(history.rows) != 0 {
		t.Fatal("H should return to Process View and release the hidden history rows")
	}
	processKey(app, tcell.KeyRune, 'P')
	processKey(app, tcell.KeyTab, 0)
	if !app.view.vanishedTable.HasFocus() {
		t.Fatal("expected vanished Connections list to have focus")
	}
	processKey(app, tcell.KeyRune, 'H')
	processKey(app, tcell.KeyRune, 'H')
	if !app.view.vanishedTable.HasFocus() {
		t.Fatal("H should return to the vanished Connections list")
	}
}
