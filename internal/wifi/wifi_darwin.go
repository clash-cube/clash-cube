package wifi

/*
#cgo CFLAGS: -x objective-c -fblocks
#cgo LDFLAGS: -framework Foundation -framework CoreWLAN -framework CoreLocation
#import <Foundation/Foundation.h>
#import <CoreWLAN/CoreWLAN.h>
#import <CoreLocation/CoreLocation.h>
#include <stdlib.h>
#include <stdatomic.h>

// CoreLocation owns a main-run-loop object. Publish just its authorization to
// background readers, without synchronously waiting on the UI thread (which
// may itself be waiting for a backend operation during shutdown).
static atomic_int wifiAuthorization = kCLAuthorizationStatusNotDetermined;
@interface MBWiFiLocationDelegate : NSObject <CLLocationManagerDelegate>
@end
@implementation MBWiFiLocationDelegate
- (void)locationManagerDidChangeAuthorization:(CLLocationManager *)manager {
	atomic_store(&wifiAuthorization, manager.authorizationStatus);
}
@end

static CLLocationManager *wifiLocationManager;
static void prepareWiFiPermission(void) {
	// Main thread only. No location tracking or coordinate collection.
	static MBWiFiLocationDelegate *delegate;
	if (!wifiLocationManager) {
		delegate = [MBWiFiLocationDelegate new];
		wifiLocationManager = [CLLocationManager new];
		wifiLocationManager.delegate = delegate;
	}
	atomic_store(&wifiAuthorization, wifiLocationManager.authorizationStatus);
}

static char *wifiSSID(int *state, char **ifname) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{ dispatch_async(dispatch_get_main_queue(), ^{ prepareWiFiPermission(); }); });
	@autoreleasepool {
		CWInterface *iface = [[CWWiFiClient sharedWiFiClient] interface];
		*ifname = iface.interfaceName.length ? strdup(iface.interfaceName.UTF8String) : NULL;
		if (!iface || !iface.powerOn) { *state = 1; return NULL; }
		NSString *ssid = iface.ssid;
		if (ssid.length) { *state = 0; return strdup(ssid.UTF8String); }
		CLAuthorizationStatus auth = atomic_load(&wifiAuthorization);
		if (auth == kCLAuthorizationStatusDenied || auth == kCLAuthorizationStatusRestricted || ![CLLocationManager locationServicesEnabled]) *state = 3;
		else if (auth == kCLAuthorizationStatusNotDetermined) *state = 2;
		else if (iface.interfaceMode == kCWInterfaceModeNone) *state = 1;
		else *state = 4;
		return NULL;
	}
}

static void requestWiFiPermission(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		prepareWiFiPermission();
		[wifiLocationManager requestWhenInUseAuthorization];
	});
}

static char *savedWiFiNames(void) {
	@autoreleasepool {
		NSArray<CWInterface *> *interfaces = [[CWWiFiClient sharedWiFiClient] interfaces];
		NSMutableOrderedSet<NSString *> *names = [NSMutableOrderedSet orderedSet];
		BOOL readable = interfaces.count == 0;
		for (CWInterface *iface in interfaces) {
			CWConfiguration *config = iface.configuration;
			if (!config) continue;
			readable = YES;
			for (CWNetworkProfile *profile in config.networkProfiles) {
				if (profile.ssid.length) [names addObject:profile.ssid];
			}
		}
		if (!readable) return NULL;
		NSData *json = [NSJSONSerialization dataWithJSONObject:names.array options:0 error:nil];
		if (!json) return NULL;
		return strndup(json.bytes, json.length);
	}
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"unsafe"
)

// Current reads the SSID; it never prompts for permission.
func Current() Status {
	var state C.int
	var iface *C.char
	name := C.wifiSSID(&state, &iface)
	defer C.free(unsafe.Pointer(name))
	defer C.free(unsafe.Pointer(iface))
	states := [...]string{"connected", "disconnected", "permission", "denied", "unavailable"}
	return Status{SSID: C.GoString(name), State: states[int(state)], Interface: C.GoString(iface)}
}

// RequestPermission asks macOS to allow SSID access after an explicit UI action.
func RequestPermission() { C.requestWiFiPermission() }

// SavedNetworks reads remembered SSIDs in system preference order, deduplicated
// across interfaces/security variants. It never scans or accesses passwords.
func SavedNetworks() ([]string, error) {
	data := C.savedWiFiNames()
	if data == nil {
		return nil, errors.New("Couldn't read saved Wi-Fi networks")
	}
	defer C.free(unsafe.Pointer(data))
	var names []string
	if err := json.Unmarshal([]byte(C.GoString(data)), &names); err != nil {
		return nil, errors.New("Couldn't read saved Wi-Fi networks")
	}
	return names, nil
}
