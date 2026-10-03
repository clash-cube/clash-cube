package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include "tray_darwin.h"

void trayMenuTracking(int open);

// The menu Wails shows on a right-click (its controller's cachedMenu).
static NSMenu *mbTrayMenu(void) {
	id ctl = trayButton().target;
	if (!ctl || ![ctl respondsToSelector:NSSelectorFromString(@"cachedMenu")]) return nil;
	id m = [ctl valueForKey:@"cachedMenu"];
	return [m isKindOfClass:[NSMenu class]] ? m : nil;
}

static NSColor *mbBadgeColor(NSString *kind) {
	if ([kind isEqualToString:@"accent"]) return NSColor.controlAccentColor;
	if ([kind isEqualToString:@"good"]) return NSColor.systemGreenColor;
	if ([kind isEqualToString:@"ok"]) return NSColor.systemOrangeColor;
	if ([kind isEqualToString:@"bad"]) return NSColor.systemRedColor;
	return NSColor.tertiaryLabelColor;
}

// A badge, as Surge's mode letter and latency: white text on a colour.
static NSImage *mbBadge(NSString *text, NSString *kind, NSFont *font) {
	NSFont *f = [NSFont systemFontOfSize:round(font.pointSize * .78) weight:NSFontWeightSemibold];
	NSDictionary *at = @{NSFontAttributeName: f, NSForegroundColorAttributeName: NSColor.whiteColor};
	CGFloat tw = ceil([text sizeWithAttributes:at].width);
	CGFloat h = round(font.capHeight + 7), w = MAX(h, tw + 10);
	NSColor *color = mbBadgeColor(kind);
	return [NSImage imageWithSize:NSMakeSize(w, h) flipped:NO drawingHandler:^BOOL(NSRect r) {
		[color set];
		[[NSBezierPath bezierPathWithRoundedRect:r xRadius:4 yRadius:4] fill];
		// the cap height centred, on a whole point
		CGFloat base = round((h - f.capHeight) / 2);
		[text drawAtPoint:NSMakePoint(round((w - tw) / 2), base + f.descender) withAttributes:at];
		return YES;
	}];
}

// An app's icon at menu size; a LAN client gets the network's.
static NSImage *mbIcon(NSString *path) {
	static NSMutableDictionary *cache;
	if (!cache) cache = [[NSMutableDictionary alloc] init];
	NSImage *img = cache[path];
	if (img) return img;
	img = path.length ? [[[NSWorkspace sharedWorkspace] iconForFile:path] copy] : [[NSImage imageNamed:NSImageNameNetwork] copy];
	img.size = NSMakeSize(16, 16);
	cache[path] = img;
	[img release];
	return img;
}

// A row's layout in points, after Surge's menu: AppKit's own leaves a wide
// gap before the submenu arrows and gives shortcuts a column of their own,
// so every row is drawn by a view instead.
static const CGFloat mbRowH = 22, mbSubRowH = 42, mbSubGap = 2, mbTextX = 16, mbCheckedTextX = 24, mbCheckX = 8;
static const CGFloat mbPadR = 14, mbArrowW = 8, mbArrowGap = 5, mbIconW = 16, mbIconGap = 6;

// How wide a row's parts may be before they're cut short, and the room
// between them: a long node name mustn't make the whole menu wide.
static const CGFloat mbLeftMax = 190, mbRightMax = 140, mbGap = 20;

// mbTinted is a template image (an SF Symbol) in a colour.
static NSImage *mbTinted(NSImage *img, NSColor *color) {
	return [NSImage imageWithSize:img.size flipped:NO drawingHandler:^BOOL(NSRect r) {
		[img drawInRect:r];
		[color set];
		NSRectFillUsingOperation(r, NSCompositingOperationSourceAtop);
		return YES;
	}];
}

// The font of a row's second line (subtitled in traymenu.go).
static NSFont *mbSubFont(NSFont *font) { return [NSFont menuFontOfSize:round(font.pointSize * .85)]; }

