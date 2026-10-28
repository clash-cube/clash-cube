package devicepolicy

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// DNSHandler has no direct resolver fallback. The only public upstream is a
// loopback TCP tunnel whose mihomo listener pins the device's cloned adapter.
func DNSHandler(home, id string, running func() bool, local func(string) net.IP) dns.Handler {
	return dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
		a := new(dns.Msg)
		a.SetReply(q)
		c, err := Load(home)
		p, e := c.Find(id)
		if err != nil || e != nil || len(q.Question) != 1 {
			a.Rcode = dns.RcodeServerFailure
			_ = w.WriteMsg(a)
			return
		}
		question := q.Question[0]
		if question.Qclass != dns.ClassINET {
			a.Rcode = dns.RcodeRefused
			_ = w.WriteMsg(a)
			return
		}
		if ip := local(question.Name); ip != nil {
			if question.Qtype == dns.TypeA && ip.To4() != nil {
				a.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30}, A: ip.To4()}}
			}
			_ = w.WriteMsg(a)
			return
		}
		if strings.HasSuffix(strings.ToLower(question.Name), ".lan.") {
			a.Rcode = dns.RcodeNameError
			_ = w.WriteMsg(a)
			return
		}
		if p.Paused || p.Pending || !running() {
			a.Rcode = dns.RcodeServerFailure
			_ = w.WriteMsg(a)
			return
		}
		if question.Qtype == dns.TypeAAAA {
			_ = w.WriteMsg(a)
			return
		}
		client := dns.Client{Net: "tcp", Timeout: 4 * time.Second}
		reply, _, err := client.Exchange(q, net.JoinHostPort("127.0.0.1", strconv.Itoa(p.TunnelPort())))
		if err != nil {
			a.Rcode = dns.RcodeServerFailure
			_ = w.WriteMsg(a)
			return
		}
		_ = w.WriteMsg(reply)
	})
}

// RunDNS lives in the core supervisor, so replacing management leaves DNS
// sessions intact. Losing this process closes the relay; it cannot enable DNS
// fallback because the independent firewall still captures port 53.
func RunDNS(ctx context.Context, home string, running func() bool) {
	var hosts atomic.Value
	hosts.Store(map[string]net.IP{})
	local := func(name string) net.IP {
		return hosts.Load().(map[string]net.IP)[strings.TrimSuffix(strings.ToLower(name), ".")]
	}
	type pair struct {
		udp, tcp *dns.Server
		address  string
	}
	servers := map[string]pair{}
	defer func() {
		for _, s := range servers {
			_ = s.udp.Shutdown()
			_ = s.tcp.Shutdown()
		}
	}()
	sync := func() {
		// Discovery invokes ubus/ip. Do it once per tick rather than spawning
		// several router processes for every individual DNS question.
		hosts.Store(LocalHosts())
		c, err := Load(home)
		if err != nil {
			return
		}
		wanted := map[string]bool{}
		for _, p := range c.Policies {
			wanted[p.ID] = true
			addr := net.JoinHostPort(c.LAN.Address, strconv.Itoa(p.DNSPort()))
			if old, ok := servers[p.ID]; ok && old.address == addr {
				continue
			}
			if old, ok := servers[p.ID]; ok {
				_ = old.udp.Shutdown()
				_ = old.tcp.Shutdown()
				delete(servers, p.ID)
			}
			udp, e := net.ListenPacket("udp", addr)
			if e != nil {
				continue
			}
			tcp, e := net.Listen("tcp", addr)
			if e != nil {
				_ = udp.Close()
				continue
			}
			h := DNSHandler(home, p.ID, running, local)
			s := pair{udp: &dns.Server{PacketConn: udp, Handler: h}, tcp: &dns.Server{Listener: tcp, Handler: h}, address: addr}
			servers[p.ID] = s
			go s.udp.ActivateAndServe()
			go s.tcp.ActivateAndServe()
		}
		for id, s := range servers {
			if !wanted[id] {
				_ = s.udp.Shutdown()
				_ = s.tcp.Shutdown()
				delete(servers, id)
			}
		}
	}
	sync()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sync()
		}
	}
}
