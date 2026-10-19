package com.clashcube.android

import android.app.Service
import android.content.Intent
import android.os.*
import org.json.JSONObject
import java.util.concurrent.Executors

// Validation gets a disposable Go runtime, never the live core's singletons.
class CheckService : Service() {
    private val worker = Executors.newSingleThreadExecutor()
    private val messenger = Messenger(Handler(Looper.getMainLooper()) { message ->
        val reply = message.replyTo
        val input = message.data.getString("json") ?: "{}"
        worker.execute {
            val result = try { CoreNative.execute(JSONObject(input)) }
            catch (e: Throwable) { JSONObject().put("error", e.message ?: "Configuration validation failed") }
            try { reply.send(Message.obtain(null, 1).also { it.data = Bundle().apply { putString("json", result.toString()) } }) }
            catch (_: Throwable) {}
        }
        true
    })
    override fun onBind(intent: Intent): IBinder = messenger.binder
    override fun onUnbind(intent: Intent): Boolean {
        stopSelf()
        Process.killProcess(Process.myPid())
        return false
    }
}
