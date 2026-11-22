package com.nu54renter.ble

import android.app.Activity
import android.app.Instrumentation
import android.os.Bundle
import android.util.Base64
import com.nu54.protocol.NearbyBluetooth
import com.nu54.protocol.NearbyCandidates
import com.nu54.protocol.NearbyPeer
import com.nu54.protocol.NearbyTls
import java.security.KeyStore
import java.security.MessageDigest
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.util.ArrayDeque

/** Disposable AVD entry. The TLS engines and key generation are the shipping SDK implementation. */
class NearbyTlsLab : Instrumentation() {
    private lateinit var arguments: Bundle
    private val branch = "b0000000-0000-4000-8000-000000000001"
    private val customer = "c0000000-0000-4000-8000-000000000001"
    private val terminal = "a0000000-0000-4000-8000-000000000001"
    private val expiry get() = System.currentTimeMillis() / 1000 + 600
    override fun onCreate(arguments: Bundle) {
        this.arguments = arguments
        start()
    }

    override fun onStart() {
        val state = Bundle()
        try {
            if (arguments.getString("phase") == "identity") {
                state.putString("customer", NearbyBluetooth(targetContext, false) {}.publicIdentity())
                state.putString("customer2", secondaryIdentity())
                state.putString("terminal", NearbyBluetooth(targetContext, true) {}.publicIdentity())
                state.putString("payment", NearbyBluetooth(targetContext, false, payment = true) {}.publicIdentity())
            } else if (arguments.getString("phase") == "payment") {
                state.putString("approval", payment())
            } else exercise()
            state.putString("nearby_tls_lab", "pass")
            finish(Activity.RESULT_OK, state)
        } catch (failure: Throwable) {
            state.putString("nearby_tls_lab", "fail")
            state.putString("error", failure.stackTraceToString())
            finish(Activity.RESULT_CANCELED, state)
        }
    }

    private fun required(key: String) = requireNotNull(arguments.getString(key))
    private fun bytes(raw: String) = Base64.decode(raw, Base64.NO_WRAP)
    private fun fingerprint(raw: String): String {
        val certificate = CertificateFactory.getInstance("X.509").generateCertificate(bytes(raw).inputStream()) as X509Certificate
        return MessageDigest.getInstance("SHA-256").digest(certificate.publicKey.encoded).joinToString("") { "%02x".format(it) }
    }

    private fun key(alias: String): KeyStore.PrivateKeyEntry =
        KeyStore.getInstance("AndroidKeyStore").apply { load(null) }.getEntry(alias, null) as KeyStore.PrivateKeyEntry

    private fun secondaryIdentity(): String {
        val alias = "nu54-nearby-disposable-customer2"
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        if (!store.containsAlias(alias)) {
            java.security.KeyPairGenerator.getInstance("EC", "AndroidKeyStore").apply {
                initialize(android.security.keystore.KeyGenParameterSpec.Builder(alias, android.security.keystore.KeyProperties.PURPOSE_SIGN)
                    .setAlgorithmParameterSpec(java.security.spec.ECGenParameterSpec("secp256r1"))
                    .setDigests(android.security.keystore.KeyProperties.DIGEST_NONE).build())
            }.generateKeyPair()
        }
        return Base64.encodeToString(store.getCertificate(alias).publicKey.encoded, Base64.NO_WRAP)
    }

    private fun engine(client: Boolean, local: String = required(if (client) "customer" else "terminal"),
                       peer: String = required(if (client) "terminal" else "customer"), pin: String = fingerprint(peer),
                       end: Long = expiry, payment: Boolean = false, alternate: Boolean = false): NearbyTls {
        val role = if (client && !payment) "customer" else "terminal"
        val alias = if (payment) "nu54-payment-terminal-v3" else if (client && alternate) "nu54-nearby-disposable-customer2" else "nu54-nearby-$role-v3"
        val principal = if (client) terminal else if (alternate) "c0000000-0000-4000-8000-000000000002" else customer
        return NearbyTls(key(alias), required("ca"), listOf(local), role,
            mapOf(pin to NearbyPeer(pin, principal, branch, end)), expiry, payment)
    }

