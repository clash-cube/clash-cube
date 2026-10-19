package com.clashcube.android

import android.app.Activity
import android.app.ActivityManager
import android.content.*
import android.net.VpnService
import android.os.*
import org.json.JSONObject
import java.io.File
import java.lang.ref.WeakReference
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

object UIBridge {
    private val main = Handler(Looper.getMainLooper())
    private var activity = WeakReference<Activity>(null)
    private lateinit var context: Context
    private var native: NativeBridge? = null
    private var consent: CountDownLatch? = null
    private var granted = false
    private var core: Connection? = null
    private const val VPN_REQUEST = 8317
    @Volatile private var startedAt = 0L

    // Monotonic service time survives UI recreation through the private status reply.
    @JvmStatic fun uptimeSeconds(): Long = if (startedAt > 0) (SystemClock.elapsedRealtime() - startedAt) / 1000 else 0

    @JvmStatic fun initialize(owner: Activity) {
        activity = WeakReference(owner)
        context = owner.applicationContext
        if (native == null) native = NativeBridge(::command).also { it.nativeInit() }
        if (Build.VERSION.SDK_INT >= 33 && owner.checkSelfPermission("android.permission.POST_NOTIFICATIONS") != android.content.pm.PackageManager.PERMISSION_GRANTED) {
            owner.requestPermissions(arrayOf("android.permission.POST_NOTIFICATIONS"), 8318)
        }
    }

    @JvmStatic fun activityResult(request: Int, result: Int): Boolean {
        if (request != VPN_REQUEST) return false
        granted = result == Activity.RESULT_OK
        consent?.countDown()
        return true
    }

    private fun prepare() {
        val intent = VpnService.prepare(context) ?: return
        val latch = CountDownLatch(1)
        consent = latch
        granted = false
        main.post {
            val owner = activity.get()
            if (owner == null) latch.countDown()
            else try { owner.startActivityForResult(intent, VPN_REQUEST) } catch (_: Throwable) { latch.countDown() }
        }
        check(latch.await(120, TimeUnit.SECONDS) && granted) { "VPN permission was refused or the permission request expired" }
        consent = null
    }

    @Synchronized private fun command(request: JSONObject): JSONObject {
        when (request.getString("op")) {
            "check" -> {
                val connection = connect(CheckService::class.java, true) ?: error("Cannot bind configuration checker")
                try { return connection.request(request) }
                finally { connection.release() }
            }
            "start" -> {
                prepare()
                context.startForegroundService(Intent(context, CoreService::class.java))
                val connection = core?.takeIf { it.alive() } ?: connect(CoreService::class.java, false)
                core = connection ?: error("Cannot bind VPN service")
                val result = connection.request(request)
                startedAt = result.optLong("startedAt")
                if (result.has("error")) connection.dead.await(10, TimeUnit.SECONDS)
                return result
            }
            "attach" -> {
                if (!serviceExists()) return JSONObject().put("running", false)
                core = core?.takeIf { it.alive() } ?: connect(CoreService::class.java, false)
                val result = core?.request(JSONObject().put("op", "status")) ?: JSONObject().put("running", false)
                startedAt = result.optLong("startedAt")
                return result
            }
            "status" -> return JSONObject().put("running", core?.alive() == true)
                .put("stopped", File(context.filesDir, "core/service-stopped").exists())
            "stop" -> {
                val connection = core?.takeIf { it.alive() } ?: if (serviceExists()) connect(CoreService::class.java, false) else null
                if (connection != null) {
                    connection.request(request)
                    check(connection.dead.await(10, TimeUnit.SECONDS)) { "VPN service did not exit" }
                    connection.release()
                }
                core = null
                return JSONObject()
            }
            else -> error("Unknown Android UI operation")
        }
    }

    @Suppress("DEPRECATION")
    private fun serviceExists(): Boolean = context.getSystemService(ActivityManager::class.java)
        .getRunningServices(Int.MAX_VALUE).any { it.service.className == CoreService::class.java.name }

    private fun connect(type: Class<*>, create: Boolean): Connection? {
        val connection = Connection()
        val intent = Intent(context, type).setAction("com.clashcube.android.CONTROL")
        if (!context.bindService(intent, connection, if (create) Context.BIND_AUTO_CREATE else 0)) return null
        connection.bound = true
        if (!connection.ready.await(10, TimeUnit.SECONDS) || !connection.alive()) {
            connection.release()
            return null
        }
        return connection
    }

    private class Connection : ServiceConnection {
        val ready = CountDownLatch(1)
        val dead = CountDownLatch(1)
        @Volatile private var remote: Messenger? = null
        @Volatile var bound = false
        @Synchronized fun release() { if (bound) { context.unbindService(this); bound = false } }
        override fun onServiceConnected(name: ComponentName, binder: IBinder) { remote = Messenger(binder); ready.countDown() }
        override fun onServiceDisconnected(name: ComponentName) { remote = null; release(); dead.countDown(); try { native?.nativeStopped() } catch (_: Throwable) {} }
        override fun onBindingDied(name: ComponentName) { onServiceDisconnected(name); ready.countDown() }
        override fun onNullBinding(name: ComponentName) { ready.countDown(); dead.countDown() }
        fun alive() = remote?.binder?.isBinderAlive == true
        fun request(request: JSONObject): JSONObject {
            val latch = CountDownLatch(1)
            var response = JSONObject().put("error", "Android service did not reply")
            val reply = Messenger(Handler(Looper.getMainLooper()) { message ->
                response = JSONObject(message.data.getString("json") ?: "{}")
                latch.countDown(); true
            })
            val message = Message.obtain(null, 1).also {
                it.data = Bundle().apply { putString("json", request.toString()) }; it.replyTo = reply
            }
            (remote ?: error("Android service disconnected")).send(message)
            check(latch.await(90, TimeUnit.SECONDS)) { "Android service request timed out" }
            return response
        }
    }
}
