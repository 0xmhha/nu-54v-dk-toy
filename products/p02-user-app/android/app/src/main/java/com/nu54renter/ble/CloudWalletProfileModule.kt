package com.nu54renter.ble

import android.content.pm.PackageManager
import com.nu54renter.BuildConfig
import android.util.Base64
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import java.io.ByteArrayOutputStream
import java.io.File
import java.security.KeyStore
import java.security.MessageDigest
import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.AtomicFile
import java.net.URL
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicLong
import javax.net.ssl.HttpsURLConnection
import org.json.JSONObject

/** Uses the public P03 endpoints supplied by the signed Android application. */
class CloudWalletProfileModule(private val context: ReactApplicationContext) : NativeCloudWalletProfileSpec(context) {
    private val executor = Executors.newSingleThreadExecutor()
    private companion object { val generation = AtomicLong() }

    private fun profileText(): String {
        @Suppress("DEPRECATION")
        val info = context.packageManager.getApplicationInfo(context.packageName, PackageManager.GET_META_DATA)
        val encoded = info.metaData?.getString("nu54.p03.profile").orEmpty()
        val bytes = Base64.decode(encoded, Base64.NO_WRAP)
        require(bytes.size <= 4096)
        return bytes.decodeToString()
    }

    override fun profile(promise: Promise) {
        try {
            promise.resolve(profileText())
        } catch (_: Exception) {
            promise.reject("P03_PROFILE", "Cloud wallet profile is unavailable")
        }
    }

    private external fun startGoParticipant(path: String, key: ByteArray, subject: String, authority: String, expires: Long): Int
    private external fun stopGoParticipant()
    private external fun prepareGoParticipant(path: String, key: ByteArray): String?
    private external fun installGoParticipant(path: String, key: ByteArray, subject: String, authority: String, roster: String): Int

    private var labScope: String? = null
    private var labFixture = false

    fun enableLabFixture() {
        require(BuildConfig.DEBUG)
        labFixture = true
    }

    fun useLabScope(subject: String) {
        require(BuildConfig.DEBUG && subject.matches(Regex("[a-f0-9-]{36}")))
        labScope = subject
    }

    private fun withParticipant(promise: Promise, subject: String, fixture: Boolean = false, work: (File, ByteArray, String) -> Any) {
        executor.execute {
            try {
                require(!fixture || labFixture)
                val profile = if (fixture) null else JSONObject(profileText())
                val scope = if (fixture) "" else {
                    require(subject.matches(Regex("[a-f0-9-]{36}")))
                    labScope ?: MessageDigest.getInstance("SHA-256")
                        .digest("${profile!!.getString("api")}|${profile.getString("identity")}|$subject".encodeToByteArray())
                        .joinToString("") { "%02x".format(it) }
                }
                val directory = File(context.noBackupFilesDir, if (fixture) "cloud-p03-participant" else "cloud-p03-enrolled-$scope")
                require(directory.isDirectory || directory.mkdir())
                System.loadLibrary("nu54_p03")
                val authority = profile?.getString("rosterAuthority").orEmpty()
                val key = participantVaultKey(directory, fixture, scope)
                try {
                    promise.resolve(work(directory, key, authority))
                } finally {
                    key.fill(0)
                }
            } catch (_: Exception) {
                promise.reject("P03_PARTICIPANT", "Local cloud wallet participant is unavailable")
            } catch (_: UnsatisfiedLinkError) {
                promise.reject("P03_PARTICIPANT", "Local cloud wallet participant is unavailable")
            }
        }
    }

    override fun startParticipant(subject: String, expires: Double, token: String, promise: Promise) {
        val intent = generation.get()
        withParticipant(promise, subject, subject.isEmpty()) { dir, key, authority ->
            if (!labFixture && labScope == null) {
                require(subject.isNotEmpty())
                val profile = JSONObject(profileText())
                val response = exchange(profile.getString("api") + "/v1/session", "GET", "", token)
                require(response.getInt("status") == 200)
                val authenticated = JSONObject(response.getString("body"))
                require(authenticated.optBoolean("linked", true) && authenticated.getString("subject") == subject && authenticated.getLong("expires") == expires.toLong())
            }
            intent == generation.get() && startGoParticipant(dir.absolutePath, key, subject, authority, expires.toLong()) == 0
        }
    }

    override fun stopParticipant(promise: Promise) {
        generation.incrementAndGet()
        executor.execute {
            try {
                stopGoParticipant()
                promise.resolve(true)
            } catch (_: UnsatisfiedLinkError) {
                promise.resolve(true)
            }
        }
    }

    override fun prepareParticipant(subject: String, promise: Promise) = withParticipant(promise, subject) { dir, key, _ ->
        checkNotNull(prepareGoParticipant(dir.absolutePath, key))
    }

