// Package dlna makes the shelf discoverable from a couch: SSDP announcements
// so televisions find the server, and a ContentDirectory they can browse.
package dlna

import (
	"fmt"
	"log"
	"net"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	multicastAddress = "239.255.255.250:1900"
	// How long a renderer should remember us between announcements.
	aliveFor = 30 * time.Second
	maxAge   = 1800
)

// Targets a MediaServer answers to. Renderers probe all of them.
func searchTargets(uuid string) []string {
	return []string{
		"upnp:rootdevice",
		"uuid:" + uuid,
		"urn:schemas-upnp-org:device:MediaServer:1",
		"urn:schemas-upnp-org:service:ContentDirectory:1",
	}
}

// Announcer is the SSDP voice of the server.
type Announcer struct {
	uuid     string
	port     int
	server   string
	conn     net.PacketConn
	stopOnce sync.Once
	stopped  chan struct{}
}

// Start announces the server and answers searches until Stop. friendlyName
// is what televisions display; the SERVER header follows the UPnP shape
// (OS/version UPnP/1.0 product/version) that strict renderers parse.
func Start(uuid string, port int, friendlyName string) (*Announcer, error) {
	serverName := fmt.Sprintf("%s/%s UPnP/1.0 watch/1.0", runtime.GOOS, runtime.Version())
	group, err := net.ResolveUDPAddr("udp4", multicastAddress)
	if err != nil {
		return nil, err
	}
	// ListenPacket alone would not join the group; ListenMulticastUDP does,
	// across every interface.
	conn, err := net.ListenMulticastUDP("udp4", nil, group)
	if err != nil {
		return nil, fmt.Errorf("join SSDP multicast: %w", err)
	}

	a := &Announcer{
		uuid:    uuid,
		port:    port,
		server:  serverName,
		conn:    conn,
		stopped: make(chan struct{}),
	}
	log.Printf("dlna: announcing on %s as %s (port %d)", multicastAddress, uuid, port)

	go a.readLoop()
	go a.aliveLoop(group)
	return a, nil
}

// Stop says goodbye and closes the socket.
func (a *Announcer) Stop() {
	a.stopOnce.Do(func() {
		close(a.stopped)
		group, err := net.ResolveUDPAddr("udp4", multicastAddress)
		if err == nil {
			for _, nt := range searchTargets(a.uuid) {
				packet := fmt.Sprintf(
					"NOTIFY * HTTP/1.1\r\nHOST: %s\r\nNT: %s\r\nNTS: ssdp:byebye\r\nUSN: %s\r\n\r\n",
					multicastAddress, nt, usnFor(a.uuid, nt),
				)
				_, _ = a.conn.WriteTo([]byte(packet), group)
			}
		}
		_ = a.conn.Close()
	})
}

func (a *Announcer) aliveLoop(group *net.UDPAddr) {
	a.notify(group, a.advertiseIP())
	ticker := time.NewTicker(aliveFor)
	defer ticker.Stop()
	for {
		select {
		case <-a.stopped:
			return
		case <-ticker.C:
			a.notify(group, a.advertiseIP())
		}
	}
}

func (a *Announcer) notify(group *net.UDPAddr, ip net.IP) {
	if ip == nil {
		return
	}
	location := fmt.Sprintf("http://%s:%d/dlna/device.xml", ip, a.port)
	now := time.Now().UTC().Format(time.RFC1123)
	for _, nt := range searchTargets(a.uuid) {
		packet := fmt.Sprintf(
			"NOTIFY * HTTP/1.1\r\nHOST: %s\r\nCACHE-CONTROL: max-age=%d\r\nLOCATION: %s\r\n"+
				"NT: %s\r\nNTS: ssdp:alive\r\nSERVER: %s\r\nUSN: %s\r\nDATE: %s\r\n\r\n",
			multicastAddress, maxAge, location, nt, a.server, usnFor(a.uuid, nt), now,
		)
		_, _ = a.conn.WriteTo([]byte(packet), group)
	}
}

func (a *Announcer) readLoop() {
	buffer := make([]byte, 2048)
	for {
		n, from, err := a.conn.ReadFrom(buffer)
		if err != nil {
			return // closed
		}
		packet := string(buffer[:n])
		if !strings.HasPrefix(packet, "M-SEARCH") {
			continue
		}
		st := headerOf(packet, "ST")
		if st == "" {
			continue
		}
		ip := a.ipToward(from.(*net.UDPAddr))
		if ip == nil {
			continue
		}
		a.answer(st, from.(*net.UDPAddr), ip)
	}
}

// answer replies to one M-SEARCH for one search target. ssdp:all gets the
// full set, which is what most renderers send.
func (a *Announcer) answer(st string, to *net.UDPAddr, ip net.IP) {
	targets := searchTargets(a.uuid)
	if st == "ssdp:all" {
		// the whole list, one response per target
	} else if !contains(targets, st) {
		return
	} else {
		targets = []string{st}
	}
	location := fmt.Sprintf("http://%s:%d/dlna/device.xml", ip, a.port)
	for _, target := range targets {
		packet := fmt.Sprintf(
			"HTTP/1.1 200 OK\r\nCACHE-CONTROL: max-age=%d\r\nDATE: %s\r\nEXT:\r\n"+
				"LOCATION: %s\r\nSERVER: %s\r\nST: %s\r\nUSN: %s\r\nCONTENT-LENGTH: 0\r\n\r\n",
			maxAge, time.Now().UTC().Format(time.RFC1123), location, a.server, target,
			usnFor(a.uuid, target),
		)
		if _, err := a.conn.WriteTo([]byte(packet), to); err != nil {
			return
		}
	}
}

// advertiseIP picks the address renderers should connect to: the one this
// machine would use to reach the outside world. No packet is sent.
func (a *Announcer) advertiseIP() net.IP {
	conn, err := net.Dial("udp4", "8.8.8.8:53")
	if err != nil {
		return nil
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP
}

// ipToward finds the local address that reaches a given renderer, so the
// answer carries a LOCATION on the right subnet.
func (a *Announcer) ipToward(to *net.UDPAddr) net.IP {
	conn, err := net.Dial("udp4", to.String())
	if err != nil {
		return a.advertiseIP()
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP
}

func usnFor(uuid, target string) string {
	if target == "uuid:"+uuid {
		return "uuid:" + uuid
	}
	return "uuid:" + uuid + "::" + target
}

func headerOf(packet, name string) string {
	for _, line := range strings.Split(packet, "\r\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// UUID makes a stable one the first time the server runs, so televisions
// remember this device across restarts instead of duplicating it.
func UUID(seed string) string {
	now := time.Now().Unix()
	return fmt.Sprintf("2ca1a7f0-%x-4e1a-b1a8-%x%x", now&0xffff, seed[:4], seed[4:8])
}
