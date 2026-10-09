//go:build darwin

package sysproxy

import "testing"

func TestParseEffective(t *testing.T) {
	out := `<dictionary> {
  ExceptionsList : <array> {
    0 : 127.0.0.1
  }
  HTTPEnable : 1
  HTTPPort : 7890
  HTTPProxy : 127.0.0.1
  HTTPSEnable : 1
}
`
	if !parseEffective(out, "127.0.0.1", 7890) {
		t.Error("ours not seen")
	}
	if parseEffective(out, "127.0.0.1", 7891) {
		t.Error("another port taken for ours")
	}
	if parseEffective("<dictionary> {\n  HTTPEnable : 0\n}\n", "127.0.0.1", 7890) {
		t.Error("disabled taken for ours")
	}
}
