package com.nu54.protocol

import android.util.Base64
import org.conscrypt.Conscrypt
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
internal class NearbyTls(
    owner: KeyStore.PrivateKeyEntry,
    rootText: String,
    certificateTexts: List<String>,
    private val localRole: String,
    private val peers: Map<String, NearbyPeer>,
    private val expiresAt: Long,
    private val payment: Boolean = false
) {
    private val engine: SSLEngine
    private var incoming = byteArrayOf()
    private var epoch: ByteArray? = null
    val ready get() = epoch != null
    private var selected: NearbyPeer? = null
    private val remoteRole = if (payment) "board" else if (localRole == "customer") "terminal" else "customer"
    private val client = payment || localRole == "customer"
    private val alpn = if (payment) "nu54-v2-payment" else "nu54-v2-nearby"

    init {
        require(peers.isNotEmpty())
        require(certificateTexts.size in 1..4)
        val factory = CertificateFactory.getInstance("X.509")
        fun certificate(text: String): X509Certificate {
            require(text.length in 1..12_288)
            return factory.generateCertificate(Base64.decode(text, Base64.NO_WRAP).inputStream()) as X509Certificate
        }
        val chain = certificateTexts.map(::certificate).toTypedArray()
        require(chain[0].publicKey.encoded.contentEquals(owner.certificate.publicKey.encoded))
        require(role(chain[0], localRole))
        chain[0].checkValidity()
        val trust = KeyStore.getInstance(KeyStore.getDefaultType()).apply {
            load(null)
            setCertificateEntry("enrolled-owner-root", certificate(rootText))
        }
        val owners = object : X509ExtendedKeyManager() {
            override fun getClientAliases(keyType: String?, issuers: Array<out Principal>?): Array<String>? =
                if (client && keyType.startsWithEc()) arrayOf("local") else null
            override fun chooseClientAlias(keys: Array<out String>?, issuers: Array<out Principal>?, socket: Socket?) =
                if (client && keys?.any { it.startsWithEc() } == true) "local" else null
            override fun chooseEngineClientAlias(keys: Array<out String>?, issuers: Array<out Principal>?, engine: SSLEngine?) =
                chooseClientAlias(keys, issuers, null)
            override fun getServerAliases(keyType: String?, issuers: Array<out Principal>?) =
                if (!client && keyType.startsWithEc()) arrayOf("local") else null
            override fun chooseServerAlias(keyType: String?, issuers: Array<out Principal>?, socket: Socket?) =
                if (!client && keyType.startsWithEc()) "local" else null
            override fun chooseEngineServerAlias(keyType: String?, issuers: Array<out Principal>?, engine: SSLEngine?) =
                chooseServerAlias(keyType, issuers, null)
            override fun getPrivateKey(alias: String?): PrivateKey? = if (alias=="local") owner.privateKey else null
            override fun getCertificateChain(alias: String?) = if (alias=="local") chain else null
        }
        val managers = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply { init(trust) }
        val anchored = managers.trustManagers.filterIsInstance<X509TrustManager>().single()
        val pinned = object : X509TrustManager {
            override fun getAcceptedIssuers() = anchored.acceptedIssuers

            override fun checkClientTrusted(chain: Array<out X509Certificate>?, authType: String?) {
                if (client) throw CertificateException("wrong role")
                anchored.checkClientTrusted(chain, authType)
                enroll(chain)
            }
            override fun checkServerTrusted(chain: Array<out X509Certificate>?, authType: String?) {
                if (!client) throw CertificateException("wrong role")
                anchored.checkServerTrusted(chain, authType)
                enroll(chain)
            }
            private fun enroll(chain: Array<out X509Certificate>?) {
                val leaf = chain?.firstOrNull() ?: throw CertificateException("leaf required")
                if (!role(leaf, remoteRole) || peers[fingerprint(leaf)]?.current() != true)
                    throw CertificateException("unregistered peer")
            }
        }
        val context = SSLContext.getInstance("TLSv1.3", Conscrypt.newProvider()).apply {
            init(arrayOf(owners), arrayOf(pinned), null)
        }
        engine = context.createSSLEngine().apply {
            useClientMode = client
            if (!client) needClientAuth = true
            enabledProtocols = arrayOf("TLSv1.3")
        }
        Conscrypt.setUseSessionTickets(engine, false)
        Conscrypt.setApplicationProtocols(engine, arrayOf(alpn))
        if (!client) Conscrypt.setApplicationProtocolSelector(engine, object : org.conscrypt.ApplicationProtocolSelector() {
            override fun selectApplicationProtocol(engine: SSLEngine?, protocols: List<String>?) =
                alpn.takeIf { protocols?.contains(it) == true }
            override fun selectApplicationProtocol(socket: javax.net.ssl.SSLSocket?, protocols: List<String>?) =
                alpn.takeIf { protocols?.contains(it) == true }
        })
        engine.beginHandshake()
    }

    fun begin() = pump(byteArrayOf())

    fun feed(data: ByteArray): NearbyPacket {
        require(incoming.size + data.size <= 49_152)
        incoming += data
        return pump(byteArrayOf())
    }

    fun send(data: ByteArray): NearbyPacket {
        require(ready && data.size in 1..3_072)
        return pump(data)
    }

    private fun pump(plaintext: ByteArray): NearbyPacket {
        require(System.currentTimeMillis() / 1000 < expiresAt && selected?.current() != false)
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
        return NearbyPacket(encryptedOut.toByteArray(), clearOut.toByteArray(), epoch, selected, closed)
    }

    private fun bind() {
        require(epoch==null && engine.session.protocol=="TLSv1.3" && Conscrypt.getApplicationProtocol(engine)==alpn)
        val peer = engine.session.peerCertificates.first() as X509Certificate
        selected = requireNotNull(peers[fingerprint(peer)]).also { require(it.current() && role(peer, remoteRole)) }
        epoch = requireNotNull(Conscrypt.exportKeyingMaterial(engine,"EXPORTER-NU54-wire2",alpn.toByteArray(Charsets.US_ASCII),16))
        require(epoch!!.size==16)
    }

    private fun fingerprint(certificate: X509Certificate): String = MessageDigest.getInstance("SHA-256")
        .digest(certificate.publicKey.encoded).joinToString("") { "%02x".format(it) }

    private fun role(certificate: X509Certificate, role: String): Boolean =
        certificate.subjectAlternativeNames?.singleOrNull()?.let { it[0] == 6 && it[1] == "urn:nu54:role:$role" } == true

    private fun String?.startsWithEc() = this == "EC" || this?.startsWith("EC_") == true

}

internal data class NearbyPeer(val pin: String, val principal: String, val branch: String, val expiresAt: Long) {
    fun current() = System.currentTimeMillis() / 1000 < expiresAt
}

internal data class NearbyPacket(val encrypted: ByteArray, val clear: ByteArray, val epoch: ByteArray?,
                                 val peer: NearbyPeer?, val closed: Boolean)
