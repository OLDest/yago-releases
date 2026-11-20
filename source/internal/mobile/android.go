//go:build android

package mobile

/*
#include <jni.h>
#include <stdlib.h>
#include <stdint.h>
static jint bridge_env(JavaVM *vm, JNIEnv **env) { return (*vm)->GetEnv(vm,(void**)env,JNI_VERSION_1_6); }
static jint bridge_attach(JavaVM *vm, JNIEnv **env) { return (*vm)->AttachCurrentThread(vm,env,NULL); }
static void bridge_detach(JavaVM *vm) { (*vm)->DetachCurrentThread(vm); }
static jint bridge_push(JNIEnv *e) { return (*e)->PushLocalFrame(e,32); }
static void bridge_pop(JNIEnv *e) { (*e)->PopLocalFrame(e,NULL); }
static int bridge_exception(JNIEnv *e) { if(!(*e)->ExceptionCheck(e))return 0;(*e)->ExceptionClear(e);return 1; }
static jstring bridge_string(JNIEnv *e,jchar *s,jint n) { return (*e)->NewString(e,s,n); }
static jclass bridge_class(JNIEnv *e,jobject ctx) {
 jclass cc=(*e)->GetObjectClass(e,ctx);
 if(!cc)return NULL;
 jmethodID get=(*e)->GetMethodID(e,cc,"getClassLoader","()Ljava/lang/ClassLoader;");
 if(!get)return NULL;
 jobject loader=(*e)->CallObjectMethod(e,ctx,get);
 if(!loader)return NULL;
 jclass lc=(*e)->GetObjectClass(e,loader);
 jmethodID load=(*e)->GetMethodID(e,lc,"loadClass","(Ljava/lang/String;)Ljava/lang/Class;");
 if(!load)return NULL;
 jstring name=(*e)->NewStringUTF(e,"com.yago.mobile.Bridge");
 return (jclass)(*e)->CallObjectMethod(e,loader,load,name);
}
static jstring bridge_call(JNIEnv *e,jclass c,char *method,jstring arg) {
 jmethodID m=(*e)->GetStaticMethodID(e,c,method,"(Ljava/lang/String;)Ljava/lang/String;");
 if(!m)return NULL;
 return (jstring)(*e)->CallStaticObjectMethod(e,c,m,arg);
}
static jint bridge_len(JNIEnv *e,jstring s) { return (*e)->GetStringLength(e,s); }
static const jchar *bridge_chars(JNIEnv *e,jstring s) { return (*e)->GetStringChars(e,s,NULL); }
static void bridge_release(JNIEnv *e,jstring s,const jchar *p) { (*e)->ReleaseStringChars(e,s,p); }
*/
import "C"

import (
	"errors"
	"gioui.org/app"
	"runtime"
	"strings"
	"unicode/utf16"
	"unsafe"
)

func Call(method, arg string) (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	vm := (*C.JavaVM)(unsafe.Pointer(app.JavaVM()))
	if vm == nil {
		return "", errors.New("Android VM unavailable")
	}
	var env *C.JNIEnv
	attached := false
	if C.bridge_env(vm, &env) != C.JNI_OK {
		if C.bridge_attach(vm, &env) != C.JNI_OK {
			return "", errors.New("Android thread unavailable")
		}
		attached = true
	}
	if attached {
		defer C.bridge_detach(vm)
	}
	if C.bridge_push(env) != C.JNI_OK {
		return "", errors.New("Android local references unavailable")
	}
	defer C.bridge_pop(env)
	cls := C.bridge_class(env, C.jobject(app.AppContext()))
	if C.bridge_exception(env) != 0 || cls == 0 {
		return "", errors.New("Android bridge class unavailable")
	}
	units := utf16.Encode([]rune(arg))
	var ptr *C.jchar
	if len(units) > 0 {
		ptr = (*C.jchar)(unsafe.Pointer(&units[0]))
	}
	value := C.bridge_string(env, ptr, C.jint(len(units)))
	name := C.CString(method)
	defer C.free(unsafe.Pointer(name))
	result := C.bridge_call(env, cls, name, value)
	if C.bridge_exception(env) != 0 {
		return "", errors.New("Android integration failed")
	}
	if result == 0 {
		return "", nil
	}
	n := C.bridge_len(env, result)
	chars := C.bridge_chars(env, result)
	if chars == nil {
		return "", errors.New("Android string unavailable")
	}
	defer C.bridge_release(env, result, chars)
	return string(utf16.Decode(unsafe.Slice((*uint16)(unsafe.Pointer(chars)), int(n)))), nil
}

//export Java_com_yago_mobile_Bridge_nativeCommand
func Java_com_yago_mobile_Bridge_nativeCommand(env *C.JNIEnv, cls C.jclass, value C.jstring) {
	if value == 0 {
		return
	}
	n := C.bridge_len(env, value)
	chars := C.bridge_chars(env, value)
	if chars == nil {
		return
	}
	command := string(utf16.Decode(unsafe.Slice((*uint16)(unsafe.Pointer(chars)), int(n))))
	C.bridge_release(env, value, chars)
	if len(command) > 64 || strings.ContainsRune(command, '\x00') {
		return
	}
	select {
	case Commands <- command:
	default:
	}
}
