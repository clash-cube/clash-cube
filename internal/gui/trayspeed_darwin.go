package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

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

// The icon and, beside it, two lines, up over down, drawn into one template
// image: AppKit's gap between a button's image and title is too wide, and a
// template follows the menu bar's colour. The text is right-aligned to a
// fixed width so the menu bar doesn't shift as the numbers change.
static void setTraySpeed(const void *icon, int n, char *up, char *down) {
	NSData *data = [[NSData alloc] initWithBytes:icon length:n];
	dispatch_async(dispatch_get_main_queue(), ^{
		@autoreleasepool {
			NSButton *b = trayButton();
			NSImage *ic = [[[NSImage alloc] initWithData:data] autorelease];
			if (b && ic) {
				CGFloat t = [[NSStatusBar systemStatusBar] thickness];
				if (!up) {
					ic.size = NSMakeSize(t, t);
					ic.template = YES;
					b.image = ic;
				} else {
					NSFont *font = [NSFont monospacedDigitSystemFontOfSize:8.5 weight:NSFontWeightSemibold];
					NSDictionary *a = @{NSFontAttributeName: font, NSForegroundColorAttributeName: NSColor.blackColor};
					static CGFloat width;
					if (width == 0) {
						for (NSString *s in @[@"999 B/s", @"999 KB/s", @"999 MB/s", @"999 GB/s"]) {
							width = MAX(width, [s sizeWithAttributes:a].width);
						}
						width = ceil(width);
					}
					NSString *u = [NSString stringWithUTF8String:up], *d = [NSString stringWithUTF8String:down];
					// text on whole pixels, or its strokes smear across two
					CGFloat px = b.window.backingScaleFactor ?: 2;
					CGFloat (^snap)(CGFloat) = ^CGFloat(CGFloat v) { return round(v * px) / px; };
					// the icon's glyph stops at 19.5 of 22 (build/icon/tray.svg)
					CGFloat x = t * 19.5 / 22 + 3, line = 9;
					// baselines that centre the two lines' cap height on the icon
					CGFloat base = snap((t + font.capHeight - line) / 2);
					NSImage *img = [NSImage imageWithSize:NSMakeSize(ceil(x + width), t) flipped:YES drawingHandler:^BOOL(NSRect r) {
						[ic drawInRect:NSMakeRect(0, 0, t, t) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1 respectFlipped:YES hints:nil];
						NSArray *lines = @[u, d];
						for (NSUInteger i = 0; i < lines.count; i++) {
							NSString *s = lines[i];
							CGFloat w = [s sizeWithAttributes:a].width;
							// drawAtPoint takes the line's top: the baseline less the ascender
							[s drawAtPoint:NSMakePoint(snap(x + width - w), base + i * line - font.ascender) withAttributes:a];
						}
						return YES;
					}];
					img.template = YES;
					b.image = img;
				}
				b.title = @"";
				b.imagePosition = NSImageOnly;
			}
		}
		[data release];
		free(up);
		free(down);
	});
}
*/
import "C"

import "unsafe"

// setTraySpeed shows icon in the tray, with up and down beside it; empty
// strings show the icon alone.
func setTraySpeed(icon []byte, up, down string) {
	p, n := unsafe.Pointer(&icon[0]), C.int(len(icon))
	if up == "" && down == "" {
		C.setTraySpeed(p, n, nil, nil)
		return
	}
	C.setTraySpeed(p, n, C.CString(up), C.CString(down))
}
