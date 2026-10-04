package core

import (
	"sync"

	"github.com/metacubex/chi"
	"github.com/metacubex/chi/render"
	"github.com/metacubex/http"
	"github.com/metacubex/mihomo/hub/route"
	"github.com/metacubex/mihomo/tunnel/statistic"
)

// maxClosed bounds the closed connections kept for the GUI; with nobody
// reading them (the GUI gone), the oldest go.
const maxClosed = 4096

// closed is the connections that ended since the GUI last asked, with
// their final byte counts: /connections lists only the open ones, so one
// that opens and closes between two of its samples would be missed by
// the traffic statistics.
var closed struct {
	sync.Mutex
	list []*statistic.TrackerInfo
}

func init() {
	statistic.DefaultManager.OnLeave(func(info *statistic.TrackerInfo) {
		closed.Lock()
		if len(closed.list) >= maxClosed {
			closed.list = append(closed.list[:0], closed.list[len(closed.list)-maxClosed/2:]...)
		}
		closed.list = append(closed.list, info)
		closed.Unlock()
	})
	// behind the controller's secret, like every other route
	route.Register(func(r chi.Router) {
		r.Get("/clashferry/closed", func(w http.ResponseWriter, r *http.Request) {
			closed.Lock()
			list := closed.list
			closed.list = nil
			closed.Unlock()
			if list == nil {
				list = []*statistic.TrackerInfo{}
			}
			render.JSON(w, r, map[string]any{"connections": list})
		})
	})
}
