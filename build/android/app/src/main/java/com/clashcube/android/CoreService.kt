package com.clashcube.android

import android.app.*
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.*
import android.os.*
import com.wails.app.R
import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.net.InetSocketAddress
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean

class CoreService : VpnService() {
    private val worker = Executors.newSingleThreadExecutor()
    private val ending = AtomicBoolean(false)
    private val packages = ConcurrentHashMap<Int, String>()
    private lateinit var native: NativeBridge
    private lateinit var connectivity: ConnectivityManager
    @Volatile private var running = false
    private var state = JSONObject()
    private var observed = false
    private var lastDNS: List<String>? = null
    private val networks = LinkedHashMap<Network, LinkProperties>()
    private val networkCallback = object : ConnectivityManager.NetworkCallback() {
        override fun onCapabilitiesChanged(network: Network, capabilities: NetworkCapabilities) {
            worker.execute { updateDNS() }
        }
        override fun onLinkPropertiesChanged(network: Network, properties: LinkProperties) {
            worker.execute { networks[network] = properties; updateDNS() }
        }
        override fun onLost(network: Network) {
            worker.execute { networks.remove(network); updateDNS() }
        }
    }
    private val messenger = Messenger(Handler(Looper.getMainLooper()) { message ->
        val reply = message.replyTo
        val input = message.data.getString("json") ?: "{}"
        worker.execute {
            var stop = false
            val response = try {
                val request = JSONObject(input)
                when (request.getString("op")) {
                    "start" -> { startCore(request); JSONObject().put("startedAt", state.optLong("startedAt")) }
                    "status" -> JSONObject(state.toString()).put("running", running)
                    "stop" -> { stop = true; JSONObject() }
                    else -> error("Unknown VPN service operation")
                }
            } catch (e: Throwable) {
                stop = true
                JSONObject().put("error", e.message ?: "VPN startup failed")
            }
            try { reply.send(Message.obtain(null, 1).also { it.data = Bundle().apply { putString("json", response.toString()) } }) }
            catch (_: Throwable) {}
            if (stop) finishCore()
        }
        true
    })

    override fun onCreate() {
        super.onCreate()
        // Force library loading before resolving NativeBridge JNI symbols.
        CoreNative
        connectivity = getSystemService(ConnectivityManager::class.java)
        native = NativeBridge(::callback).also { it.nativeInit() }
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(NotificationChannel("vpn", "ClashCube VPN", NotificationManager.IMPORTANCE_LOW))
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == "stop") { worker.execute { finishCore() }; return START_NOT_STICKY }
        val open = PendingIntent.getActivity(this, 0, packageManager.getLaunchIntentForPackage(packageName), PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT)
        val stop = PendingIntent.getService(this, 1, Intent(this, CoreService::class.java).setAction("stop"), PendingIntent.FLAG_IMMUTABLE)
        val notification = Notification.Builder(this, "vpn").setSmallIcon(R.mipmap.ic_launcher)
            .setContentTitle("ClashCube").setContentText(getString(R.string.vpn_running)).setOngoing(true).setContentIntent(open)
            .addAction(Notification.Action.Builder(null, getString(R.string.vpn_stop), stop).build()).build()
        if (Build.VERSION.SDK_INT >= 34) startForeground(17, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        else startForeground(17, notification)
        return START_NOT_STICKY
    }

    override fun onBind(intent: Intent): IBinder? =
        if (intent.action == "com.clashcube.android.CONTROL") messenger.binder else super.onBind(intent)

    private fun startCore(request: JSONObject) {
        check(!running && !ending.get()) { "VPN service is already running or stopping" }
        val home = File(filesDir, "core").canonicalFile
        check(File(request.getString("home")).canonicalFile == home && File(request.getString("file")).canonicalFile == File(home, "runtime.yaml")) { "Invalid core configuration path" }
        File(home, "service-stopped").delete()
        val builder = Builder().setSession("ClashCube").setMtu(9000).setBlocking(false).setMetered(false)
            .addAddress("172.19.0.1", 30).addDnsServer("172.19.0.2")
        fun routes(id: Int) { resources.getStringArray(id).forEach { route ->
            val parts = route.split('/'); builder.addRoute(parts[0], parts[1].toInt())
        } }
        routes(R.array.bypass_private_route)
        builder.addRoute("172.19.0.2", 32)
        if (request.optBoolean("ipv6")) {
            builder.addAddress("fdfe:dcba:9876::1", 126)
            builder.addDnsServer("fdfe:dcba:9876::2").addRoute("fdfe:dcba:9876::2", 128)
            routes(R.array.bypass_private_route6)
        }
        val fd = (builder.establish() ?: error("VPN permission was revoked")).detachFd()
        request.put("fd", fd)
        val result = CoreNative.execute(request)
        check(!result.has("error")) { result.optString("error") }
        state = JSONObject().put("addr", request.getString("addr")).put("secret", request.getString("secret"))
            .put("startedAt", android.os.SystemClock.elapsedRealtime())
        running = true
        connectivity.registerNetworkCallback(NetworkRequest.Builder()
            .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
            .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN).build(), networkCallback)
        observed = true
    }

    private fun updateDNS() {
        if (!running) return
        val network = networks.keys.minByOrNull {
            val caps = connectivity.getNetworkCapabilities(it)
            val transport = when {
                caps == null -> 100
                caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> 0
                caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> 1
                caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> 2
                else -> 3
            }
            transport + if (caps?.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED) == true) 0 else 10
        }
        val addresses = networks[network]?.dnsServers?.mapNotNull { it.hostAddress } ?: emptyList()
        if (addresses == lastDNS) return
        lastDNS = addresses
        CoreNative.execute(JSONObject().put("op", "dns").put("dns", JSONArray(addresses)))
    }

    private fun endpoint(text: String): InetSocketAddress {
        val index = text.lastIndexOf(':')
        return InetSocketAddress(text.substring(0, index).removePrefix("[").removeSuffix("]"), text.substring(index + 1).toInt())
    }

    private fun callback(request: JSONObject): JSONObject = when (request.getString("op")) {
        // Refused protection after revocation must not cause mihomo dial retries.
        "protect" -> { try { protect(request.getInt("fd")) } catch (_: Throwable) {}; JSONObject() }
        "owner" -> {
            val uid = try { connectivity.getConnectionOwnerUid(request.getInt("protocol"), endpoint(request.getString("source")), endpoint(request.getString("target"))) } catch (_: Throwable) { -1 }
            val name = if (uid < 0) "" else packages.getOrPut(uid) { packageManager.getPackagesForUid(uid)?.firstOrNull() ?: "" }
            JSONObject().put("uid", uid).put("package", name)
        }
        else -> error("Unknown core callback")
    }

    private fun finishCore() {
        if (!ending.compareAndSet(false, true)) return
        if (running) runCatching { File(filesDir, "core/service-stopped").writeText("stopped\n") }
        running = false
        if (observed) try { connectivity.unregisterNetworkCallback(networkCallback) } catch (_: Throwable) {}
        try { CoreNative.execute(JSONObject().put("op", "stop")) } finally {
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
            // Shutdown cannot reset all mihomo package state or controller
            // listeners. Every subsequent start must load a fresh Go runtime.
            Process.killProcess(Process.myPid())
        }
    }

    override fun onRevoke() { worker.execute { finishCore() } }
    override fun onDestroy() { worker.execute { finishCore() }; super.onDestroy() }
}
