package tui

import "github.com/MrBrooks89/BatStat/internal/models"

type historyMatch struct {
	pairedIndexPlusOne int // Zero means unmatched.
	uncertain          bool
}

type historySocket struct {
	ownedConnection
	fd uint32
}

type historyMatchClass int

const (
	unknownHistoryIdentity historyMatchClass = iota
	knownHistoryIdentity
	ineligibleHistoryIdentity
)

// Match strong evidence first. A weaker match can fill in a missing descriptor
// or owner, but cannot join two sockets with conflicting known descriptors or PIDs.
func matchHistorySnapshots(before, after []models.Connection) ([]historyMatch, []historyMatch) {
	oldMatches, newMatches := make([]historyMatch, len(before)), make([]historyMatch, len(after))
	if len(before) == 0 || len(after) == 0 {
		return oldMatches, newMatches
	}
	matchHistoryGroups(before, after, oldMatches, newMatches,
		func(c models.Connection) (historySocket, historyMatchClass) {
			if c.Pid <= 0 || !c.HasFileDescriptor() {
				return historySocket{}, ineligibleHistoryIdentity
			}
			return historySocket{ownedConnection{endpoints(c), c.Pid}, c.Fd}, unknownHistoryIdentity
		})
	matchHistoryGroups(before, after, oldMatches, newMatches,
		func(c models.Connection) (ownedConnection, historyMatchClass) {
			if c.Pid <= 0 {
				return ownedConnection{}, ineligibleHistoryIdentity
			}
			return ownedConnection{endpoints(c), c.Pid}, classifyHistoryIdentity(c.HasFileDescriptor())
		})
	matchHistoryGroups(before, after, oldMatches, newMatches,
		func(c models.Connection) (connectionEndpoints, historyMatchClass) {
			return endpoints(c), classifyHistoryIdentity(c.Pid > 0)
		})
	return oldMatches, newMatches
}

type historyMatchGroup struct {
	before, after [2][]int // Unknown and known values for this pass's identity field.
}

func matchHistoryGroups[K comparable](
	before, after []models.Connection, oldMatches, newMatches []historyMatch,
	keyFor func(models.Connection) (K, historyMatchClass),
) {
	groups := make(map[K]*historyMatchGroup)
	for i, c := range before {
		if oldMatches[i].pairedIndexPlusOne != 0 {
			continue
		}
		key, class := keyFor(c)
		if class == ineligibleHistoryIdentity {
			continue
		}
		group := groups[key]
		if group == nil {
			group = &historyMatchGroup{}
			groups[key] = group
		}
		group.before[class] = append(group.before[class], i)
	}
	for i, c := range after {
		if newMatches[i].pairedIndexPlusOne != 0 {
			continue
		}
		key, class := keyFor(c)
		group := groups[key]
		if class == ineligibleHistoryIdentity || group == nil {
			continue
		}
		group.after[class] = append(group.after[class], i)
	}
	for _, group := range groups {
		group.match(oldMatches, newMatches)
	}
}

func classifyHistoryIdentity(known bool) historyMatchClass {
	if known {
		return knownHistoryIdentity
	}
	return unknownHistoryIdentity
}

func (g *historyMatchGroup) match(oldMatches, newMatches []historyMatch) {
	// A known value can only pair with a missing value in the fallback passes.
	// Pair those observations first, preserving matches for the less constrained
	// unknown-to-unknown case.
	pairs := [...][2]historyMatchClass{
		{knownHistoryIdentity, unknownHistoryIdentity},
		{unknownHistoryIdentity, knownHistoryIdentity},
		{unknownHistoryIdentity, unknownHistoryIdentity},
	}
	// Count alternatives before pairing. An observation with missing identity
	// can have candidates in both groups, even when each pair of groups is 1:1.
	for _, pair := range pairs {
		oldClass, newClass := pair[0], pair[1]
		before, after := g.before[oldClass], g.after[newClass]
		if len(before) == 0 || len(after) == 0 {
			continue
		}
		if historyCandidateCount(oldClass, g.after) > 1 || historyCandidateCount(newClass, g.before) > 1 {
			for _, i := range before {
				oldMatches[i].uncertain = true
			}
			for _, i := range after {
				newMatches[i].uncertain = true
			}
		}
	}
	for _, pair := range pairs {
		oldClass, newClass := pair[0], pair[1]
		before, after := g.before[oldClass], g.after[newClass]
		count := min(len(before), len(after))
		for i := range count {
			oldMatches[before[i]].pairedIndexPlusOne = after[i] + 1
			newMatches[after[i]].pairedIndexPlusOne = before[i] + 1
		}
		g.before[oldClass], g.after[newClass] = before[count:], after[count:]
	}
}

func historyCandidateCount(class historyMatchClass, candidates [2][]int) int {
	count := len(candidates[unknownHistoryIdentity])
	if class == unknownHistoryIdentity {
		count += len(candidates[knownHistoryIdentity])
	}
	return count
}