    override fun installParticipant(subject: String, roster: String, promise: Promise) = withParticipant(promise, subject) { dir, key, authority ->
        require(subject.isNotEmpty() && roster.length <= 65536)
        installGoParticipant(dir.absolutePath, key, subject, authority, roster) == 0
    }

    private fun participantVaultKey(directory: File, fixture: Boolean, scope: String): ByteArray {
        val id = if (fixture) JSONObject(File(directory, "participant.json").readText()).getString("ID") else scope
        require(id.matches(Regex("[a-z0-9][a-z0-9-]{1,63}")))
        val alias = "nu54-p03-vault-" + id
        val sealed = AtomicFile(File(directory, "vault-key.sealed"))
        val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        val key = (keyStore.getKey(alias, null) as? SecretKey) ?: KeyGenerator.getInstance(
            KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore"
        ).apply {
            require(!sealed.baseFile.exists() && !File(directory, "vault").exists())
            init(KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setKeySize(256).setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        if (sealed.baseFile.exists()) {
            val bytes = sealed.readFully()
            require(bytes.size == 60)
            cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(128, bytes.copyOfRange(0, 12)))
            cipher.updateAAD(alias.encodeToByteArray())
            return cipher.doFinal(bytes, 12, 48)
        }
        require(!File(directory, "vault").exists())
        val fresh = ByteArray(32).also(SecureRandom()::nextBytes)
        try {
            cipher.init(Cipher.ENCRYPT_MODE, key)
            cipher.updateAAD(alias.encodeToByteArray())
            require(cipher.iv.size == 12)
            val output = sealed.startWrite()
            try {
                output.write(cipher.iv + cipher.doFinal(fresh))
                sealed.finishWrite(output)
            } catch (error: Exception) {
                sealed.failWrite(output)
                throw error
            }
            return fresh.copyOf()
        } finally {
            fresh.fill(0)
        }
    }

    override fun request(endpoint: String, method: String, body: String, token: String, promise: Promise) {
        executor.execute {
            try {
                promise.resolve(exchange(endpoint, method, body, token).toString())
            } catch (_: Exception) {
                promise.reject("P03_NETWORK", "Cloud wallet service is unavailable")
            }
        }
    }

    private fun exchange(endpoint: String, method: String, body: String, token: String): JSONObject {
        require(method in setOf("GET", "POST", "PUT", "DELETE") && body.length <= 8192 && token.length <= 8192)
        val profile = JSONObject(profileText())
        val url = URL(endpoint)
        val origin = "${url.protocol}://${url.authority}"
        val api = origin == profile.getString("api")
        require(url.protocol == "https" && url.userInfo == null && url.ref == null && url.query == null)
        if (api) {
            if (url.path == "/v1/onboarding") {
                require(profile.optJSONObject("onboarding") != null && method == "GET" && body.isEmpty() && token.isEmpty())
            } else {
                require(url.path.startsWith("/v1/") && token.matches(Regex("[A-Za-z0-9_.-]+")))
            }
        } else {
            require(listOf("identity", "approval", "recovery").any { origin == profile.getString(it) })
            require(url.path in setOf("/p03/device/start", "/p03/device/poll") && method == "POST" && token.isEmpty())
        }
        val connection = url.openConnection() as HttpsURLConnection
        try {
            connection.connectTimeout = 8000
            connection.readTimeout = 8000
            connection.instanceFollowRedirects = false
            connection.useCaches = false
            connection.requestMethod = method
            connection.setRequestProperty("Accept", "application/json")
            connection.setRequestProperty("Content-Type", "application/json")
            if (api && token.isNotEmpty()) connection.setRequestProperty("Authorization", "Bearer $token")
            if (body.isNotEmpty()) {
                require(method == "POST" || method == "PUT")
                val bytes = body.encodeToByteArray()
                require(bytes.size <= 8192)
                connection.doOutput = true
                connection.setFixedLengthStreamingMode(bytes.size)
                connection.outputStream.use { it.write(bytes) }
            }
            val status = connection.responseCode
            val output = ByteArrayOutputStream()
            if (status in 200..299 && status != 204) {
                connection.inputStream.use { input ->
                    val chunk = ByteArray(4096)
                    while (true) {
                        val length = input.read(chunk)
                        if (length < 0) break
                        require(output.size() + length <= 16384)
                        output.write(chunk, 0, length)
                    }
                }
            }
            return JSONObject().put("status", status).put("body", output.toString("UTF-8"))
        } finally {
            connection.disconnect()
        }
    }

    override fun invalidate() {
        generation.incrementAndGet()
        executor.execute { try { stopGoParticipant() } catch (_: UnsatisfiedLinkError) {} }
        executor.shutdown()
        super.invalidate()
    }
}
