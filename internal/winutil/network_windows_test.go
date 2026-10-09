package winutil

import (
	"net"
	"os"
	"testing"
)

func TestListenersReportsOwningProcess(t *testing.T) {
	for _, network := range []string{"tcp4", "tcp6"} {
		t.Run(network, func(t *testing.T) {
			host := "127.0.0.1:0"
			if network == "tcp6" {
				host = "[::1]:0"
			}
			listener, err := net.Listen(network, host)
			if err != nil {
				if network == "tcp6" {
					t.Skip(err)
				}
				t.Fatal(err)
			}
			defer listener.Close()
			port := listener.Addr().(*net.TCPAddr).Port
			rows, err := Listeners()
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range rows {
				if row.Port == port && row.PID == uint32(os.Getpid()) {
					return
				}
			}
			t.Fatalf("listening socket %d missing from owner table", port)
		})
	}
}
