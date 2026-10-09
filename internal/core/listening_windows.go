package core

import (
	"github.com/localhost-copilot/clashcube/internal/winutil"
	"os"
	"sort"
)

func listeningPorts() []int {
	rows, _ := winutil.Listeners()
	out := []int{}
	seen := map[int]bool{}
	for _, row := range rows {
		if row.PID == uint32(os.Getpid()) && !seen[row.Port] {
			seen[row.Port] = true
			out = append(out, row.Port)
		}
	}
	sort.Ints(out)
	return out
}
