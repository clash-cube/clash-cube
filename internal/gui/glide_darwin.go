package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore
#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>

// The panel hangs from the menu bar: its top edge stays, the bottom moves,
// animated by the system on the curve the page's CSS uses.
static void glidePanel(void *w, int height, int ms, double x1, double y1, double x2, double y2) {
	NSWindow *win = (NSWindow *)w;
	dispatch_async(dispatch_get_main_queue(), ^{
		NSRect f = win.frame;
		NSRect to = NSMakeRect(f.origin.x, NSMaxY(f) - height, f.size.width, height);
		[NSAnimationContext runAnimationGroup:^(NSAnimationContext *ctx) {
			ctx.duration = ms / 1000.0;
			ctx.timingFunction = [CAMediaTimingFunction functionWithControlPoints:x1 :y1 :x2 :y2];
			ctx.allowsImplicitAnimation = YES;
			[[win animator] setFrame:to display:YES];
		} completionHandler:nil];
	});
}

// Regular puts the app in the Dock, Accessory takes it out.
static void setDock(int on, int front) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NSApplicationActivationPolicy p = on ? NSApplicationActivationPolicyRegular : NSApplicationActivationPolicyAccessory;
		if ([NSApp activationPolicy] == p) return;
		[NSApp setActivationPolicy:p];
		if (front) [NSApp activateIgnoringOtherApps:YES];
	});
}
// activate brings the app forward, as a click on its window would: an
// accessory app's window shown from a global shortcut otherwise opens
// behind the app in front.
static void activateApp(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		[NSApp activateIgnoringOtherApps:YES];
	});
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// glide moves the shown panel to height over ms on the curve; false when the
// window has no native handle yet.
func glide(w *application.WebviewWindow, height, ms int, curve [4]float64) bool {
	nw := w.NativeWindow()
	if nw == nil {
		return false
	}
	C.glidePanel(nw, C.int(height), C.int(ms), C.double(curve[0]), C.double(curve[1]), C.double(curve[2]), C.double(curve[3]))
	return true
}

func activateApp() { C.activateApp() }

func setDock(on, front bool) {
	b := func(v bool) C.int {
		if v {
			return 1
		}
		return 0
	}
	C.setDock(b(on), b(front))
}

func dockPolicy(on bool) application.ActivationPolicy {
	if on {
		return application.ActivationPolicyRegular
	}
	return application.ActivationPolicyAccessory
}
