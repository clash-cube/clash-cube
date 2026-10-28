package devicepolicy

import (
	"github.com/sagernet/netlink"
	"golang.org/x/sys/unix"
	"net"
)

// Evict removes already-established NAT/forwarding decisions before a new
// restriction is acknowledged. Only this client's original source is affected.
func Evict(ip, mac string) error {
	f := &netlink.ConntrackFilter{}
	if err := f.AddIP(netlink.ConntrackOrigSrcIP, net.ParseIP(ip)); err != nil {
		return err
	}
	if _, err := netlink.ConntrackDeleteFilter(netlink.ConntrackTable, unix.AF_INET, f); err != nil {
		return err
	}
	for _, ip := range IPv6Addresses(mac) {
		filter := &netlink.ConntrackFilter{}
		if err := filter.AddIP(netlink.ConntrackOrigSrcIP, net.ParseIP(ip)); err != nil {
			return err
		}
		if _, err := netlink.ConntrackDeleteFilter(netlink.ConntrackTable, unix.AF_INET6, filter); err != nil {
			return err
		}
	}
	return nil
}
