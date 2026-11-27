package com.nu54renter.ble

import android.util.Base64
import org.conscrypt.Conscrypt
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.net.Socket
import java.nio.ByteBuffer
import java.security.KeyStore
import java.security.MessageDigest
import java.security.Principal
import java.security.PrivateKey
import java.security.cert.CertificateException
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLEngine
import javax.net.ssl.SSLEngineResult
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509ExtendedKeyManager
import javax.net.ssl.X509TrustManager

/** TLS 1.3 stream records travel in GATT writes/notifications; the GATT link is untrusted. */
internal class OwnerTls(
    owner: KeyStore.PrivateKeyEntry,
    rootText: String,
    certificateTexts: List<String>,
    private val expectedPeer: String
) {
    private val engine: SSLEngine
    private var incoming = byteArrayOf()
    private var epoch: ByteArray? = null
    val ready get() = epoch != null

    init {
        require(Regex("^[0-9a-f]{64}$").matches(expectedPeer))
        require(certificateTexts.size in 1..4)
        val factory = CertificateFactory.getInstance("X.509")
        fun certificate(text: String): X509Certificate {
            require(text.length in 1..12_288)
            return factory.generateCertificate(Base64.decode(text, Base64.NO_WRAP).inputStream()) as X509Certificate
        }
        val chain = certificateTexts.map(::certificate).toTypedArray()
        require(chain[0].publicKey.encoded.contentEquals(owner.certificate.publicKey.encoded))
        val trust = KeyStore.getInstance(KeyStore.getDefaultType()).apply {
            load(null)
            setCertificateEntry("enrolled-owner-root", certificate(rootText))
        }
        val owners = object : X509ExtendedKeyManager() {
            override fun getClientAliases(keyType: String?, issuers: Array<out Principal>?): Array<String>? =
                if (keyType == "EC") arrayOf("owner") else null
            override fun chooseClientAlias(keys: Array<out String>?, issuers: Array<out Principal>?, socket: Socket?) =
                if (keys?.contains("EC") == true) "owner" else null
            override fun chooseEngineClientAlias(keys: Array<out String>?, issuers: Array<out Principal>?, engine: SSLEngine?) =
                chooseClientAlias(keys, issuers, null)
            override fun getServerAliases(keyType: String?, issuers: Array<out Principal>?) = null
            override fun chooseServerAlias(keyType: String?, issuers: Array<out Principal>?, socket: Socket?) = null
            override fun getPrivateKey(alias: String?): PrivateKey? = if (alias=="owner") owner.privateKey else null
            override fun getCertificateChain(alias: String?) = if (alias=="owner") chain else null
        }
        val managers = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(trust) }
        val anchored = managers.trustManagers.filterIsInstance<X509TrustManager>().single()
        val pinned = object : X509TrustManager {
            override fun getAcceptedIssuers() = anchored.acceptedIssuers

            override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) =
                throw CertificateException("client trust is disabled")

            override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
                anchored.checkServerTrusted(chain, authType)
                val leaf = chain?.firstOrNull() ?: throw CertificateException("server leaf required")
                if (fingerprint(leaf) != expectedPeer) throw CertificateException("server pin mismatch")
            }
        }
        val context = SSLContext.getInstance("TLSv1.3", Conscrypt.newProvider()).apply {
            init(arrayOf(owners), arrayOf(pinned), null)
        }
        engine = context.createSSLEngine().apply {
            useClientMode = true
            enabledProtocols = arrayOf("TLSv1.3")
        }
        Conscrypt.setUseSessionTickets(engine, false)
        Conscrypt.setApplicationProtocols(engine, arrayOf(ALPN))
        engine.beginHandshake()
    }

    fun feed(data: ByteArray): String {
        require(incoming.size + data.size <= 49_152)
        incoming += data
        return pump(byteArrayOf())
    }

    fun send(data: ByteArray): String {
        require(ready && data.size in 1..3_072)
        return pump(data)
    }

    private fun pump(plaintext: ByteArray): String {
        val encryptedOut = ByteArrayOutputStream()
        val clearOut = ByteArrayOutputStream()
        val inBuffer = ByteBuffer.wrap(incoming)
        val app = ByteBuffer.wrap(plaintext)
        val output = ByteBuffer.allocate(32_768)
        var closed = false
        for (step in 0 until 96) {
            val state = engine.handshakeStatus
            output.clear()
            if (state == SSLEngineResult.HandshakeStatus.NEED_TASK) {
                requireNotNull(engine.delegatedTask).run()
                continue
            }
            val wrap = state == SSLEngineResult.HandshakeStatus.NEED_WRAP ||
                (state == SSLEngineResult.HandshakeStatus.NOT_HANDSHAKING && app.hasRemaining())
            val unwrap = !wrap && (inBuffer.hasRemaining() || state.name == "NEED_UNWRAP_AGAIN")
            if (!wrap && !unwrap) break
            val result = if (wrap) engine.wrap(app, output) else engine.unwrap(inBuffer, output)
            if (result.status == SSLEngineResult.Status.BUFFER_UNDERFLOW && !wrap) break
            require(result.status == SSLEngineResult.Status.OK || result.status == SSLEngineResult.Status.CLOSED)
            if (output.position() != 0) {
                val writer = if (wrap) encryptedOut else clearOut
                require(writer.size() + output.position() <= 49_152)
                writer.write(output.array(), 0, output.position())
            }
            if (result.status == SSLEngineResult.Status.CLOSED) {
                closed = true
                break
            }
            if (result.handshakeStatus == SSLEngineResult.HandshakeStatus.FINISHED) bind()
            if (result.bytesProduced() == 0 && result.bytesConsumed() == 0 && state == result.handshakeStatus) break
        }
        require(!app.hasRemaining())
        incoming = ByteArray(inBuffer.remaining()).also(inBuffer::get)
        require(ready || clearOut.size()==0)
        return JSONObject().put("out", encode(encryptedOut.toByteArray())).put("clear", encode(clearOut.toByteArray()))
            .put("closed", closed).put("epoch", epoch?.let(::encode) ?: "").put("peer", if (ready) expectedPeer else "").toString()
    }

    private fun bind() {
        val negotiated = Conscrypt.getApplicationProtocol(engine)
        require(epoch==null && engine.session.protocol=="TLSv1.3" && negotiated==ALPN)
        val peer = engine.session.peerCertificates.first() as X509Certificate
        require(fingerprint(peer) == expectedPeer)
        // The board uses the negotiated ALPN bytes as the wire2 exporter context.
        epoch = requireNotNull(Conscrypt.exportKeyingMaterial(engine,"EXPORTER-NU54-wire2",
            negotiated.toByteArray(Charsets.US_ASCII),16))
        require(epoch!!.size==16)
    }

    private fun fingerprint(certificate: X509Certificate): String = MessageDigest.getInstance("SHA-256")
        .digest(certificate.publicKey.encoded).joinToString("") { "%02x".format(it) }

    private fun encode(value: ByteArray): String = Base64.encodeToString(value, Base64.NO_WRAP)

    companion object { private const val ALPN = "nu54-v2-management" }
}