    private class Link(val client: NearbyTls, val server: NearbyTls) {
        var clientEpoch: ByteArray? = null
        var serverEpoch: ByteArray? = null
        var started = false

        fun handshake() {
            val toServer = ArrayDeque<ByteArray>()
            val toClient = ArrayDeque<ByteArray>()
            fun enqueue(destination: ArrayDeque<ByteArray>, encrypted: ByteArray, size: Int) {
                for (i in encrypted.indices step size)
                    destination.add(encrypted.copyOfRange(i, minOf(i + size, encrypted.size)))
            }
            enqueue(toServer, client.begin().encrypted, 20)
            for (i in 0..2000) {
                if (toServer.isNotEmpty()) {
                    val packet = server.feed(toServer.remove())
                    check(packet.clear.isEmpty())
                    enqueue(toClient, packet.encrypted, 137)
                    serverEpoch = packet.epoch ?: serverEpoch
                }
                if (toClient.isNotEmpty()) {
                    val packet = client.feed(toClient.remove())
                    check(packet.clear.isEmpty())
                    enqueue(toServer, packet.encrypted, 20)
                    clientEpoch = packet.epoch ?: clientEpoch
                }
                if (toServer.isEmpty() && toClient.isEmpty()) {
                    check(clientEpoch?.size == 16 && clientEpoch?.contentEquals(serverEpoch) == true)
                    started = true
                    return
                }
            }
            error("handshake did not complete")
        }

        fun send(value: ByteArray, width: Int, clientWrites: Boolean): ByteArray {
            check(started)
            val sender = if (clientWrites) client else server
            val receiver = if (clientWrites) server else client
            val data = sender.send(value).encrypted
            val output = java.io.ByteArrayOutputStream()
            for (i in data.indices step width) {
                val packet = receiver.feed(data.copyOfRange(i, minOf(i + width, data.size)))
                check(packet.encrypted.isEmpty())
                output.write(packet.clear)
            }
            return output.toByteArray()
        }
    }

    private fun exercise() {
        var time = 5L
        val candidates = NearbyCandidates<String> { time }
        candidates.add("private Bluetooth identifier")
        val old = candidates.finish().single()
        time += 20_000
        check(candidates.take(old) == null)
        candidates.add("private Bluetooth identifier")
        val fresh = candidates.finish().single()
        check(fresh != old && candidates.take(old) == null && candidates.take(fresh) != null)
        check(candidates.take(fresh) == null)
        candidates.reset()
        candidates.add("first terminal")
        candidates.add("second terminal")
        val (first, second) = candidates.finish().toList()
        check(candidates.take(first) == "first terminal")
        check(candidates.take(first) == null)
        check(candidates.take(second) == "second terminal")
        candidates.reset()
        candidates.add("first board")
        candidates.add("second board")
        val (board, alternate) = candidates.finish().toList()
        check(candidates.take(board, clearBatch = true) == "first board")
        check(candidates.take(alternate) == null)

        val left = Link(engine(true), engine(false))
        val right = Link(engine(true, local = required("customer2"), alternate = true),
            engine(false, peer = required("customer2"), alternate = true))
        left.handshake()
        right.handshake()
        check(!left.clientEpoch!!.contentEquals(right.clientEpoch))
        repeat(600) { i ->
            val size = 2 + (i % 240)
            val frame = ByteArray(size) { (i + it).toByte() }
            frame[0] = ((size - 2) shr 8).toByte()
            frame[1] = (size - 2).toByte()
            val other = frame.copyOf().apply { this[lastIndex] = (this[lastIndex] + 1).toByte() }
            check(left.send(frame, 20, i % 2 == 0).contentEquals(frame))
            check(right.send(other, 244, i % 2 == 1).contentEquals(other))
        }
        check(runCatching { engine(true, local = required("customer2")) }.isFailure)
        check(runCatching { Link(engine(true, pin = "01".repeat(32)), engine(false)).handshake() }.isFailure)
        check(runCatching { Link(engine(true, end = 1), engine(false)).handshake() }.isFailure)
        check(runCatching { Link(engine(true), engine(false, peer = required("terminal"))).handshake() }.isFailure)
        check(runCatching {
            Link(engine(true, local = required("payment"), payment = true), engine(false)).handshake()
        }.isFailure)
        val forged = left.client.send(byteArrayOf(0, 1, 7)).encrypted.apply {
            this[lastIndex] = (this[lastIndex].toInt() xor 1).toByte()
        }
        check(runCatching { left.server.feed(forged) }.isFailure)
        check(right.send(byteArrayOf(0, 1, 9), 20, true).contentEquals(byteArrayOf(0, 1, 9)))
    }

