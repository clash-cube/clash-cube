// The tray's status item, as Wails made it; shared by the cgo preambles
// that draw its icon (trayspeed_darwin.go) and style its menu
// (traymenu_darwin.go).
#import <Cocoa/Cocoa.h>

// The tray's button, the one Wails made (its target is Wails' controller).
static NSButton *findTrayButton(NSView *v, Class ctl) {
	if ([v isKindOfClass:[NSStatusBarButton class]] && [[(NSButton *)v target] isKindOfClass:ctl]) {
		return (NSButton *)v;
	}
	for (NSView *s in v.subviews) {
		NSButton *b = findTrayButton(s, ctl);
		if (b) return b;
	}
	return nil;
}

static NSButton *trayButton(void) {
	static NSButton *found;
	if (found && found.window) return found;
	found = nil;
	Class ctl = NSClassFromString(@"StatusItemController");
	if (!ctl) return nil;
	for (NSWindow *w in NSApp.windows) {
		NSView *root = w.contentView.superview ?: w.contentView;
		if ((found = findTrayButton(root, ctl))) break;
	}
	return found;
}