static NSImage *mbSymbol(NSString *name, NSFont *font, CGFloat scale) {
	NSImage *img = [NSImage imageWithSystemSymbolName:name accessibilityDescription:nil];
	NSImageSymbolConfiguration *c = [NSImageSymbolConfiguration configurationWithPointSize:round(font.pointSize * scale) weight:NSFontWeightSemibold];
	return [img imageWithSymbolConfiguration:c];
}

// MBRowView draws one row: a check, an app's icon, the title, and at the
// right a detail in the secondary colour or a badge, then the submenu arrow.
// It reads the item's state, enablement and highlight as it draws.
@interface MBRowView : NSView
@property (nonatomic, copy) NSString *left, *right, *kind; // kind is a badge's, nil for text
@property (nonatomic, copy) NSString *sub; // a second line under left, or nil
@property (nonatomic, retain) NSImage *icon;
@property (nonatomic) BOOL checks; // the menu has a checked row: titles move over for the marks
@property (nonatomic) BOOL stays;  // a click leaves the menu up (stay in traymenu.go)
@end

@implementation MBRowView
- (void)dealloc {
	[_left release];
	[_right release];
	[_kind release];
	[_sub release];
	[_icon release];
	[super dealloc];
}

- (BOOL)isFlipped { return YES; }

- (void)drawRect:(NSRect)dirty {
	NSMenuItem *it = self.enclosingMenuItem;
	NSFont *font = [NSFont menuFontOfSize:0];
	NSRect b = self.bounds;
	BOOL lit = it.isHighlighted && it.isEnabled;
	if (lit) {
		[NSColor.selectedContentBackgroundColor set];
		[[NSBezierPath bezierPathWithRoundedRect:NSInsetRect(b, 5, 0) xRadius:5 yRadius:5] fill];
	}
	NSColor *fg = lit ? NSColor.selectedMenuItemTextColor : it.isEnabled ? NSColor.labelColor : NSColor.tertiaryLabelColor;
	NSColor *dim = lit ? [NSColor.selectedMenuItemTextColor colorWithAlphaComponent:.8] : NSColor.secondaryLabelColor;
	CGFloat textY = round((b.size.height - (font.ascender - font.descender)) / 2);
	NSFont *subFont = mbSubFont(font);
	if (self.sub) {
		// the two lines centred together
		CGFloat th = font.ascender - font.descender, sh = subFont.ascender - subFont.descender;
		textY = round((b.size.height - th - mbSubGap - sh) / 2);
	}

	if (it.state == NSControlStateValueOn) {
		NSImage *check = mbSymbol(@"checkmark", font, .8);
		[mbTinted(check, fg) drawInRect:NSMakeRect(mbCheckX, round((b.size.height - check.size.height) / 2), check.size.width, check.size.height) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1 respectFlipped:YES hints:nil];
	}
	CGFloat x = self.checks ? mbCheckedTextX : mbTextX;
	if (self.icon) {
		[self.icon drawInRect:NSMakeRect(x, round((b.size.height - mbIconW) / 2), mbIconW, mbIconW) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:it.isEnabled ? 1 : .5 respectFlipped:YES hints:nil];
		x += mbIconW + mbIconGap;
	}
	[self.left drawAtPoint:NSMakePoint(x, textY) withAttributes:@{NSFontAttributeName: font, NSForegroundColorAttributeName: fg}];
	if (self.sub) {
		CGFloat subY = textY + font.ascender - font.descender + mbSubGap;
		[self.sub drawAtPoint:NSMakePoint(x, subY) withAttributes:@{NSFontAttributeName: subFont, NSForegroundColorAttributeName: lit ? fg : NSColor.secondaryLabelColor}];
	}

	CGFloat r = b.size.width - mbPadR;
	if (it.hasSubmenu) {
		NSImage *arrow = mbSymbol(@"chevron.right", font, .75);
		NSSize s = arrow.size;
		[mbTinted(arrow, lit ? fg : NSColor.labelColor) drawInRect:NSMakeRect(r - s.width, round((b.size.height - s.height) / 2), s.width, s.height) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1 respectFlipped:YES hints:nil];
		r -= s.width + mbArrowGap;
	}
	if ([self.kind isEqualToString:@"space"]) {
		// room held for a delay to come
	} else if (self.kind) {
		NSImage *badge = mbBadge(self.right, self.kind, font);
		NSSize s = badge.size;
		[badge drawInRect:NSMakeRect(r - s.width, round((b.size.height - s.height) / 2), s.width, s.height) fromRect:NSZeroRect operation:NSCompositingOperationSourceOver fraction:1 respectFlipped:YES hints:nil];
	} else if (self.right.length) {
		NSDictionary *at = @{NSFontAttributeName: font, NSForegroundColorAttributeName: dim};
		[self.right drawAtPoint:NSMakePoint(r - [self.right sizeWithAttributes:at].width, textY) withAttributes:at];
	}
}