    private fun payment(): String {
        val tls = engine(true, local = required("payment"), peer = required("board"), payment = true)
        val quote = bytes(required("quote"))
        check(quote.size == 737)
        val expires = required("expires").toLong()
        val requestId = ByteArray(16) { 7 }
        java.net.Socket("127.0.0.1", required("port").toInt()).use { socket ->
            socket.soTimeout = 12_000
            val data = ByteArray(137)
            var packet = tls.begin()
            var pending = byteArrayOf()
            var sent = false
            var fragment = 0
            var parts = 0
            val body = java.io.ByteArrayOutputStream()
            var session: ByteArray? = null
            repeat(1000) {
                if (packet.encrypted.isNotEmpty()) socket.getOutputStream().write(packet.encrypted)
                if (packet.epoch != null) session = packet.epoch
                pending += packet.clear
                while (pending.size >= 2) {
                    val count = ((pending[0].toInt() and 255) shl 8) or (pending[1].toInt() and 255)
                    check(count in 23..214)
                    if (pending.size < count + 2) break
                    val frame = pending.copyOfRange(2, 2 + count)
                    pending = pending.copyOfRange(2 + count, pending.size)
                    check(frame.copyOfRange(0, 16).contentEquals(session))
                    check(frame.sliceArray(16..19).all { it == 0.toByte() })
                    if (fragment == 0) parts = frame[21].toInt() and 255
                    check(frame[20].toInt() == fragment++ && frame[21].toInt() == parts)
                    body.write(frame.copyOfRange(22, frame.size))
                    if (fragment == parts) {
                        val response = java.nio.ByteBuffer.wrap(body.toByteArray())
                        check(response.get().toInt() == 5)
                        check(ByteArray(16).also { response.get(it) }.contentEquals(requestId))
                        check(response.long == expires)
                        return Base64.encodeToString(ByteArray(response.remaining()).also { response.get(it) }, Base64.NO_WRAP)
                    }
                }
                packet = if (!sent && tls.ready) {
                    sent = true
                    val clear = java.nio.ByteBuffer.allocate(25 + quote.size).put(2).put(requestId).putLong(expires).put(quote).array()
                    val stream = java.io.ByteArrayOutputStream()
                    val total = (clear.size + 191) / 192
                    for (i in 0 until total) {
                        val piece = clear.copyOfRange(i * 192, minOf((i + 1) * 192, clear.size))
                        val wire = java.nio.ByteBuffer.allocate(24 + piece.size).putShort((22 + piece.size).toShort())
                            .put(requireNotNull(session)).putInt(0).put(i.toByte()).put(total.toByte()).put(piece)
                        stream.write(wire.array())
                    }
                    tls.send(stream.toByteArray())
                } else {
                    val used = socket.getInputStream().read(data)
                    check(used > 0)
                    tls.feed(data.copyOf(used))
                }
            }
        }
        error("payment wire did not complete")
    }
}
