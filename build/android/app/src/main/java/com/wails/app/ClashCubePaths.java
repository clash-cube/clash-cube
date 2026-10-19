package com.wails.app;

/** Initializes Go storage before Wails starts the application goroutine. */
final class ClashCubePaths {
    static { System.loadLibrary("wails"); }
    static native void nativeSetFilesDir(String path);
    private ClashCubePaths() {}
}
