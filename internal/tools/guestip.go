package tools

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Rishaad-AWN/mcp-virtualbox/internal/hostnet"
	"github.com/Rishaad-AWN/mcp-virtualbox/internal/vbox"
)

// guestIP is one address found for a VM, and where it came from.
type guestIP struct {
	IP     string `json:"ip"`
	Source string `json:"source"` // guest-additions | dhcp-lease | host-arp
	MAC    string `json:"mac,omitempty"`
	NIC    string `json:"nic,omitempty"`
}

// findGuestIPs asks Guest Additions first. VMs without them (routers, appliances,
// fresh installs) are found by matching their NIC MACs against VirtualBox's own
// DHCP leases and then the host ARP table.
func (d *deps) findGuestIPs(ctx context.Context, vm string) ([]guestIP, error) {
	out, err := d.vb.Run(ctx, "guestproperty", "enumerate", vm, "--patterns", "/VirtualBox/GuestInfo/Net/*/V4/IP")
	if err != nil {
		return nil, err
	}
	var res []guestIP
	for _, ip := range extractIPs(out) {
		res = append(res, guestIP{IP: ip, Source: "guest-additions"})
	}
	if len(res) > 0 {
		return res, nil
	}

	info, err := d.vb.Run(ctx, "showvminfo", vm, "--machinereadable")
	if err != nil {
		return nil, err
	}
	macs := vmMACs(vbox.ParseMachineReadable(info))
	if len(macs) == 0 {
		return nil, nil
	}
	seen := map[string]bool{}
	addFrom := func(table map[string]string, source string) {
		for mac, nic := range macs {
			if ip, ok := table[mac]; ok && !seen[ip] {
				seen[ip] = true
				res = append(res, guestIP{IP: ip, Source: source, MAC: formatMAC(mac), NIC: nic})
			}
		}
	}
	addFrom(dhcpLeases(vboxUserHome()), "dhcp-lease")
	addFrom(hostnet.ARPTable(ctx), "host-arp")
	sort.Slice(res, func(i, j int) bool { return nicNum(res[i].NIC) < nicNum(res[j].NIC) })
	return res, nil
}

// vmMACs maps normalised MAC -> "nicN (type)" for every enabled adapter.
func vmMACs(info map[string]string) map[string]string {
	macs := map[string]string{}
	for k, v := range info {
		if !strings.HasPrefix(k, "macaddress") {
			continue
		}
		n := strings.TrimPrefix(k, "macaddress")
		typ := info["nic"+n]
		if typ == "" || typ == "none" || typ == "null" {
			continue
		}
		if m := hostnet.NormMAC(v); m != "" {
			macs[m] = "nic" + n + " (" + typ + ")"
		}
	}
	return macs
}

// nicNum pulls N out of "nicN (type)" so nic2 sorts before nic10.
func nicNum(nic string) int {
	var n int
	fmt.Sscanf(nic, "nic%d", &n)
	return n
}

func formatMAC(m string) string {
	var b strings.Builder
	for i := 0; i < len(m); i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(m[i : i+2])
	}
	return b.String()
}

func vboxUserHome() string {
	if h := os.Getenv("VBOX_USER_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".VirtualBox")
}

type leaseFile struct {
	Leases []struct {
		MAC     string `xml:"mac,attr"`
		State   string `xml:"state,attr"`
		Address struct {
			Value string `xml:"value,attr"`
		} `xml:"Address"`
		Time struct {
			Issued     int64 `xml:"issued,attr"`
			Expiration int64 `xml:"expiration,attr"`
		} `xml:"Time"`
	} `xml:"Lease"`
}

type lease struct {
	ip     string
	issued int64
}

// dhcpLeases reads every *-Dhcpd.leases file VirtualBox's DHCP server writes
// (host-only and NAT networks) and returns MAC -> IP for acked, unexpired leases.
// A MAC leased on several networks (VM moved) keeps its newest lease.
func dhcpLeases(dir string) map[string]string {
	found := map[string]lease{}
	files, _ := filepath.Glob(filepath.Join(dir, "*Dhcpd.leases"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		parseLeases(data, time.Now().Unix(), found)
	}
	out := map[string]string{}
	for mac, l := range found {
		out[mac] = l.ip
	}
	return out
}

func parseLeases(data []byte, now int64, out map[string]lease) {
	var lf leaseFile
	if xml.Unmarshal(data, &lf) != nil {
		return
	}
	for _, l := range lf.Leases {
		if l.State != "acked" || l.Address.Value == "" {
			continue
		}
		if t := l.Time; t.Issued > 0 && t.Expiration > 0 && t.Issued+t.Expiration < now {
			continue // expired: the guest has moved on or is off
		}
		m := hostnet.NormMAC(l.MAC)
		if m == "" || out[m].issued > l.Time.Issued {
			continue
		}
		out[m] = lease{l.Address.Value, l.Time.Issued}
	}
}
