package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore
#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#include "tray_darwin.h"

// The panel takes the shape of the menu bar's popovers: a rounded body and
// an arrow above it, pointing at the tray icon. The shape masks the content
// view, so the backdrop, the page and the window's shadow all follow it.
// The page leaves the arrow's height free at its top (panel.css).
static const CGFloat arrowHeight = 10, arrowHalf = 11, bodyRadius = 12;

@interface ClashCubePanelShape : NSObject
@property (assign) NSWindow *win;
@property (retain) CALayer *mask;
@property (retain) CALayer *body;
@property (retain) CAShapeLayer *arrow;
@end

@implementation ClashCubePanelShape
// tipX is the tray icon's middle, from the panel's left edge, kept off the
// body's rounded corners.
- (CGFloat)tipX {
	NSRect f = self.win.frame;
	CGFloat x = f.size.width / 2;
	NSButton *b = trayButton();
	if (b.window) {
		x = NSMidX([b.window convertRectToScreen:[b convertRect:b.bounds toView:nil]]) - f.origin.x;
	}
	CGFloat edge = bodyRadius + arrowHalf;
	return MIN(MAX(x, edge), f.size.width - edge);
}

- (void)update:(NSNotification *)n {
	NSRect b = self.win.contentView.bounds;
	CGFloat top = b.size.height - arrowHeight, tip = [self tipX];
	CGMutablePathRef p = CGPathCreateMutable();
	CGPathMoveToPoint(p, NULL, tip - arrowHalf, top - 1); // into the body: no seam
	CGPathAddLineToPoint(p, NULL, tip - arrowHalf, top);
	CGPathAddQuadCurveToPoint(p, NULL, tip - 3, top + 2, tip, b.size.height);
	CGPathAddQuadCurveToPoint(p, NULL, tip + 3, top + 2, tip + arrowHalf, top);
	CGPathAddLineToPoint(p, NULL, tip + arrowHalf, top - 1);
	CGPathCloseSubpath(p);
	[CATransaction begin];
	[CATransaction setDisableActions:YES];
	self.mask.frame = b;
	self.body.frame = NSMakeRect(0, 0, b.size.width, top);
	self.arrow.path = p;
	[CATransaction commit];
	CGPathRelease(p);
	[self.win invalidateShadow];
}
@end

static ClashCubePanelShape *panelShape;

static void shapePanel(void *w) {
	NSWindow *win = (NSWindow *)w;
	dispatch_async(dispatch_get_main_queue(), ^{
		if (panelShape) return;
		NSView *v = win.contentView;
		v.wantsLayer = YES;
		ClashCubePanelShape *s = [ClashCubePanelShape new];
		s.win = win;
		s.mask = [CALayer layer];
		s.body = [CALayer layer];
		s.body.backgroundColor = CGColorGetConstantColor(kCGColorBlack);
		s.body.cornerRadius = bodyRadius;
		if (@available(macOS 11.0, *)) s.body.cornerCurve = kCACornerCurveContinuous;
		s.arrow = [CAShapeLayer layer];
		s.arrow.fillColor = CGColorGetConstantColor(kCGColorBlack);
		[s.mask addSublayer:s.body];
		[s.mask addSublayer:s.arrow];
		v.layer.mask = s.mask;
		v.postsFrameChangedNotifications = YES;
		NSNotificationCenter *nc = [NSNotificationCenter defaultCenter];
		[nc addObserver:s selector:@selector(update:) name:NSViewFrameDidChangeNotification object:v];
		[nc addObserver:s selector:@selector(update:) name:NSWindowDidMoveNotification object:win];
		[s update:nil];
		panelShape = s;
	});
}

// panelOpening hides the panel before it shows, so it opens from nothing.
static void panelOpening(void *w) {
	void (^hide)(void) = ^{ ((NSWindow *)w).alphaValue = 0; };
	if ([NSThread isMainThread]) hide();
	else dispatch_sync(dispatch_get_main_queue(), hide);
}

