package com.nu54renter.ble

import android.app.Activity
import android.app.Instrumentation
import android.os.Bundle
import android.util.Base64
import com.nu54.protocol.CommerceHttp
import org.json.JSONObject
import java.io.ByteArrayOutputStream
import java.net.Socket
import java.nio.ByteBuffer
import java.security.SecureRandom
import java.security.KeyFactory
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import java.security.spec.PKCS8EncodedKeySpec
import javax.net.ssl.SSLException

/** Runs the shipping OwnerTls transport on Android with a disposable local PKI. */
class OwnerTlsLab : Instrumentation() {
    private lateinit var arguments: Bundle

    override fun onCreate(arguments: Bundle) {
        this.arguments = arguments
        start()
    }

    override fun onStart() {
        val result = Bundle()
        try {
            val mode=arguments.getString("lab")
            if(mode=="c3-key"||mode=="c3-exchange"){
                commerce(mode,result)
                finish(Activity.RESULT_OK,result)
                return
            }
            val port = required("port").toInt()
            val ca = required("ca")
            val cert = required("cert")
            val pin = required("pin")
            val parser = CertificateFactory.getInstance("X.509")
            val leaf = parser.generateCertificate(decode(cert).inputStream()) as X509Certificate
            val privateKey = KeyFactory.getInstance("EC").generatePrivate(PKCS8EncodedKeySpec(decode(required("priv"))))
            val owner = KeyStore.PrivateKeyEntry(privateKey, arrayOf(leaf))
            val good = managementStatus(port, OwnerTls(owner, ca, listOf(cert), pin), alterEpoch = false)
            check(good) { "C board rejected the ALPN-bound owner epoch" }
            val wrong = managementStatus(port, OwnerTls(owner, ca, listOf(cert), pin), alterEpoch = true)
            check(!wrong) { "C board accepted a mismatched wire2 epoch" }
            try {
                managementStatus(port, OwnerTls(owner, ca, listOf(cert), "01".repeat(32)), alterEpoch = false)
                error("wrong board pin accepted")
            } catch (_: SSLException) {
                // Expected trust failure from the actual Conscrypt handshake.
            }
            result.putString("ng", "wrong-pin-rejected")
            result.putString("epoch", "alpn-matched-and-mismatch-rejected")
            result.putString("owner_tls_lab", "pass")
            finish(Activity.RESULT_OK, result)
        } catch (error: Throwable) {
            result.putString("owner_tls_lab", "fail")
            result.putString("error", error.stackTraceToString())
            finish(Activity.RESULT_CANCELED, result)
        }
    }

    private fun commerce(mode:String,result:Bundle){
        val c3=CommerceHttp(targetContext,setOf("customer"))
        if(mode=="c3-key"){
            result.putString("commerce_spki",c3.publicIdentity("customer"))
            return
        }
        val raw=decode(required("profile")).toString(Charsets.UTF_8)
        c3.configure("customer",raw)
        check(c3.load("customer")==raw)
        val answer=c3.exchange("customer","{\"synthetic\":true}")
        check(answer=="{\"localTls13\":true}")
        val denied=JSONObject(raw).put("pin","00".repeat(32)).toString()
        c3.configure("customer",denied)
        val failed=runCatching{c3.exchange("customer","{\"synthetic\":true}")}.exceptionOrNull()
        check(failed is SSLException)
        c3.configure("customer",raw)
        c3.saveJournal("customer-qr-v2","{\"synthetic\":true}")
        check(c3.loadJournal("customer-qr-v2")=="{\"synthetic\":true}")
        c3.saveJournal("customer-qr-v2","")
        check(c3.loadJournal("customer-qr-v2").isEmpty())
        result.putString("commerce","pass")
        result.putString("commerce_pin","reject")
    }

    private fun managementStatus(port: Int, transport: OwnerTls, alterEpoch: Boolean): Boolean {
        val id = ByteArray(16).also(SecureRandom()::nextBytes)
        val expires = System.currentTimeMillis() / 1000 + 30
        Socket("127.0.0.1", port).use { socket ->
            socket.soTimeout = 10_000
            val buffer = ByteArray(137)
            val answer = ByteArrayOutputStream()
            var result = JSONObject(transport.feed(byteArrayOf()))
            var sentEpoch: ByteArray? = null
            repeat(2000) {
                val outbound = decode(result.getString("out"))
                if (outbound.isNotEmpty()) socket.getOutputStream().write(outbound)
                answer.write(decode(result.getString("clear")))
                if (answer.size() >= 2) {
                    val reply = ByteBuffer.wrap(answer.toByteArray())
                    val size = reply.short.toInt()
                    check(size == 124) { "unexpected owner status frame size" }
                    if (reply.remaining() == size) {
                        val epoch = ByteArray(16).also(reply::get)
                        check(epoch.contentEquals(sentEpoch))
                        check(reply.int == 0 && reply.get() == 0.toByte() && reply.get() == 1.toByte())
                        check(reply.get() == 5.toByte() && ByteArray(16).also(reply::get).contentEquals(id))
                        check(reply.long == expires && reply.get() == 3.toByte() && reply.get() == 0.toByte())
                        return true
                    }
                }
                if (transport.ready && sentEpoch == null) {
                    val epoch = decode(result.getString("epoch"))
                    check(epoch.size == 16 && epoch.any { it != 0.toByte() })
                    if (alterEpoch) epoch[0] = (epoch[0].toInt() xor 1).toByte()
                    sentEpoch = epoch
                    // Wire2 stream length, epoch, sequence and single-fragment header.
                    val status = ByteBuffer.allocate(50)
                    status.putShort(48).put(epoch).putInt(0).put(0).put(1)
                    // Management command, request metadata, then the owner STATUS opcode.
                    status.put(1).put(id).putLong(expires).put(3)
                    result = JSONObject(transport.send(status.array()))
                } else {
                    val length = socket.getInputStream().read(buffer)
                    if (length == -1) return false
                    result = JSONObject(transport.feed(buffer.copyOf(length)))
                }
            }
        }
        error("management status exceeded the bounded packet count")
    }

    private fun required(name: String) = requireNotNull(arguments.getString(name))
    private fun decode(value: String) = Base64.decode(value, Base64.NO_WRAP)
}
