package com.nu54renter.ble

import android.content.Context
import android.content.pm.PackageManager
import com.nu54renter.BuildConfig
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.net.HttpURLConnection
import java.net.Proxy
import java.net.URL
import java.security.KeyStore
import java.security.SecureRandom
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.util.concurrent.Executors
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLSocketFactory
import javax.net.ssl.TrustManagerFactory

class MarketLabModule(context: ReactApplicationContext) : NativeMarketLabSpec(context) {
    companion object {
        private const val alias = "nu54-market-lab-wrap-8283-v1"
        private const val scopeSize = 112
        private const val seedSize = 32
        private val routes = Regex("/v1/(profile|wallet/0x[0-9a-fA-F]{40}|positions/[0-9]{1,78}(/history)?|" +
            "executions/0x[0-9a-fA-F]{64}/0x[0-9a-fA-F]{64}|approvals/0x[0-9a-fA-F]{64}/0x[0-9a-fA-F]{40}|quotes|transactions)")
        init {
            System.loadLibrary("nu54_market_lab")
        }
    }

    private data class Installed(val origin: String, val policy: String, val tls: SSLSocketFactory? = null,
        val operatorOrigin: String = "")

    private fun validOrigin(candidate: String): Boolean = runCatching {
        val origin = java.net.URI(candidate)
        val port = origin.port
        val authority = origin.host + if (port == -1) "" else ":$port"
        candidate.length <= 260 && origin.scheme == "https" && origin.rawAuthority == authority &&
            (port == -1 || port in 1..65535) &&
            origin.host?.matches(Regex("[a-z0-9][a-z0-9.-]*[.][a-z][a-z0-9-]*")) == true &&
            origin.rawPath.isEmpty() && origin.rawQuery == null && origin.rawFragment == null
    }.getOrDefault(false)

