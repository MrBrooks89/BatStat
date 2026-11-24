package tui

import "github.com/MrBrooks89/BatStat/internal/models"

const (
	vanishedRefreshes   = 3
	maxRecentlyVanished = 20
)

type connectionEndpoints struct {
	family, protocol, local, remote string
}

func endpoints(c models.Connection) connectionEndpoints {
	return connectionEndpoints{c.Family, c.Type, c.Laddr, c.Raddr}
}

type ownedConnection struct {
	endpoints connectionEndpoints
	pid       int32
}

// matchSnapshots pairs observations one-to-one. Known owners get first pick,
// then an unknown owner on either side can match a remaining observation.
func matchSnapshots(before, after []models.Connection) (beforeMatched, afterMatched []bool) {
	beforeMatched = make([]bool, len(before))
	afterMatched = make([]bool, len(after))
	pair := func(candidates []int, afterIndex int) []int {
		if len(candidates) == 0 {
			return nil
		}
		beforeMatched[candidates[0]], afterMatched[afterIndex] = true, true
		return candidates[1:]
	}
	owned := make(map[ownedConnection][]int)
	for i, c := range before {
		if c.Pid > 0 {
			key := ownedConnection{endpoints(c), c.Pid}
			owned[key] = append(owned[key], i)
		}
	}
	for i, c := range after {
		if c.Pid > 0 {
			key := ownedConnection{endpoints(c), c.Pid}
			owned[key] = pair(owned[key], i)
		}
	}

	available := make(map[connectionEndpoints][]int)
	for i, c := range before {
		if c.Pid <= 0 {
			key := endpoints(c)
			available[key] = append(available[key], i)
		}
	}
	for i, c := range after {
		if afterMatched[i] || c.Pid <= 0 {
			continue
		}
		key := endpoints(c)
		available[key] = pair(available[key], i)
	}

	clear(available)
	for i, c := range before {
		if !beforeMatched[i] {
			key := endpoints(c)
			available[key] = append(available[key], i)
		}
	}
	for i, c := range after {
		if c.Pid > 0 {
			continue
		}
		key := endpoints(c)
		available[key] = pair(available[key], i)
	}
	return beforeMatched, afterMatched
}

func unmatchedConnections(conns []models.Connection, matched []bool) []models.Connection {
	var unmatched []models.Connection
	for i, c := range conns {
		if !matched[i] {
			unmatched = append(unmatched, c)
		}
	}
	return unmatched
}

type recentlyVanished struct {
	connection    models.Connection
	refreshesLeft int
}

func updateRecentlyVanished(history []recentlyVanished, vanished, added []models.Connection) []recentlyVanished {
	next := make([]recentlyVanished, 0, maxRecentlyVanished)
	for _, c := range vanished[:min(len(vanished), maxRecentlyVanished)] {
		next = append(next, recentlyVanished{c, vanishedRefreshes})
	}
	if len(next) == maxRecentlyVanished || len(history) == 0 {
		return next
	}

	var retained []recentlyVanished
	var retainedConnections []models.Connection
	for _, entry := range history {
		if entry.refreshesLeft > 1 {
			entry.refreshesLeft--
			retained = append(retained, entry)
			retainedConnections = append(retainedConnections, entry.connection)
		}
	}
	returned, _ := matchSnapshots(retainedConnections, added)
	for i, entry := range retained {
		if !returned[i] {
			next = append(next, entry)
			if len(next) == maxRecentlyVanished {
				break
			}
		}
	}
	return next
}
