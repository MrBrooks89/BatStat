package models

import (
	"fmt"
	"net/netip"
	"os/user"
	"runtime"
	"strconv"
	"strings"

	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type Connection struct {
	Fd          uint32
	FDKnown     bool
	Family      string
	Type        string
	Laddr       string
	Raddr       string
	Status      string
	Pid         int32
	ProcessName string
}

// HasFileDescriptor reports whether the observation includes a descriptor.
func (c Connection) HasFileDescriptor() bool {
	return c.FDKnown || c.Fd != 0
}

type DetailedInfo struct {
	Username string
	Cmdline  string
}

// LocalIP returns the IP of the local endpoint, or an invalid address if none is available.
func (c Connection) LocalIP() netip.Addr {
	if endpoint, err := netip.ParseAddrPort(c.Laddr); err == nil {
		return endpoint.Addr()
	}
	// Collected IPv6 endpoints have no brackets. The final colon separates
	// the address from the port.
	if separator := strings.LastIndexByte(c.Laddr, ':'); c.Family == "IPv6" && separator >= 0 {
		addr, _ := netip.ParseAddr(c.Laddr[:separator])
		return addr
	}
	return netip.Addr{}
}

func FromNetConnectionStat(stat net.ConnectionStat, procCache map[int32]*process.Process) (Connection, map[int32]*process.Process) {
	var procName string
	p, exists := procCache[stat.Pid]

	if !exists && stat.Pid > 0 {
		var err error
		p, err = process.NewProcess(stat.Pid)
		if err == nil {
			procCache[stat.Pid] = p
		}
	}

	if p != nil {
		procName, _ = p.Name()
	}

	return Connection{
		Fd: stat.Fd,
		// For Linux system-wide snapshots, an attributed PID comes from
		// a /proc/<pid>/fd entry. Descriptor zero is valid there.
		FDKnown:     stat.Fd != 0 || (runtime.GOOS == "linux" && stat.Pid > 0),
		Family:      mapFamily(stat.Family),
		Type:        mapType(stat.Type),
		Laddr:       fmt.Sprintf("%s:%d", stat.Laddr.IP, stat.Laddr.Port),
		Raddr:       fmt.Sprintf("%s:%d", stat.Raddr.IP, stat.Raddr.Port),
		Status:      stat.Status,
		Pid:         stat.Pid,
		ProcessName: procName,
	}, procCache
}

func GetDetailedInfo(pid int32) DetailedInfo {
	if pid == 0 {
		return DetailedInfo{Username: "N/A", Cmdline: "N/A"}
	}

	p, err := process.NewProcess(pid)
	if err != nil {
		return DetailedInfo{Username: "error", Cmdline: "error fetching process data"}
	}

	cmdline, _ := p.Cmdline()
	uids, err := p.Uids()
	username := "N/A"
	if err == nil && len(uids) > 0 {
		u, err := user.LookupId(strconv.Itoa(int(uids[0])))
		if err == nil {
			username = u.Username
		}
	}

	return DetailedInfo{
		Username: username,
		Cmdline:  cmdline,
	}
}

func mapFamily(f uint32) string {
	switch f {
	case 2:
		return "IPv4"
	case 10, 24, 26, 28:
		return "IPv6"
	case 1:
		return "Unix"
	default:
		return "Unknown"
	}
}

func mapType(t uint32) string {
	switch t {
	case 1:
		return "TCP"
	case 2:
		return "UDP"
	default:
		return "Unknown"
	}
}
