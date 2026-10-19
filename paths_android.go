//go:build android && cgo

package main

/*
#include <jni.h>
static const char* filesPath(JNIEnv *env, jstring path) {
 return (*env)->GetStringUTFChars(env, path, 0);
}
static void releasePath(JNIEnv *env, jstring path, const char *text) {
 (*env)->ReleaseStringUTFChars(env, path, text);
}
*/
import "C"

import "os"

//export Java_com_wails_app_ClashCubePaths_nativeSetFilesDir
func Java_com_wails_app_ClashCubePaths_nativeSetFilesDir(env *C.JNIEnv, clazz C.jclass, path C.jstring) {
	text := C.filesPath(env, path)
	if text == nil {
		return
	}
	defer C.releasePath(env, path, text)
	// Java Os.setenv alone is too late once Go has cached the environment.
	_ = os.Setenv("CLASHCUBE_HOME", C.GoString(text))
}
