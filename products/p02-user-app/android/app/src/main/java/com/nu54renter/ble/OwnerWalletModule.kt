package com.nu54renter.ble

import android.app.Activity
import com.nu54renter.BuildConfig
import com.nu54renter.MainActivity
import android.content.Intent
import android.provider.OpenableColumns
import com.facebook.react.bridge.BaseActivityEventListener
import android.net.ConnectivityManager
import android.net.Network
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.provider.Settings
import android.speech.tts.TextToSpeech
import android.util.Base64
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReadableArray
import java.net.InetSocketAddress
import java.net.Proxy
import java.net.URI
import javax.net.ssl.HttpsURLConnection
import java.io.ByteArrayOutputStream
import java.util.concurrent.Executors
import org.json.JSONObject
import java.security.MessageDigest
import java.security.SecureRandom

class OwnerWalletModule(context: ReactApplicationContext) : NativeOwnerWalletSpec(context) {
    private val keys = OwnerKeys(context)
    private val rpcWorker = Executors.newSingleThreadExecutor()
    private val fileWorker = Executors.newSingleThreadExecutor()
    private val main = Handler(Looper.getMainLooper())
    private var speech: TextToSpeech? = null
    private var tls: OwnerTls? = null
    private var tlsStarted = 0L
    private val connectivity = context.getSystemService(ConnectivityManager::class.java)
    @Volatile private var observingLink = false
    private val networkCallback = OwnerDefaultNetworkCallback(connectivity) { link ->
        if (observingLink) emitOnNetworkLink(link)
    }

    @Synchronized
    override fun getNetworkLink(promise: Promise) {
        if (!observingLink && connectivity != null) {
            try {
                connectivity.registerDefaultNetworkCallback(networkCallback)
                observingLink = true
            } catch (_: RuntimeException) {
                promise.resolve("unknown")
                return
            }
        }
        promise.resolve(if (!observingLink) "unknown" else if (connectivity?.activeNetwork == null) "offline" else "online")
    }

    private var filePromise: Promise? = null
    private var psbtDocument = false
    private val fileRequest = 15342
    init {
        context.addActivityEventListener(object : BaseActivityEventListener() {
            override fun onActivityResult(activity: Activity, requestCode: Int, resultCode: Int, data: Intent?) {
                if (requestCode != fileRequest) return
                val pending = filePromise ?: return
                filePromise = null
                val psbt = psbtDocument
                psbtDocument = false
                if (resultCode != Activity.RESULT_OK || data?.data == null) {
                    pending.reject("FILE_CANCELLED", "No file selected."); return
                }
                val uri = requireNotNull(data.data)
                fileWorker.execute { try {
                    val resolver = reactApplicationContext.contentResolver
                    val mime = resolver.getType(uri)
                    if (!psbt) require(mime in setOf("application/json", "text/json", "application/octet-stream"))
                    val name = resolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use {
                        if (it.moveToFirst()) it.getString(0) else null
                    }
                    val extension = name?.substringAfterLast(".")?.lowercase()
                    val image = psbt && extension in setOf("png", "jpg", "jpeg")
                    require(if (psbt) image || extension in setOf("psbt", "txt") else extension == "json")
                    val limit = if (image) OwnerPsbtQr.MaxFileBytes else if (psbt) 2378 else 8192
                    val text = resolver.openInputStream(uri).use { stream ->
                        val input = requireNotNull(stream); val bytes = ByteArrayOutputStream(); val chunk = ByteArray(1024)
                        while (true) { val count = input.read(chunk); if (count < 0) break
                            require(bytes.size() + count <= limit); bytes.write(chunk, 0, count) }
                        val raw = bytes.toByteArray(); require(raw.isNotEmpty())
                        if (image) return@use OwnerPsbtQr.readImage(raw, requireNotNull(extension))
                        if (psbt && raw.size >= 5 && raw.take(5) == listOf<Byte>(0x70,0x73,0x62,0x74,-1)) {
                            require(extension == "psbt" && raw.size <= 1780); return@use Base64.encodeToString(raw, Base64.NO_WRAP)
                        }
                        val decoded = raw.toString(Charsets.UTF_8); require(decoded.toByteArray(Charsets.UTF_8).contentEquals(raw)); decoded
                    }
                    pending.resolve(text)
                } catch (_: Exception) { pending.reject("OWNER_UNAVAILABLE", "The selected file is unavailable.") } }
            }
        })
    }
    override fun pickInstallationFile(promise: Promise) = pickWalletDocument(promise, false)

