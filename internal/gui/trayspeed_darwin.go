package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>
#include "tray_darwin.h"

// What the item shows: the cube, filled while traffic is taken over, the
// mode's letter on it, and the speed.
static BOOL mbOn;
static NSString *mbUp, *mbDown, *mbLetter;

// The cube's three faces in build/icon/tray.svg's units: top, right, left.
static const CGFloat mbFace[3][8] = {
	{11.25, 2.2, 18.75, 6.55, 11.25, 10.9, 3.75, 6.55},
	{11.25, 10.9, 18.75, 6.55, 18.75, 15.45, 11.25, 19.8},
	{3.75, 6.55, 11.25, 10.9, 11.25, 19.8, 3.75, 15.45},
};

// The click's play: a light goes once round the cube, clockwise, left, top,
// right, each face lighting fast and fading slow behind it, so one face
// leads and the cube's edges stay readable. It ends on the right face,
// which stays lit while on: the light comes home.
static const CGFloat mbStart[3] = {.11, .22, 0}, mbRise = .1, mbFade = .25;
static const CFTimeInterval mbPlayLength = .22 + .1 + .25;
static CFTimeInterval mbPlayAt;
static NSTimer *mbTimer;

static CGFloat mbEase(CGFloat u) { return 1 - pow(1 - MIN(MAX(u, 0), 1), 4); }

// mbFill is how full each face is now; at rest, the right face while on.
static void mbFill(CGFloat v[3]) {
	CGFloat p = mbPlayAt ? CFAbsoluteTimeGetCurrent() - mbPlayAt : mbPlayLength;
	for (int i = 0; i < 3; i++) {
		BOOL home = i == 1 && mbOn;
		CGFloat a = p - mbStart[i];
		CGFloat lit = a < 0 ? 0 : a < mbRise ? mbEase(a / mbRise) : home ? 1 : 1 - mbEase((a - mbRise) / mbFade);
		// the light leaving home, as it sets out
		CGFloat leaving = home ? 1 - mbEase(p / mbFade) : 0;
		v[i] = MAX(lit, leaving);
	}
}

// mbImage draws it all into one template image, t high at px pixels a
// point: AppKit's gap between a button's image and title is too wide, and a
// template follows the menu bar's colour. The speed is right-aligned to a
// fixed width so the menu bar doesn't shift as the numbers change.
static NSImage *mbImage(CGFloat t, CGFloat px, const CGFloat fill[3], BOOL on, NSString *u, NSString *d, NSString *l) {
	CGFloat v0 = fill[0], v1 = fill[1], v2 = fill[2]; // a block can't take an array
	CGFloat k = t / 22;
	if (!l.length) l = nil;
	NSFont *font = [NSFont monospacedDigitSystemFontOfSize:8.5 weight:NSFontWeightSemibold];
	NSDictionary *a = @{NSFontAttributeName: font, NSForegroundColorAttributeName: NSColor.blackColor};
	static CGFloat width;
	if (width == 0) {
		for (NSString *s in @[@"999 B/s", @"999 KB/s", @"999 MB/s", @"999 GB/s"]) {
			width = MAX(width, [s sizeWithAttributes:a].width);
		}
		width = ceil(width);
	}
	// text on whole pixels, or its strokes smear across two
	CGFloat (^snap)(CGFloat) = ^CGFloat(CGFloat v) { return round(v * px) / px; };
	// the icon's glyph stops at 19.5 of 22 (build/icon/tray.svg), the
	// letter's badge at 21.5
	CGFloat x = t * (l ? 21.5 : 19.5) / 22 + (l ? 2 : 3), line = 9;
	// baselines that centre the two lines' cap height on the icon
	CGFloat base = snap((t + font.capHeight - line) / 2);
	NSImage *img = [NSImage imageWithSize:NSMakeSize(u ? ceil(x + width) : t, t) flipped:YES drawingHandler:^BOOL(NSRect r) {
		CGContextRef cg = NSGraphicsContext.currentContext.CGContext;
		// the cube as tray.svg draws it, in one layer so that a dimmed
		// (off) cube's fills and strokes don't darken where they overlap
		CGContextSetAlpha(cg, on ? 1 : .55);
		CGContextBeginTransparencyLayer(cg, NULL);
		CGFloat v[3] = {v0, v1, v2};
		for (int i = 0; i < 3; i++) {
			if (v[i] <= 0) continue;
			const CGFloat *f = mbFace[i];
			NSBezierPath *face = [NSBezierPath bezierPath];
			[face moveToPoint:NSMakePoint(f[0] * k, f[1] * k)];
			for (int j = 2; j < 8; j += 2) [face lineToPoint:NSMakePoint(f[j] * k, f[j + 1] * k)];
			[face closePath];
			[[NSColor colorWithWhite:0 alpha:MIN(v[i], 1)] set];
			[face fill];
		}
		NSBezierPath *edge = [NSBezierPath bezierPath];
		const CGFloat *o = (const CGFloat[]){11.25, 2.2, 18.75, 6.55, 18.75, 15.45, 11.25, 19.8, 3.75, 15.45, 3.75, 6.55};
		[edge moveToPoint:NSMakePoint(o[0] * k, o[1] * k)];
		for (int j = 2; j < 12; j += 2) [edge lineToPoint:NSMakePoint(o[j] * k, o[j + 1] * k)];
		[edge closePath];
		[edge moveToPoint:NSMakePoint(3.75 * k, 6.55 * k)];
		[edge lineToPoint:NSMakePoint(11.25 * k, 10.9 * k)];
		[edge lineToPoint:NSMakePoint(18.75 * k, 6.55 * k)];
		[edge moveToPoint:NSMakePoint(11.25 * k, 10.9 * k)];
		[edge lineToPoint:NSMakePoint(11.25 * k, 19.8 * k)];
		edge.lineWidth = 1.5 * k;
		edge.lineJoinStyle = NSLineJoinStyleRound;
		[NSColor.blackColor set];
		[edge stroke];
		CGContextEndTransparencyLayer(cg);
		CGContextSetAlpha(cg, 1);
		if (l) {
			// a badge at the bottom right, a gap cut round it, the letter cut out
			NSRect box = NSMakeRect(snap(13 * k), snap(12.75 * k), snap(8.5 * k), snap(8.5 * k));
			CGContextSetBlendMode(cg, kCGBlendModeClear);
			[[NSBezierPath bezierPathWithRoundedRect:NSInsetRect(box, -1.25 * k, -1.25 * k) xRadius:3.25 * k yRadius:3.25 * k] fill];
			CGContextSetBlendMode(cg, kCGBlendModeNormal);
			[NSColor.blackColor set];
			[[NSBezierPath bezierPathWithRoundedRect:box xRadius:2.25 * k yRadius:2.25 * k] fill];
			NSFont *lf = [NSFont systemFontOfSize:7 * k weight:NSFontWeightBold];
			NSDictionary *la = @{NSFontAttributeName: lf, NSForegroundColorAttributeName: NSColor.blackColor};
			CGFloat lw = [l sizeWithAttributes:la].width;
			CGContextSetBlendMode(cg, kCGBlendModeDestinationOut);
			[l drawAtPoint:NSMakePoint(NSMidX(box) - lw / 2, snap(NSMidY(box) + lf.capHeight / 2) - lf.ascender) withAttributes:la];
			CGContextSetBlendMode(cg, kCGBlendModeNormal);
		}
		NSArray *lines = u ? @[u, d] : @[];
		for (NSUInteger i = 0; i < lines.count; i++) {
			NSString *s = lines[i];
			CGFloat w = [s sizeWithAttributes:a].width;
			// drawAtPoint takes the line's top: the baseline less the ascender
			[s drawAtPoint:NSMakePoint(snap(x + width - w), base + i * line - font.ascender) withAttributes:a];
		}
		return YES;
	}];
	img.template = YES;
	return img;
}

