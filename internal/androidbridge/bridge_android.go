package androidbridge

/*
#include <jni.h>
#include <stdlib.h>
#include <string.h>
static JavaVM *vm;
static jobject bridge;
static jmethodID callMethod;
static void initBridge(JNIEnv *env, jobject obj) {
 (*env)->GetJavaVM(env, &vm);
 bridge = (*env)->NewGlobalRef(env, obj);
 jclass cls = (*env)->GetObjectClass(env, obj);
 callMethod = (*env)->GetMethodID(env, cls, "call", "(Ljava/lang/String;)Ljava/lang/String;");
 (*env)->DeleteLocalRef(env, cls);
}
static char* bridgeCall(const char *input) {
 if (!vm || !bridge) return strdup("{\"error\":\"Android bridge not initialized\"}");
 JNIEnv *env; int attached = 0;
 if ((*vm)->GetEnv(vm, (void**)&env, JNI_VERSION_1_6) != JNI_OK) {
  if ((*vm)->AttachCurrentThread(vm, &env, NULL) != JNI_OK) return strdup("{\"error\":\"Cannot attach JNI thread\"}");
  attached = 1;
 }
 jstring arg = (*env)->NewStringUTF(env, input);
 jstring result = (*env)->CallObjectMethod(env, bridge, callMethod, arg);
 char *out = NULL;
 if ((*env)->ExceptionCheck(env)) { (*env)->ExceptionClear(env); }
 else if (result) {
  const char *s = (*env)->GetStringUTFChars(env, result, NULL);
  if (s) { out = strdup(s); (*env)->ReleaseStringUTFChars(env, result, s); }
 }
 if (result) (*env)->DeleteLocalRef(env, result);
 (*env)->DeleteLocalRef(env, arg);
 if (attached) (*vm)->DetachCurrentThread(vm);
 return out ? out : strdup("{\"error\":\"Android JNI callback failed\"}");
}
*/
import "C"

import (
	"encoding/json"
	"errors"
	"unsafe"
)

// ServiceEvents wakes the runner when Binder reports service death. Polling
// remains a fallback for a missed lifecycle callback during Activity teardown.
var ServiceEvents = make(chan struct{}, 1)

//export Java_com_clashcube_android_NativeBridge_nativeStopped
func Java_com_clashcube_android_NativeBridge_nativeStopped(env *C.JNIEnv, obj C.jobject) {
	select {
	case ServiceEvents <- struct{}{}:
	default:
	}
}

//export Java_com_clashcube_android_NativeBridge_nativeInit
func Java_com_clashcube_android_NativeBridge_nativeInit(env *C.JNIEnv, obj C.jobject) {
	C.initBridge(env, obj)
}

// Call invokes the process-local Android adapter. Java catches all exceptions;
// the C boundary also clears exceptions before returning to a Go-created thread.
func Call(request any, result any) error {
	b, err := json.Marshal(request)
	if err != nil {
		return err
	}
	arg := C.CString(string(b))
	defer C.free(unsafe.Pointer(arg))
	out := C.bridgeCall(arg)
	defer C.free(unsafe.Pointer(out))
	data := []byte(C.GoString(out))
	var status struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &status); err != nil {
		return err
	}
	if status.Error != "" {
		return errors.New(status.Error)
	}
	if result != nil {
		return json.Unmarshal(data, result)
	}
	return nil
}