    override fun pickPsbtFile(promise: Promise) = pickWalletDocument(promise, true)

    private fun pickWalletDocument(promise: Promise, psbt: Boolean) {
        val activity = reactApplicationContext.currentActivity
        if (activity == null || filePromise != null) { promise.reject("OWNER_UNAVAILABLE", "File selection unavailable."); return }
        filePromise = promise
        psbtDocument = psbt
        try {
            val intent = Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType(if (psbt) "*/*" else "application/json")
            activity.startActivityForResult(intent, fileRequest)
        } catch (_: Exception) { filePromise = null; promise.reject("OWNER_UNAVAILABLE", "File selection unavailable.") }
    }

    private fun resolve(promise: Promise, body: () -> Any?) {
        try {
            promise.resolve(body())
        } catch (_: Exception) {
            promise.reject("OWNER_UNAVAILABLE", "The requested owner operation is unavailable")
        }
    }

    private var protectedActivity: MainActivity? = null
    override fun setHomeProtection(enabled: Boolean, promise: Promise) {
        main.post {
            val activity = reactApplicationContext.currentActivity as? MainActivity
            if (enabled) {
                if (activity == null) { promise.reject("OWNER_UNAVAILABLE", "No active window"); return@post }
                if (protectedActivity !== activity) protectedActivity?.setWalletHomeProtection(false)
                activity.setWalletHomeProtection(true)
                protectedActivity = activity
            } else {
                protectedActivity?.setWalletHomeProtection(false)
                protectedActivity = null
            }
            promise.resolve(null)
        }
    }

    override fun loadProfile(promise: Promise) = resolve(promise) { keys.load() }
    override fun saveProfile(value: String, promise: Promise) = resolve(promise) { keys.save(value); null }
    override fun compareAndSaveProfile(expected: String, value: String, promise: Promise) =
        resolve(promise) { keys.compareAndSave(expected, value) }
    override fun publicIdentity(promise: Promise) = resolve(promise) {
        Base64.encodeToString(keys.identity().certificate.publicKey.encoded, Base64.NO_WRAP)
    }
    override fun randomBytes(size: Double, promise: Promise) = resolve(promise) {
        require(size == size.toInt().toDouble() && size.toInt() in 1..64)
        Base64.encodeToString(ByteArray(size.toInt()).also(SecureRandom()::nextBytes), Base64.NO_WRAP)
    }

    override fun installationClock(promise: Promise) = resolve(promise) {
        val boot = Settings.Global.getInt(reactApplicationContext.contentResolver, Settings.Global.BOOT_COUNT, -1)
        require(boot >= 0)
        JSONObject().put("boot", "android-boot-$boot").put("uptimeMs", SystemClock.elapsedRealtime())
            .put("wallMs", System.currentTimeMillis()).toString()
    }

    @Synchronized
    override fun signChallenge(record: String, promise: Promise) = resolve(promise) {
        val session = requireNotNull(tls)
        require(session.ready && System.currentTimeMillis()-tlsStarted in 0..90_000)
        val bytes = Base64.decode(record, Base64.NO_WRAP)
        val owner = MessageDigest.getInstance("SHA-256").digest(keys.identity().certificate.publicKey.encoded)
        Base64.encodeToString(keys.sign(bytes, owner), Base64.NO_WRAP)
    }

