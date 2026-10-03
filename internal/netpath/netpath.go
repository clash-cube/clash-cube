// Package netpath reads what macOS knows about the network the Mac is on:
// whether it is expensive (a personal hotspot) or constrained (Low Data
// Mode). It needs no permission.
package netpath

type Path struct {
	Expensive   bool `json:"expensive"`
	Constrained bool `json:"constrained"`
}