// A view's row gets no action from AppKit: the click is passed on as the
// item's own, after the menu closes, or with it still up for a row that
// stays.
- (void)mouseUp:(NSEvent *)e {
	NSMenuItem *it = self.enclosingMenuItem;
	if (!it.isEnabled || it.hasSubmenu) return;
	NSMenu *m = it.menu, *top = m;
	while (top.supermenu) top = top.supermenu;
	if (!self.stays) [top cancelTracking];
	[m performActionForItemAtIndex:[m indexOfItem:it]];
}
@end

// mbFit cuts s to fit w with an ellipsis, never through an emoji.
static NSString *mbFit(NSString *s, NSDictionary *at, CGFloat w) {
	if ([s sizeWithAttributes:at].width <= w) return s;
	NSUInteger n = s.length;
	while (n > 0) {
		n = [s rangeOfComposedCharacterSequenceAtIndex:n - 1].location;
		NSString *t = [[[s substringToIndex:n] stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceCharacterSet] stringByAppendingString:@"…"];
		if ([t sizeWithAttributes:at].width <= w) return t;
	}
	return @"…";
}

// mbStyle gives every row of a menu, and of its submenus, a view drawn from
// the item's title and its markup (see detail in traymenu.go): the part
// after a tab at the right, in the secondary colour or as a badge, and an
// icon before an app's name. All rows are as wide as the widest needs. The
// title itself is left alone, so a new label from Wails restyles cleanly.
static void mbStyle(NSMenu *menu) {
	NSFont *font = [NSFont menuFontOfSize:0];
	NSDictionary *plain = @{NSFontAttributeName: font};
	NSMutableArray *rows = [NSMutableArray array];
	BOOL checks = NO;
	CGFloat width = 0;
	for (NSMenuItem *it in menu.itemArray) {
		if (it.hasSubmenu) mbStyle(it.submenu);
		// a hidden twin that holds a row's shortcut (keyed in traymenu.go)
		if (it.isHidden) {
			if (it.keyEquivalent.length) it.allowsKeyEquivalentWhenHidden = YES;
			continue;
		}
		if (it.isSeparatorItem) continue;
		checks = checks || it.state == NSControlStateValueOn;

		NSString *raw = it.title;
		NSRange tab = [raw rangeOfString:@"\t"];
		NSString *l = tab.location == NSNotFound ? raw : [raw substringToIndex:tab.location];
		NSString *r = tab.location == NSNotFound ? nil : [raw substringFromIndex:NSMaxRange(tab)];
		MBRowView *v = [it.view isKindOfClass:[MBRowView class]] ? (MBRowView *)it.view : nil;
		if (!v) {
			v = [[[MBRowView alloc] initWithFrame:NSMakeRect(0, 0, 100, mbRowH)] autorelease];
			v.autoresizingMask = NSViewWidthSizable;
			it.view = v;
		}
		v.toolTip = it.toolTip;
		v.stays = [l hasPrefix:@"\x05"];
		if (v.stays) l = [l substringFromIndex:1];

		CGFloat need = mbPadR;
		v.icon = nil;
		if ([l hasPrefix:@"\x03"]) {
			NSRange end = [l rangeOfString:@"\x04"];
			if (end.location != NSNotFound) {
				v.icon = mbIcon([l substringWithRange:NSMakeRange(1, end.location - 1)]);
				l = [l substringFromIndex:NSMaxRange(end)];
				need += mbIconW + mbIconGap;
			}
		}
		NSRange sub = [l rangeOfString:@"\x06"];
		v.sub = sub.location == NSNotFound ? nil : [l substringFromIndex:NSMaxRange(sub)];
		if (v.sub) l = [l substringToIndex:sub.location];
		v.left = mbFit(l, plain, mbLeftMax);
		// a second line is a sentence: it sets the width, uncut
		CGFloat lw = [v.left sizeWithAttributes:plain].width;
		if (v.sub) lw = MAX(lw, [v.sub sizeWithAttributes:@{NSFontAttributeName: mbSubFont(font)}].width);
		need += ceil(lw);

		v.kind = nil;
		v.right = nil;
		if ([r hasPrefix:@"\x01"]) {
			NSRange mid = [r rangeOfString:@"\x02"];
			if (mid.location != NSNotFound) {
				v.kind = [r substringWithRange:NSMakeRange(1, mid.location - 1)];
				v.right = [r substringFromIndex:NSMaxRange(mid)];
				CGFloat w = mbBadge(v.right, v.kind, font).size.width;
				// a latency keeps the room of a slow one, as speeds do
				if (![v.kind isEqualToString:@"accent"]) w = MAX(w, mbBadge(@"999 ms", @"none", font).size.width);
				need += mbGap + w;
			}
		} else if (r.length) {
			v.right = mbFit(r, plain, mbRightMax);
			CGFloat w = [v.right sizeWithAttributes:plain].width;
			// a speed keeps the room of a wide one, so the menu holds still
			if ([v.right hasSuffix:@"/s"]) w = MAX(w, [@"999 KB/s" sizeWithAttributes:plain].width);
			need += mbGap + ceil(w);
		}
		if (it.hasSubmenu) need += (v.right ? mbArrowGap : mbGap) + mbArrowW;
		width = MAX(width, need);
		[rows addObject:v];
	}
	CGFloat x = checks ? mbCheckedTextX : mbTextX;
	for (MBRowView *v in rows) {
		v.checks = checks;
		v.frame = NSMakeRect(0, 0, ceil(x + width), v.sub ? mbSubRowH : mbRowH);
		[v setNeedsDisplay:YES];
	}
}

static void styleTrayMenu(void) {
	@autoreleasepool {
		NSMenu *m = mbTrayMenu();
		if (m) mbStyle(m);
	}
}

// watchTrayMenu reports the tray menu opening and closing, so what it shows
// can move while it's up.
static void watchTrayMenu(void) {
	static BOOL done;
	if (done) return;
	done = YES;
	NSNotificationCenter *nc = NSNotificationCenter.defaultCenter;
	[nc addObserverForName:NSMenuDidBeginTrackingNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		if (n.object && n.object == mbTrayMenu()) trayMenuTracking(1);
	}];
	[nc addObserverForName:NSMenuDidEndTrackingNotification object:nil queue:nil usingBlock:^(NSNotification *n) {
		if (n.object && n.object == mbTrayMenu()) trayMenuTracking(0);
	}];
}
*/
import "C"

// styleTrayMenu draws the tray menu's markup; on the main thread.
func styleTrayMenu() { C.styleTrayMenu() }

// watchTrayMenu has trayMenuTracking called as the menu opens and closes;
// on the main thread.
func watchTrayMenu() { C.watchTrayMenu() }
