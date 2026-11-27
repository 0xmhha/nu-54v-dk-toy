package com.nu54renter.ble

import android.app.Activity
import android.app.Instrumentation
import android.os.Bundle
import com.nu54.protocol.CommerceHttp
import org.json.JSONObject
import javax.net.ssl.SSLException

/**
 * Runs the shipping customer entrance transport against a disposable local PKI (test_customer_entrance.sh).
 * No enrollment profile is configured: the trust comes from the anchor alone.
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
            entrance(CommerceHttp(targetContext, setOf("customer")), result)
            finish(Activity.RESULT_OK, result)
        } catch (error: Throwable) {
            result.putString("customer_entrance_lab", "fail")
            result.putString("error", error.stackTraceToString())
            finish(Activity.RESULT_CANCELED, result)
        }
    }

    private fun entrance(c3: CommerceHttp, result: Bundle) {
        val port = required("port").toInt()
        val origin = "https://127.0.0.1:$port"
        val ca = required("ca")
        val pin = required("pin")
        fun anchor(origin_: String = origin, ca_: String = ca, pin_: String = pin) =
            JSONObject().put("origin", origin_).put("ca", ca_).put("pin", pin_).toString()
        val body = "{\"synthetic\":true}"
        val challenge = "/v2/customer/challenge"
        val session = "5e".repeat(32)

        // The lab server asks for a client certificate and reports whether it got one.
        val answer = JSONObject(c3.customerEntrance(anchor(), challenge, "", body))
        check(answer.getInt("status") == 200) { "unexpected status" }
        val seen = JSONObject(answer.getString("body"))
        check(!seen.getBoolean("clientCertificate")) { "a client certificate was offered" }
        check(seen.getString("tls") == "TLSv1.3") { "not TLS 1.3" }
        result.putString("entrance", "pass")
        result.putString("entrance_client_certificate", "none")

        // A session request carries its token; the store list is read with GET.
        val commerce = JSONObject(JSONObject(c3.customerEntrance(anchor(), "/v2/customer/commerce", session, body)).getString("body"))
        check(commerce.getString("authorization") == "Bearer $session") { "the session token was not sent" }
        val stores = JSONObject(JSONObject(c3.customerEntrance(anchor(), "/v2/customer/stores", "", "")).getString("body"))
        check(stores.getString("method") == "GET" && stores.getString("authorization").isEmpty()) { "the store list was not a plain GET" }
        result.putString("entrance_session", "pass")

        // A refusal keeps its status and body.
        val busy = JSONObject(c3.customerEntrance(anchor(), "/v2/customer/proof", "", "{\"busy\":true}"))
        check(busy.getInt("status") == 429 && JSONObject(busy.getString("body")).getString("error") == "BUSY")

        // A redirect is not followed and is not an answer.
        check(refused { c3.customerEntrance(anchor(), "/v2/customer/proof", "", "{\"moved\":true}") } is IllegalArgumentException)
        result.putString("entrance_redirect", "reject")

        // The right CA with another leaf key pinned.
        check(refused { c3.customerEntrance(anchor(pin_ = "00".repeat(32)), challenge, "", body) } is SSLException) { "wrong pin accepted" }
        result.putString("entrance_pin", "reject")

        // The current key and the next one: the server may carry either. Two wrong keys, the same key twice, three keys
        // and no key are no anchor.
        fun pinned(vararg pins: String) =
            JSONObject().put("origin", origin).put("ca", ca).put("pin", org.json.JSONArray(pins.toList())).toString()
        check(JSONObject(c3.customerEntrance(pinned("00".repeat(32), pin), challenge, "", body)).getInt("status") == 200) { "the second pin was refused" }
        check(JSONObject(c3.customerEntrance(pinned(pin), challenge, "", body)).getInt("status") == 200) { "a list of one pin was refused" }
        check(refused { c3.customerEntrance(pinned("00".repeat(32), "11".repeat(32)), challenge, "", body) } is SSLException) { "two wrong pins accepted" }
        check(refused { c3.customerEntrance(pinned(pin, pin), challenge, "", body) } is IllegalArgumentException)
        check(refused { c3.customerEntrance(pinned(pin, "00".repeat(32), "11".repeat(32)), challenge, "", body) } is IllegalArgumentException)
        check(refused { c3.customerEntrance(pinned(), challenge, "", body) } is IllegalArgumentException)
        result.putString("entrance_pins", "pass")

        // The right pin under a CA that did not issue the server certificate.
        check(refused { c3.customerEntrance(anchor(ca_ = required("otherCa")), challenge, "", body) } is SSLException) { "foreign CA accepted" }
        result.putString("entrance_ca", "reject")

        // The certificate names 127.0.0.1 only.
        check(refused { c3.customerEntrance(anchor("https://localhost:$port"), challenge, "", body) } is SSLException) { "wrong host accepted" }
        result.putString("entrance_host", "reject")

        // Nothing is sent to a plain origin, an origin with a path, a path outside the entrance, an oversized body,
        // a session where none belongs, or a commerce request without one.
        check(refused { c3.customerEntrance(anchor("http://127.0.0.1:$port"), challenge, "", body) } is IllegalArgumentException)
        check(refused { c3.customerEntrance(anchor("$origin/v2"), challenge, "", body) } is IllegalArgumentException)
        check(refused { c3.customerEntrance(anchor(), "/v2/commerce", "", body) } is IllegalArgumentException)
        // A store's menu page and a menu photo part are public reads; nothing else under a store is a path.
        val store = "/v2/customer/stores/0fbc54ef-2827-4ab5-bfcd-3a27611c3002"
        for (path in listOf("$store/menu", "$store/menu/01", "$store/menu/-1", "$store/menu/0?page=1", "$store/menu/0/0", "$store/orders",
            "$store/photos/${"a".repeat(64)}", "$store/photos/${"A".repeat(64)}/0", "$store/photos/${"a".repeat(64)}/00"))
            check(refused { c3.customerEntrance(anchor(), path, "", "") } is IllegalArgumentException) { "$path was sent" }
        check(refused { c3.customerEntrance(anchor(), "$store/menu/0", "", "{}") } is IllegalArgumentException) { "a menu read carried a body" }
        check(refused { c3.customerEntrance(anchor(), "$store/menu/0", "ab".repeat(32), "") } is IllegalArgumentException) { "a menu read carried a session" }
        check(refused { c3.customerEntrance(anchor(), challenge, "", "{\"pad\":\"" + "x".repeat(1024) + "\"}") } is IllegalArgumentException)
        check(refused { c3.customerEntrance(anchor(), challenge, session, body) } is IllegalArgumentException)
        check(refused { c3.customerEntrance(anchor(), "/v2/customer/commerce", "", body) } is IllegalArgumentException)
        check(refused { c3.customerEntrance(anchor(), "/v2/customer/stores", "", body) } is IllegalArgumentException)
        check(refused { c3.customerEntrance(anchor(), "/v2/customer/stores/../commerce", "", "") } is IllegalArgumentException)
        result.putString("entrance_scope", "reject")

        // Order sessions are kept in the sealed journal under their own name; a name outside the list is refused.
        val sessions = "customer-sessions-" + "ab".repeat(32)
        c3.saveJournal(sessions, "[]")
        check(c3.loadJournal(sessions) == "[]") { "the order sessions were not kept" }
        c3.saveJournal(sessions, "")
        check(refused { c3.saveJournal("customer-session-" + "ab".repeat(32), "[]") } is IllegalArgumentException)
        check(refused { c3.loadJournal(sessions + "\n") } is IllegalArgumentException)
        result.putString("entrance_journal", "pass")
        result.putString("customer_entrance_lab", "pass")
    }

    private fun refused(call: () -> Any?): Throwable =
        runCatching(call).exceptionOrNull() ?: error("a refused request was answered")

    private fun required(name: String) = requireNotNull(arguments.getString(name))
}
