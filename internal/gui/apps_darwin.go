package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

// runningApps is the apps in the Dock and the menu bar, as lines of
// "name\tbundle\texecutable", leaving out the system's own (under
// /System, which a rule hardly names); the caller frees it.
static char *runningApps(void) {
	@autoreleasepool {
		NSMutableString *out = [NSMutableString string];
		for (NSRunningApplication *a in [[NSWorkspace sharedWorkspace] runningApplications]) {
			if (a.activationPolicy == NSApplicationActivationPolicyProhibited || !a.executableURL) continue;
			if ([a.executableURL.path hasPrefix:@"/System/"]) continue;
			NSString *name = a.localizedName ?: a.executableURL.lastPathComponent;
			NSString *bundle = a.bundleURL ? a.bundleURL.path : @"";
			[out appendFormat:@"%@\t%@\t%@\n", name, bundle, a.executableURL.path];
		}
		return strdup(out.UTF8String);
	}
}

// bundleInfo is an app bundle's name and executable as "name\texecutable",
// or NULL when path isn't one; the caller frees it.
static char *bundleInfo(const char *path) {
	@autoreleasepool {
		NSBundle *b = [NSBundle bundleWithPath:[NSString stringWithUTF8String:path]];
		if (!b || !b.executablePath) return NULL;
		NSString *name = [[NSFileManager defaultManager] displayNameAtPath:b.bundlePath];
		if ([name hasSuffix:@".app"]) name = [name stringByDeletingPathExtension];
		return strdup([NSString stringWithFormat:@"%@\t%@", name, b.executablePath].UTF8String);
	}
}
*/
import "C"

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unsafe"
)

// App is a program a process rule can name.
type App struct {
	Name       string `json:"name"`
	Bundle     string `json:"bundle"`     // its .app, "" for a bare executable
	Executable string `json:"executable"` // the binary
}

// runningApps is the apps running now, by name. A helper inside another
// app (Chrome's, Electron's) counts as that app, which is listed once.
func runningApps() []App {
	p := C.runningApps()
	defer C.free(unsafe.Pointer(p))
	var out []App
	seen := map[string]bool{}
	for _, l := range strings.Split(C.GoString(p), "\n") {
		f := strings.Split(l, "\t")
		if len(f) != 3 {
			continue
		}
		a := App{Name: f[0], Bundle: f[1], Executable: f[2]}
		if outer := outerBundle(a.Executable); outer != "" && outer != a.Bundle {
			if b, ok := appAt(outer); ok {
				a = b
			}
		}
		key := a.Bundle
		if key == "" {
			key = a.Executable
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// outerBundle is the outermost .app on a path, or "".
func outerBundle(p string) string {
	if i := strings.Index(p, ".app/"); i >= 0 {
		return p[:i+4]
	}
	return ""
}

// appAt is the program at path: an app bundle, or an executable file.
func appAt(path string) (App, bool) {
	cp := C.CString(path)
	defer C.free(unsafe.Pointer(cp))
	if p := C.bundleInfo(cp); p != nil {
		defer C.free(unsafe.Pointer(p))
		if name, exe, ok := strings.Cut(C.GoString(p), "\t"); ok {
			return App{Name: name, Bundle: path, Executable: exe}, true
		}
	}
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
		return App{Name: filepath.Base(path), Executable: path}, true
	}
	return App{}, false
}
