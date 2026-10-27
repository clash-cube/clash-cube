package core

import (
	"net"
	"slices"
	"testing"
)

func TestListeningPortsReportsTCPListeners(t *testing.T) {
	tcp, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	tcpPort := tcp.Addr().(*net.TCPAddr).Port
	udpPort := udp.LocalAddr().(*net.UDPAddr).Port
	ports := listeningPorts()
	if !slices.Contains(ports, tcpPort) {
		t.Fatalf("TCP listener %d absent from %v", tcpPort, ports)
	}
	if udpPort != tcpPort && slices.Contains(ports, udpPort) {
		t.Fatal("UDP reported as a TCP listener")
	}
	tcp.Close()
	if slices.Contains(listeningPorts(), tcpPort) {
		t.Fatal("closed listener still reported")
	}
}
