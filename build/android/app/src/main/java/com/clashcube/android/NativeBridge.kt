package com.clashcube.android

import org.json.JSONObject

// Each process loads exactly one Go runtime. The adapter is process-owned;
// core callbacks never depend on an Activity or on the UI process.
class NativeBridge(private val handler: (JSONObject) -> JSONObject) {
    external fun nativeInit()
    external fun nativeStopped()
    fun call(input: String): String = try {
        handler(JSONObject(input)).toString()
    } catch (e: Throwable) {
        JSONObject().put("error", e.message ?: e.javaClass.simpleName).toString()
    }
}

object CoreNative {
    init { System.loadLibrary("clashcore") }
    external fun run(input: String): String
    fun execute(request: JSONObject): JSONObject = JSONObject(run(request.toString()))
}