    private val installed = runCatching {
        val text = context.packageManager.getApplicationInfo(context.packageName, PackageManager.GET_META_DATA)
            .metaData?.getString("nu54.market.profile").orEmpty()
        if (text == "loopback" && BuildConfig.DEBUG) {
            Installed("http://127.0.0.1:8284", "loopback")
        } else {
            val parts = text.split('|')
            require(parts.size in 3..4 && parts[1].matches(Regex("0x[0-9a-f]{64}")) && validOrigin(parts[0]))
            val operator = parts.getOrNull(3).orEmpty()
            val operatorOrigin = operator.takeIf {
                validOrigin(it) && it.removeSuffix(":443") != parts[0].removeSuffix(":443")
            }.orEmpty()
            require(parts[2].length <= 6000 && parts[2].matches(Regex("[A-Za-z0-9+/]+={0,2}")))
            val cert = CertificateFactory.getInstance("X.509").generateCertificate(
                ByteArrayInputStream(Base64.decode(parts[2], Base64.DEFAULT))) as X509Certificate
            require(cert.basicConstraints >= 0)
            val anchors = KeyStore.getInstance(KeyStore.getDefaultType()).apply {
                load(null)
                setCertificateEntry("market-test-ca", cert)
            }
            val trust = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(anchors) }
            val tls = SSLContext.getInstance("TLS").apply { init(null, trust.trustManagers, null) }
            Installed(parts[0], parts[1], tls.socketFactory, operatorOrigin)
        }
    }.getOrNull()

    override fun deploymentPolicy(promise: Promise) {
        promise.resolve(installed?.policy.orEmpty())
    }

    override fun operatorOrigin(promise: Promise) {
        promise.resolve(installed?.operatorOrigin.orEmpty())
    }

    private val prefs = context.getSharedPreferences("nu54-market-lab-8283-v1", Context.MODE_PRIVATE)
    private val worker = Executors.newSingleThreadExecutor()
    private val rng = SecureRandom()
    private external fun nativeAddress(seed: ByteArray, entropy: ByteArray): String?
    private external fun nativeSign(seed: ByteArray, entropy: ByteArray, scope: ByteArray, to: ByteArray,
        nonce: Long, gasPrice: Long, data: ByteArray, quote: ByteArray): String?

    private fun bytes(input: String, count: Int): ByteArray {
        require(input.length == count * 2 && input.all { it in '0'..'9' || it in 'a'..'f' || it in 'A'..'F' })
        return ByteArray(count) { index -> input.substring(index * 2, index * 2 + 2).toInt(16).toByte() }
    }

    private fun key(create: Boolean): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey(alias, null) as? SecretKey)?.let { return it }
        require(create)
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setKeySize(256).setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
    }

    private fun seed(scope: ByteArray, create: Boolean): ByteArray {
        val existing = prefs.getString("sealed", null)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        if (existing != null) {
            val enclosed = Base64.decode(existing, Base64.NO_WRAP)
            require(enclosed.size == 12 + seedSize + 16)
            cipher.init(Cipher.DECRYPT_MODE, key(false), GCMParameterSpec(128, enclosed.copyOfRange(0, 12)))
            cipher.updateAAD(scope)
            return cipher.doFinal(enclosed, 12, enclosed.size - 12)
        }
        require(create)
        val generated = ByteArray(seedSize).also(rng::nextBytes)
        try {
            cipher.init(Cipher.ENCRYPT_MODE, key(true))
            cipher.updateAAD(scope)
            require(cipher.iv.size == 12)
            val enclosed = Base64.encodeToString(cipher.iv + cipher.doFinal(generated), Base64.NO_WRAP)
            require(prefs.edit().putString("sealed", enclosed).commit())
            return generated
        } catch (error: Exception) {
            generated.fill(0)
            throw error
        }
    }

    private fun run(promise: Promise, action: () -> Any?) {
        worker.execute {
            try { promise.resolve(requireNotNull(action())) }
            catch (_: Exception) {
                promise.reject("MARKET_LAB_UNAVAILABLE", "The isolated market wallet or connection is unavailable.")
            }
        }
    }

    private fun withSeed(scope: ByteArray, create: Boolean, action: (ByteArray, ByteArray) -> String?): String? {
        val seed = seed(scope, create)
        val entropy = ByteArray(32).also(rng::nextBytes)
        return try { action(seed, entropy) } finally {
            seed.fill(0)
            entropy.fill(0)
        }
    }

    override fun walletAddress(scope: String, create: Boolean, promise: Promise) = run(promise) {
        withSeed(bytes(scope, scopeSize), create, ::nativeAddress)
    }

    override fun deleteWallet(scope: String, promise: Promise) = run(promise) {
        withSeed(bytes(scope, scopeSize), false) { _, _ -> "confirmed" }
        require(prefs.edit().remove("sealed").commit())
        true
    }

    override fun sign(scope: String, target: String, nonce: Double, gasPrice: Double,
        data: String, quote: String, promise: Promise) = run(promise) {
        require(nonce >= 0 && nonce <= 9007199254740991.0 && nonce == nonce.toLong().toDouble())
        require(gasPrice > 0 && gasPrice <= 100000000000.0 && gasPrice == gasPrice.toLong().toDouble())
        require(data.length == 136 || data.length == 200 || data.length == 904)
        val domain = bytes(scope, scopeSize)
        val destination = bytes(target, 20)
        val calldata = bytes(data, data.length / 2)
        val offer = bytes(quote, 452)
        withSeed(domain, false) { secret, entropy ->
            nativeSign(secret, entropy, domain, destination, nonce.toLong(), gasPrice.toLong(), calldata, offer)
        }
    }

    override fun exchange(path: String, json: String, promise: Promise) = run(promise) {
        val writes = path == "/v1/quotes" || path == "/v1/transactions"
        require(path.length <= 160 && routes.matches(path) && writes == json.isNotEmpty() && json.length <= 8192)
        val deployment = requireNotNull(installed)
        val connection = URL(deployment.origin + path).openConnection(Proxy.NO_PROXY) as HttpURLConnection
        if (deployment.tls != null) (connection as HttpsURLConnection).sslSocketFactory = deployment.tls
        try {
            connection.instanceFollowRedirects = false
            connection.connectTimeout = 7000
            connection.readTimeout = 9000
            if (writes) {
                val body = json.encodeToByteArray()
                require(body.size <= 8192)
                connection.doOutput = true
                connection.requestMethod = "POST"
                connection.setRequestProperty("Content-Type", "application/json")
                connection.setFixedLengthStreamingMode(body.size)
                connection.outputStream.use { it.write(body) }
            }
            require(connection.responseCode in 200..299 && connection.contentType.orEmpty().startsWith("application/json"))
            val result = ByteArrayOutputStream()
            connection.inputStream.use { input ->
                val buffer = ByteArray(4096)
                while (true) {
                    val count = input.read(buffer)
                    if (count < 0) break
                    require(result.size() + count <= 32768)
                    result.write(buffer, 0, count)
                }
            }
            result.toString(Charsets.UTF_8.name())
        } finally {
            connection.disconnect()
        }
    }

    override fun invalidate() {
        worker.shutdownNow()
        super.invalidate()
    }
}