    override fun speak(text: String, language: String, promise: Promise) {
        if (language !in setOf("ko", "en") || text.length !in 1..400) return promise.reject("UNAVAILABLE", "Read-aloud is unavailable")
        main.post {
            speech?.shutdown()
            speech = TextToSpeech(reactApplicationContext) { status ->
                main.post {
                    val reader = speech
                    val localVoice = reader?.voices?.firstOrNull {
                        it.locale.language == language && !it.isNetworkConnectionRequired
                    }
                    if (status != TextToSpeech.SUCCESS || reader == null || localVoice == null) {
                        promise.reject("UNAVAILABLE", "A local voice is unavailable")
                    } else {
                        reader.voice = localVoice
                        if (reader.speak(text, TextToSpeech.QUEUE_FLUSH, null, "nu54-wallet") == TextToSpeech.ERROR)
                            promise.reject("UNAVAILABLE", "Read-aloud is unavailable")
                        else promise.resolve(null)
                    }
                }
            }
        }
    }

    @Synchronized
    override fun startTls(caDer: String, clientChain: ReadableArray, boardSPKI: String, promise: Promise) = resolve(promise) {
        tls = null
        tlsStarted = System.currentTimeMillis()
        val chain = (0 until clientChain.size()).map { requireNotNull(clientChain.getString(it)) }
        OwnerTls(keys.identity(), caDer, chain, boardSPKI).also { tls = it }.feed(byteArrayOf())
    }

    @Synchronized
    override fun feedTls(input: String, promise: Promise) = resolve(promise) {
        require(input.length <= 43_700)
        requireNotNull(tls).feed(Base64.decode(input, Base64.NO_WRAP))
    }

    @Synchronized
    override fun sendTls(cleartext: String, promise: Promise) = resolve(promise) {
        require(cleartext.length <= 4_096)
        requireNotNull(tls).send(Base64.decode(cleartext, Base64.NO_WRAP))
    }

    @Synchronized
    override fun stopTls(promise: Promise) {
        tls = null
        promise.resolve(null)
    }

