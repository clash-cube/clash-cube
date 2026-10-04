package hotkeys

/*
#cgo LDFLAGS: -framework Carbon
#include <Carbon/Carbon.h>

// systemTaken says whether macOS has an enabled shortcut (Spotlight,
// screenshots, input sources, Mission Control…) on code with mods.
static int systemTaken(unsigned int code, unsigned int mods) {
	CFArrayRef list = NULL;
	if (CopySymbolicHotKeys(&list) != noErr || list == NULL) return 0;
	int taken = 0;
	for (CFIndex i = 0; i < CFArrayGetCount(list) && !taken; i++) {
		CFDictionaryRef d = CFArrayGetValueAtIndex(list, i);
		CFBooleanRef on = CFDictionaryGetValue(d, kHISymbolicHotKeyEnabled);
		CFNumberRef c = CFDictionaryGetValue(d, kHISymbolicHotKeyCode);
		CFNumberRef m = CFDictionaryGetValue(d, kHISymbolicHotKeyModifiers);
		if (!on || !CFBooleanGetValue(on) || !c || !m) continue;
		int kc = 0, km = 0;
		CFNumberGetValue(c, kCFNumberIntType, &kc);
		CFNumberGetValue(m, kCFNumberIntType, &km);
		// only the four modifiers count; the rest are flags such as fn
		taken = (unsigned int)kc == code && ((unsigned int)km & (cmdKey|shiftKey|optionKey|controlKey)) == mods;
	}
	CFRelease(list);
	return taken;
}
*/
import "C"

func systemTaken(norm string) bool {
	code, mods := carbon(norm)
	return C.systemTaken(C.uint(code), C.uint(mods)) != 0
}
