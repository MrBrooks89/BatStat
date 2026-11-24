package tui

import (
	"strings"
	"testing"

	"github.com/MrBrooks89/BatStat/internal/models"
)

func TestConnectionFilter(t *testing.T) {
	listener := models.Connection{
		ProcessName: "Web  Server", Pid: 1234, Type: "TCP", Status: "LISTEN", Family: "IPv6", Laddr: "::1:8080",
	}
	unix := models.Connection{Type: "TCP", Status: "LISTEN", Family: "Unix"}
	udp := models.Connection{Type: "UDP", Status: "NONE", Family: "IPv4"}
	for _, test := range []struct {
		query   string
		row     models.Connection
		matches bool
		invalid bool
	}{
		{query: "STATE:LiStEn pid:01234 Proto:TCP", row: listener, matches: true},
		{query: "pid:123", row: listener},
		{query: "pid:1234 pid:1235", row: listener},
		{query: "pid:0", row: udp, matches: true},
		{query: "proto:udp state:none", row: udp, matches: true},
		{query: "proto:tcp", row: unix},
		{query: "state:listen", row: unix},
		{query: "state:lis", row: listener},
		{query: "Web proto:tcp", row: listener, matches: true},
		{query: "Web state:established", row: listener},
		{query: "web  server", row: listener, matches: true},
		{query: "web server", row: listener},
		{query: "web  server proto:tcp", row: listener, matches: true},
		{query: "proto:tcp web  server pid:1234", row: listener, matches: true},
		{query: "::1:8080", row: listener, matches: true},
		{query: "proto:tcp ::1:8080", row: listener, matches: true},
		{query: "name:web", row: listener},
		{query: "pid:", row: listener, invalid: true},
		{query: "pid:abc", row: listener, invalid: true},
		{query: "pid:-1", row: listener, invalid: true},
		{query: "pid:2147483648", row: listener, invalid: true},
		{query: "proto:icmp", row: listener, invalid: true},
		{query: "state:", row: listener, invalid: true},
	} {
		t.Run(test.query, func(t *testing.T) {
			filter := parseConnectionFilter(test.query)
			if got := filter.problem != ""; got != test.invalid {
				t.Errorf("problem = %q, want invalid = %t", filter.problem, test.invalid)
			}
			if got := filter.matches(test.row); got != test.matches {
				t.Errorf("match = %t, want %t", got, test.matches)
			}
		})
	}
}

func TestListenerBindingStatus(t *testing.T) {
	for _, test := range []struct {
		addr   string
		family string
		label  string
	}{
		{"127.0.0.2:80", "IPv4", "Loopback"},
		{"::1:80", "IPv6", "Loopback"},
		{"0.0.0.0:80", "IPv4", "All interfaces"},
		{":::80", "IPv6", "All interfaces"},
		{"[::]:80", "IPv6", "All interfaces"},
		{"192.168.1.10:80", "IPv4", ""},
		{"2001:db8::1:80", "IPv6", ""},
		{":80", "IPv4", ""},
	} {
		t.Run(test.addr, func(t *testing.T) {
			row := models.Connection{Status: "LISTEN", Type: "TCP", Family: test.family, Laddr: test.addr}
			want := row.Status
			if test.label != "" {
				want += " · " + test.label
			}
			if got := displayStatus(row); got != want {
				t.Errorf("status = %q, want %q", got, want)
			}
			row.Status = "ESTABLISHED"
			if got := displayStatus(row); strings.Contains(got, " · ") {
				t.Errorf("established socket has a binding label: %q", got)
			}
		})
	}
}
