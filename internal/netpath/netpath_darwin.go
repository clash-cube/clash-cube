package netpath

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework Foundation -framework Network
#import <Foundation/Foundation.h>
#import <Network/Network.h>
#include <stdatomic.h>

// bit 0 expensive, bit 1 constrained
static atomic_int pathFlags = 0;

static int currentPath(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		nw_path_monitor_t m = nw_path_monitor_create();
		// TUN's utun is "other": look at the physical network under it
		nw_path_monitor_prohibit_interface_type(m, nw_interface_type_other);
		nw_path_monitor_set_queue(m, dispatch_get_global_queue(QOS_CLASS_UTILITY, 0));
		nw_path_monitor_set_update_handler(m, ^(nw_path_t path) {
			int f = 0;
			if (nw_path_is_expensive(path)) f |= 1;
			if (nw_path_is_constrained(path)) f |= 2;
			atomic_store(&pathFlags, f);
		});
		nw_path_monitor_start(m);
	});
	return atomic_load(&pathFlags);
}
*/
import "C"

// Current is the path as last reported; the first call starts the monitor.
func Current() Path {
	f := int(C.currentPath())
	return Path{Expensive: f&1 != 0, Constrained: f&2 != 0}
}