    override fun evmRpc(endpoint: String, packet: String, promise: Promise) {
        rpcWorker.execute {
            resolve(promise) {
                require(endpoint.length in 1..512 && packet.length in 1..6144)
                val uri = URI(endpoint)
                require(uri.scheme == "https" && uri.host != null && uri.userInfo == null && uri.fragment == null)
                val request = JSONObject(packet)
                val method = request.getString("method")
                val evm = setOf("eth_chainId", "eth_getBalance", "eth_getBlockByNumber", "eth_getTransactionReceipt", "eth_sendRawTransaction", "nu54_walletRegistry", "nu54_walletQuote", "nu54_marketBoardGrant", "nu54_walletLogin","eth_call", "nu54_walletRevokeAllowance","nu54_walletExternalSpend","nu54_walletDappAllowance","nu54_walletConnectPeer")
                val localChains = setOf("nu54_bip48Policy", "nu54_bip48Prepare", "nu54_bitcoinReplacementStatus", "nu54_submitBitcoinReplacement", "nu54_walletTime", "nu54_prepareWalletTransfer", "getblockhash", "getblockchaininfo", "getrawtransaction", "testmempoolaccept", "listunspent", "getaddressinfo", "sendrawtransaction",
                    "getGenesisHash", "getSignatureStatuses", "isBlockhashValid", "getBalance", "getBlockHeight", "sendTransaction",
                    "server_info", "tx", "ledger_current", "account_info", "ledger", "submit")
                val externalReads = setOf("getblockchaininfo", "getblockhash", "gettxout", "getaddressinfo", "getreceivedbyaddress", "listunspent",
                    "getGenesisHash", "getBalance", "server_info", "account_info", "ledger", "getAccountInfo", "getTokenAccountsByOwner", "account_lines")
                val signetRoute = Regex("""https://mempool\.space/signet/api/(block-height/[0-9]{1,9}|blocks/tip/(height|hash)|address/tb1q[a-z0-9]{38}(/utxo)?|block/[a-f0-9]{64}/header|tx/[a-f0-9]{64}/(merkle-proof|hex))""")
                val signet = method == "nu54_signetRead" && request.length() == 1 && signetRoute.matches(endpoint)
                val commissionedRoute = mapOf(
                    "/bitcoin" to setOf("nu54_bip48Policy", "nu54_bip48Prepare", "getblockhash", "getblockchaininfo", "getrawtransaction", "testmempoolaccept",
                        "getaddressinfo", "listunspent"),
                    "/solana" to setOf("getGenesisHash", "getSignatureStatuses", "isBlockhashValid", "getBlockHeight", "getBalance"),
                    "/xrpl" to setOf("server_info", "tx", "ledger_current", "account_info", "ledger"))
                val dnsHost = Regex("""(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}""")
                val pathMethods = commissionedRoute[uri.rawPath]
                val qaLoopback = BuildConfig.NU54_WALLET_SEND_QA &&
                    reactApplicationContext.packageName == "com.nu54renter.wallet.sendqa" &&
                    uri.host == "wallet.localhost" && uri.port == 18449
                val commissioned = pathMethods != null && uri.rawQuery == null && dnsHost.matches(uri.host) &&
                    (!uri.host.endsWith(".localhost") || qaLoopback) &&
                    request.optString("walletToken").matches(Regex("[a-f0-9]{64}")) &&
                    (method in pathMethods || method in setOf("nu54_walletTime", "nu54_prepareWalletTransfer", "nu54_submitCommissionedTransfer"))
                require(if (request.has("walletToken")) commissioned else signet ||
                    if (method in evm || method in externalReads) packet.length <= 2048 else method in localChains &&
                    uri.host in setOf("localhost", "127.0.0.1", "[::1]", "::1") && uri.query == null)
                val socket = if (qaLoopback) uri.toURL().openConnection(
                    Proxy(Proxy.Type.HTTP, InetSocketAddress("127.0.0.1", 18450))) else uri.toURL().openConnection()
                val connection = socket as HttpsURLConnection
                try {
                    connection.instanceFollowRedirects = false
                    connection.connectTimeout = 8000
                    connection.readTimeout = 8000
                    connection.requestMethod = if (signet) "GET" else "POST"
                    if (!signet) {
                        connection.doOutput = true
                        connection.setRequestProperty("Content-Type", "application/json")
                        connection.setFixedLengthStreamingMode(packet.toByteArray(Charsets.UTF_8).size)
                        connection.outputStream.use { it.write(packet.toByteArray(Charsets.UTF_8)) }
                    }
                    require(connection.responseCode == 200)
                    val result = ByteArrayOutputStream()
                    connection.inputStream.use { input ->
                        val chunk = ByteArray(4096)
                        while (true) {
                            val count = input.read(chunk)
                            if (count < 0) break
                            require(result.size() + count <= 256000)
                            result.write(chunk, 0, count)
                        }
                    }
                    result.toString("UTF-8")
                } finally {
                    connection.disconnect()
                }
            }
        }
    }

    @Synchronized
    override fun invalidate() {
        if (observingLink) connectivity?.unregisterNetworkCallback(networkCallback)
        observingLink = false
        rpcWorker.shutdownNow()
        tls = null
        main.post { speech?.shutdown(); speech = null }
        super.invalidate()
    }
}


private class OwnerDefaultNetworkCallback(
    private val connectivity: ConnectivityManager?,
    private val report: (String) -> Unit,
) : ConnectivityManager.NetworkCallback() {
    override fun onAvailable(network: Network) = report("online")

    override fun onLost(network: Network) {
        val current = connectivity?.activeNetwork
        report(if (current == null || current == network) "offline" else "online")
    }
}