static void mbApply(void) {
	NSButton *b = trayButton();
	if (!b) return;
	CGFloat v[3];
	mbFill(v);
	b.image = mbImage([[NSStatusBar systemStatusBar] thickness], b.window.backingScaleFactor ?: 2, v, mbOn, mbUp, mbDown, mbLetter);
	b.title = @"";
	b.imagePosition = NSImageOnly;
}

static NSString *mbString(char *s) {
	if (!s) return nil;
	NSString *v = [[NSString alloc] initWithUTF8String:s];
	free(s);
	return v;
}

static void setTray(int on, char *up, char *down, char *letter) {
	dispatch_async(dispatch_get_main_queue(), ^{
		@autoreleasepool {
			mbOn = on != 0;
			[mbUp release];
			[mbDown release];
			[mbLetter release];
			mbUp = mbString(up), mbDown = mbString(down), mbLetter = mbString(letter);
			mbApply();
		}
	});
}

// trayPlay lights the faces in turn; a click during it doesn't start over.
static void trayPlay(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (mbTimer || NSWorkspace.sharedWorkspace.accessibilityDisplayShouldReduceMotion) return;
		mbPlayAt = CFAbsoluteTimeGetCurrent();
		mbTimer = [NSTimer timerWithTimeInterval:1.0 / 60 repeats:YES block:^(NSTimer *tm) {
			if (CFAbsoluteTimeGetCurrent() - mbPlayAt >= mbPlayLength) {
				mbPlayAt = 0;
				[tm invalidate];
				mbTimer = nil;
			}
			@autoreleasepool { mbApply(); }
		}];
		[[NSRunLoop mainRunLoop] addTimer:mbTimer forMode:NSRunLoopCommonModes];
	});
}

*/
import "C"

func cString(s string) *C.char {
	if s == "" {
		return nil
	}
	return C.CString(s)
}

// setTray shows the cube in the tray, filled when on, with the mode's
// letter on it and up and down beside it; empty strings leave them out.
func setTray(on bool, up, down, letter string) {
	o := C.int(0)
	if on {
		o = 1
	}
	if up == "" || down == "" {
		up, down = "", ""
	}
	C.setTray(o, cString(up), cString(down), cString(letter))
}

// trayPlay lights the cube's faces one after another, on a click.
func trayPlay() { C.trayPlay() }
