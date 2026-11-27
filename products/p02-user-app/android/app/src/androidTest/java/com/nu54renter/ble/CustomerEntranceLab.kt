package com.nu54renter.ble

import android.app.Activity
import android.app.Instrumentation
import android.os.Bundle
import android.util.Base64
import com.nu54.protocol.CommerceHttp
import org.json.JSONObject
import javax.net.ssl.SSLException

/**
 * Runs the shipping customer entrance transport against a disposable local PKI (test_customer_entrance.sh).
 * Selected with -Pnu54LabRunner=com.nu54renter.ble.CustomerEntranceLab.
 */
class CustomerEntranceLab : Instrumentation() {
    private lateinit var arguments: Bundle

    override fun onCreate(arguments: Bundle) {
        this.arguments = arguments
        start()
    }

    override fun onStart() {
        val result = Bundle()
        try {
            val c3 = CommerceHttp(targetContext, setOf("customer"))
            if (arguments.getString("lab") == "key") {
                result.putString("commerce_spki", c3.publicIdentity("customer"))
            } else {
                entrance(c3, result)
            }
            finish(Activity.RESULT_OK, result)
        } catch (error: Throwable) {
            result.putString("customer_entrance_lab", "fail")
            result.putString("error", error.stackTraceToString())
            finish(Activity.RESULT_CANCELED, result)
        }
    }

    private fun entrance(c3: CommerceHttp, result: Bundle) {
        val raw = decode(required("profile")).toString(Charsets.UTF_8)
        val port = required("port").toInt()
        val origin = "https://127.0.0.1:$port"
        val body = "{\"synthetic\":true}"
        val challenge = "/v2/customer/challenge"
        c3.configure("customer", raw)

        // The lab server asks for a client certificate and reports whether it got one.
        val answer = JSONObject(c3.customerEntrance(origin, challenge, body))
        check(answer.getInt("status") == 200) { "unexpected status" }
        val seen = JSONObject(answer.getString("body"))
        check(!seen.getBoolean("clientCertificate")) { "a client certificate was offered" }
        check(seen.getString("tls") == "TLSv1.3") { "not TLS 1.3" }
        result.putString("entrance", "pass")
        result.putString("entrance_client_certificate", "none")

        // A refusal keeps its status and body.
        val busy = JSONObject(c3.customerEntrance(origin, "/v2/customer/proof", "{\"busy\":true}"))
        check(busy.getInt("status") == 429 && JSONObject(busy.getString("body")).getString("error") == "BUSY")

        // A redirect is not followed and is not an answer.
        check(refused { c3.customerEntrance(origin, "/v2/customer/proof", "{\"moved\":true}") } is IllegalArgumentException)
        result.putString("entrance_redirect", "reject")

        // The right CA with another leaf key pinned.
        c3.configure("customer", JSONObject(raw).put("pin", "00".repeat(32)).toString())
        check(refused { c3.customerEntrance(origin, challenge, body) } is SSLException) { "wrong pin accepted" }
        result.putString("entrance_pin", "reject")

        // The right pin under a CA that did not issue the server certificate.
        c3.configure("customer", JSONObject(raw).put("ca", required("otherCa")).toString())
        check(refused { c3.customerEntrance(origin, challenge, body) } is SSLException) { "foreign CA accepted" }
        result.putString("entrance_ca", "reject")
        c3.configure("customer", raw)

        // The certificate names 127.0.0.1 only.
        check(refused { c3.customerEntrance("https://localhost:$port", challenge, body) } is SSLException) { "wrong host accepted" }
        result.putString("entrance_host", "reject")

        // Nothing is sent to a plain origin, an origin with a path, or a path outside the entrance.
        check(refused { c3.customerEntrance("http://127.0.0.1:$port", challenge, body) } is IllegalArgumentException)
        check(refused { c3.customerEntrance("$origin/v2", challenge, body) } is IllegalArgumentException)
        check(refused { c3.customerEntrance(origin, "/v2/commerce", body) } is IllegalArgumentException)
        check(refused { c3.customerEntrance(origin, challenge, "{\"pad\":\"" + "x".repeat(1024) + "\"}") } is IllegalArgumentException)
        result.putString("entrance_scope", "reject")
        result.putString("customer_entrance_lab", "pass")
    }

    private fun refused(call: () -> Any?): Throwable =
        runCatching(call).exceptionOrNull() ?: error("a refused request was answered")

    private fun required(name: String) = requireNotNull(arguments.getString(name))
    private fun decode(value: String) = Base64.decode(value, Base64.NO_WRAP)
}
