package backend

import (
	"os/exec"
	"strconv"
	"strings"
)

// portHolder names the program listening on port, "" when lsof can't see
// it (another user's, such as root's).
func portHolder(port int) string {
	out, _ := exec.Command("/usr/sbin/lsof", "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-Fc").Output()
	for _, line := range strings.Split(string(out), "\n") {
		if name, ok := strings.CutPrefix(line, "c"); ok && name != "" {
			return name
		}
	}
	return ""
}
