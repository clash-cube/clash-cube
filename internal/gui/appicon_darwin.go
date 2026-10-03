package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

// appIconPNG draws a file's icon (the network's for no path) into a px-square
// PNG; the caller frees it.
static void *appIconPNG(const char *path, int px, int *len) {
	@autoreleasepool {
		NSString *p = path[0] ? [NSString stringWithUTF8String:path] : nil;
		NSImage *img = p ? [[NSWorkspace sharedWorkspace] iconForFile:p] : [NSImage imageNamed:NSImageNameNetwork];
		NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL pixelsWide:px pixelsHigh:px
			bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO colorSpaceName:NSCalibratedRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
		[NSGraphicsContext saveGraphicsState];
		NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
		[img drawInRect:NSMakeRect(0, 0, px, px) fromRect:NSZeroRect operation:NSCompositingOperationCopy fraction:1];
		[NSGraphicsContext restoreGraphicsState];
		NSData *d = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
		[rep release];
		*len = (int)d.length;
		void *buf = malloc(d.length);
		memcpy(buf, d.bytes, d.length);
		return buf;
	}
}
*/
import "C"

import (
	"encoding/base64"
	"sync"
	"unsafe"
)

var appIcons sync.Map // path → data URL

// appIcon is a file's icon as a PNG data URL, 32 pixels square (16 points
// on a Retina screen).
func appIcon(path string) string {
	if v, ok := appIcons.Load(path); ok {
		return v.(string)
	}
	p := C.CString(path)
	defer C.free(unsafe.Pointer(p))
	var n C.int
	buf := C.appIconPNG(p, 32, &n)
	defer C.free(buf)
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(C.GoBytes(buf, n))
	appIcons.Store(path, url)
	return url
}
