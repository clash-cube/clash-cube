package gui

import (
	"os/exec"
	"strings"
	"sync"
)

var sysLang = sync.OnceValue(func() bool {
	out, err := exec.Command("defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return false
	}
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.Trim(strings.TrimSpace(l), `",()`)
		if l != "" {
			return strings.HasPrefix(l, "zh")
		}
	}
	return false
})

// systemPrefersChinese says whether the first of the user's languages is Chinese.
func systemPrefersChinese() bool { return sysLang() }