// popPanel moves the panel between its arrow, at 90%, and its full size:
// the system's popovers open and close so. A close holds the end, until
// panelClosed, after the window is hidden.
static void popPanel(NSWindow *win, BOOL open, double secs, CAMediaTimingFunction *curve) {
	if (!panelShape || NSWorkspace.sharedWorkspace.accessibilityDisplayShouldReduceMotion) return;
	[panelShape update:nil];
	CALayer *l = win.contentView.layer;
	CGRect b = l.bounds;
	CGFloat px = [panelShape tipX] - l.anchorPoint.x * b.size.width;
	CGFloat py = b.size.height - l.anchorPoint.y * b.size.height;
	CATransform3D small = CATransform3DMakeTranslation(px, py, 0);
	small = CATransform3DScale(small, .9, .9, 1);
	small = CATransform3DTranslate(small, -px, -py, 0);
	NSValue *from = [NSValue valueWithCATransform3D:small], *to = [NSValue valueWithCATransform3D:CATransform3DIdentity];
	CABasicAnimation *a = [CABasicAnimation animationWithKeyPath:@"transform"];
	a.fromValue = open ? from : to;
	a.toValue = open ? to : from;
	a.duration = secs;
	a.timingFunction = curve;
	if (!open) {
		a.removedOnCompletion = NO;
		a.fillMode = kCAFillModeForwards;
	}
	[l addAnimation:a forKey:@"pop"];
}

static void fadePanel(NSWindow *win, CGFloat alpha, double secs, CAMediaTimingFunction *curve) {
	[NSAnimationContext runAnimationGroup:^(NSAnimationContext *ctx) {
		ctx.duration = secs;
		ctx.timingFunction = curve;
		win.animator.alphaValue = alpha;
	} completionHandler:nil];
}

static void panelOpened(void *w, double x1, double y1, double x2, double y2) {
	NSWindow *win = (NSWindow *)w;
	dispatch_async(dispatch_get_main_queue(), ^{
		CAMediaTimingFunction *curve = [CAMediaTimingFunction functionWithControlPoints:x1 :y1 :x2 :y2];
		popPanel(win, YES, .26, curve);
		fadePanel(win, 1, .16, curve);
	});
}

static void panelClosing(void *w, double secs) {
	NSWindow *win = (NSWindow *)w;
	dispatch_async(dispatch_get_main_queue(), ^{
		CAMediaTimingFunction *curve = [CAMediaTimingFunction functionWithName:kCAMediaTimingFunctionEaseIn];
		popPanel(win, NO, secs, curve);
		fadePanel(win, 0, secs, curve);
	});
}

static void panelClosed(void *w) {
	NSWindow *win = (NSWindow *)w;
	dispatch_async(dispatch_get_main_queue(), ^{
		[win.contentView.layer removeAnimationForKey:@"pop"];
		win.alphaValue = 1;
	});
}
*/
import "C"

import (
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// The arrow reaches up to the menu bar.
const panelOffset = 0

const panelCloseTime = 150 * time.Millisecond

func shapePanel(w *application.WebviewWindow) {
	if nw := w.NativeWindow(); nw != nil {
		C.shapePanel(nw)
	}
}

func panelOpening(w *application.WebviewWindow) {
	if nw := w.NativeWindow(); nw != nil {
		C.panelOpening(nw)
	}
}

func panelOpened(w *application.WebviewWindow) {
	if nw := w.NativeWindow(); nw != nil {
		c := glideCurve
		C.panelOpened(nw, C.double(c[0]), C.double(c[1]), C.double(c[2]), C.double(c[3]))
	}
}

func panelClosing(w *application.WebviewWindow) {
	if nw := w.NativeWindow(); nw != nil {
		C.panelClosing(nw, C.double(panelCloseTime.Seconds()))
	}
}

func panelClosed(w *application.WebviewWindow) {
	if nw := w.NativeWindow(); nw != nil {
		C.panelClosed(nw)
	}
}
