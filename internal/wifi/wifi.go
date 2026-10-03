// Package wifi reads the connected Wi-Fi network without scanning or changing it.
package wifi

// Status distinguishes an unreadable SSID from a confirmed disconnection.
type Status struct {
	SSID      string `json:"ssid"`
	State     string `json:"state"`     // connected | disconnected | permission | denied | unavailable
	Interface string `json:"interface"` // the Wi-Fi interface, e.g. en0; "" without one
}
