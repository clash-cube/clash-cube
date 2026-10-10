package gui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>
#include <stdlib.h>

// frontApp is the bundle identifier of the app in front, or NULL; the
// caller frees it. With the tray menu up it is still the app the user was
// in, as the menu bar doesn't take activation.
static char *frontApp(void) {
	@autoreleasepool {
		NSString *id = NSWorkspace.sharedWorkspace.frontmostApplication.bundleIdentifier;
		return id ? strdup(id.UTF8String) : NULL;
	}
}

// hideApp gives activation back to the app that had it.
static void hideApp(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ [NSApp hide:nil]; });
}
*/
import "C"

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
	"unsafe"
)

// browsers is the browsers whose page a rule can be added for, by bundle
// identifier, with the AppleScript that reads the front page's address.
// Safari's documents have a URL; Chromium's windows have an active tab.
// Firefox has no AppleScript dictionary.
var browsers = map[string]string{
	"com.apple.Safari":                  "URL of front document",
	"com.apple.SafariTechnologyPreview": "URL of front document",
	"com.google.Chrome":                 "URL of active tab of front window",
	"com.google.Chrome.beta":            "URL of active tab of front window",
	"com.google.Chrome.dev":             "URL of active tab of front window",
	"com.google.Chrome.canary":          "URL of active tab of front window",
	"com.microsoft.edgemac":             "URL of active tab of front window",
	"com.microsoft.edgemac.Beta":        "URL of active tab of front window",
	"com.microsoft.edgemac.Dev":         "URL of active tab of front window",
	"com.brave.Browser":                 "URL of active tab of front window",
	"company.thebrowser.Browser":        "URL of active tab of front window",
	"com.vivaldi.Vivaldi":               "URL of active tab of front window",
	"org.chromium.Chromium":             "URL of active tab of front window",
}

// frontBrowser is the browser in front, or "" when the app in front isn't
// one whose page can be read.
func frontBrowser() string {
	p := C.frontApp()
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	if id := C.GoString(p); browsers[id] != "" {
		return id
	}
	return ""
}

// browserURL is the address of the page in front in a browser frontBrowser
// gave. The first time, macOS asks whether ClashCube may control it.
func browserURL(id string) (string, error) {
	property := browsers[id]
	if property == "" {
		return "", errors.New("not a supported browser")
	}
	// the user has the permission prompt to answer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", `tell application id "`+id+`" to get `+property).CombinedOutput()
	text := strings.TrimSpace(string(out))
	switch {
	case err == nil && text != "" && text != "missing value":
		return text, nil
	case strings.Contains(text, "-1743"):
		return "", errAutomation
	case err == nil, strings.Contains(text, "-1728"), strings.Contains(text, "-1719"):
		return "", errNoPage
	}
	return "", errors.New(text)
}

func hideApp() { C.hideApp() }
