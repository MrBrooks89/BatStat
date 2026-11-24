package tui

import (
	"strconv"
	"strings"

	"github.com/MrBrooks89/BatStat/internal/models"
)

type connectionFilter struct {
	text    string
	clauses []func(models.Connection) bool
	problem string
}

func parseConnectionFilter(text string) connectionFilter {
	var filter connectionFilter
	var freeText strings.Builder
	remaining := text
	previousWasText := false
	for token := range strings.FieldsSeq(text) {
		gap, rest, _ := strings.Cut(remaining, token)
		remaining = rest
		field, value, hasColon := strings.Cut(token, ":")
		if !hasColon {
			field = ""
		}
		switch strings.ToLower(field) {
		case "pid":
			pid, err := strconv.ParseUint(value, 10, 31)
			if err != nil {
				return connectionFilter{problem: "Invalid PID"}
			}
			filter.clauses = append(filter.clauses, func(c models.Connection) bool {
				return c.Pid == int32(pid)
			})
		case "proto":
			if !strings.EqualFold(value, "tcp") && !strings.EqualFold(value, "udp") {
				return connectionFilter{problem: "Expected TCP or UDP"}
			}
			filter.clauses = append(filter.clauses, func(c models.Connection) bool {
				return matchesIPProtocol(c, value)
			})
		case "state":
			if value == "" {
				return connectionFilter{problem: "State is required"}
			}
			filter.clauses = append(filter.clauses, func(c models.Connection) bool {
				if strings.EqualFold(value, "listen") {
					return isTCPListener(c)
				}
				return strings.EqualFold(c.Status, value)
			})
		default:
			if previousWasText {
				freeText.WriteString(gap)
			} else if freeText.Len() > 0 {
				freeText.WriteByte(' ')
			}
			freeText.WriteString(token)
			previousWasText = true
			continue
		}
		previousWasText = false
	}
	if len(filter.clauses) > 0 {
		text = freeText.String()
	}
	filter.text = strings.ToLower(text)
	return filter
}

func (f connectionFilter) matches(c models.Connection) bool {
	if f.problem != "" {
		return false
	}
	for _, clause := range f.clauses {
		if !clause(c) {
			return false
		}
	}
	if f.text == "" {
		return true
	}
	searchable := strings.Join([]string{c.ProcessName, c.Status, c.Family, c.Laddr, c.Raddr}, " ")
	return strings.Contains(strings.ToLower(searchable), f.text)
}

func matchesIPProtocol(c models.Connection, protocol string) bool {
	return (c.Family == "IPv4" || c.Family == "IPv6") && strings.EqualFold(c.Type, protocol)
}

func isTCPListener(c models.Connection) bool {
	return matchesIPProtocol(c, "TCP") && strings.EqualFold(c.Status, "LISTEN")
}
