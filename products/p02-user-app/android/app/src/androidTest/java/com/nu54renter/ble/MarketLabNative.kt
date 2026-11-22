package com.nu54renter.ble

import android.app.Activity
import android.app.Instrumentation
import android.os.Bundle
import android.util.Base64
import com.facebook.react.bridge.Callback
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.PromiseImpl
import com.facebook.react.bridge.BridgeReactContext
import org.json.JSONArray
import org.json.JSONObject
import java.math.BigInteger
import java.net.HttpURLConnection
import java.net.Proxy
import java.net.URL
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit

/** Uses the shipping Android module, AndroidKeyStore, JNI, and loopback market service. */
class MarketLabNative : Instrumentation() {
    override fun onCreate(arguments: Bundle) { start() }

    override fun onStart() {
        val details = Bundle()
        val module = MarketLabModule(BridgeReactContext(targetContext))
        try {
            exercise(module)
            details.putString("market_lab", "pass")
            details.putString("native_guard", "wrong-scope-tampering-target-route-and-future-head-rejected")
            finish(Activity.RESULT_OK,details)
        } catch (error: Throwable) {
            details.putString("market_lab","fail")
            details.putString("error",error.stackTraceToString())
            finish(Activity.RESULT_CANCELED,details)
        } finally {
            module.invalidate()
        }
    }

    private fun resolve(action: (Promise) -> Unit): Any {
        val reply = LinkedBlockingQueue<Result<Any>>()
        action(PromiseImpl(Callback { args ->
            reply.offer(Result.success(requireNotNull(args.firstOrNull())))
        }, Callback { args ->
            reply.offer(Result.failure(IllegalStateException(args.firstOrNull().toString())))
        }))
        return requireNotNull(reply.poll(14,TimeUnit.SECONDS)) { "native route timeout" }.getOrThrow()
    }

    private fun exchange(module: MarketLabModule, path: String, body: JSONObject? = null): JSONObject =
        JSONObject(resolve { module.exchange(path,body?.toString() ?: "",it) } as String)

    private fun rpc(method: String, vararg arguments: Any): Any {
        val request = JSONObject().put("jsonrpc","2.0").put("id",1).put("method",method).put("params",JSONArray(arguments.toList()))
        val conn = URL("http://127.0.0.1:8285").openConnection(Proxy.NO_PROXY) as HttpURLConnection
        try {
            conn.connectTimeout = 5000
            conn.readTimeout = 5000
            conn.requestMethod = "POST"
            conn.setRequestProperty("Content-Type","application/json")
            conn.doOutput = true
            conn.outputStream.use { it.write(request.toString().encodeToByteArray()) }
            val reply = JSONObject(conn.inputStream.bufferedReader().use { it.readText() })
            check(!reply.has("error")) { "fixture RPC rejected $method" }
            return reply.get("result")
        } finally { conn.disconnect() }
    }

    private fun price(target: String) {
        val accounts = rpc("eth_accounts") as JSONArray
        val head = rpc("eth_getBlockByNumber","latest",false) as JSONObject
        val stamp = head.getString("timestamp").removePrefix("0x").padStart(64,'0')
        for (index in 2..3) {
            rpc("eth_sendTransaction",JSONObject().put("from",accounts.getString(index)).put("to",target)
                .put("data","0x07640c1f" + BigInteger.valueOf(1_200_000).toString(16).padStart(64,'0') + stamp))
        }
        rpc("anvil_mine","0x03")
    }

    private fun exercise(module: MarketLabModule) {
        val profile = exchange(module,"/v1/profile").getJSONObject("profile")
        val venue = profile.getJSONObject("venue")
        check(profile.getInt("chainId") == 8283)
        val target = venue.getString("address")
        val domain = listOf(target, profile.getJSONObject("base").getString("address"),profile.getJSONObject("quote").getString("address"),
            profile.getString("quoteSigner"),venue.getString("codeHash")).joinToString("") { it.removePrefix("0x") }
        val user = resolve { module.walletAddress(domain,true,it) } as String
        try {
            check(user.matches(Regex("0x[0-9a-f]{40}")))
            check(resolve { module.walletAddress(domain,false,it) } == user)
            val other = domain.dropLast(1) + (if (domain.endsWith("1")) "2" else "1")
            check(runCatching { resolve { module.walletAddress(other,false,it) } }.isFailure)
            val encrypted = targetContext.getSharedPreferences("nu54-market-lab-8283-v1",0).all
            check(encrypted.keys == setOf("sealed") && Base64.decode(encrypted["sealed"] as String,Base64.NO_WRAP).size == 60)
            check(runCatching { exchange(module,"https://192.0.2.1/v1/profile") }.isFailure)
            check(runCatching { exchange(module,"/v1/profile/../transactions") }.isFailure)

            rpc("anvil_setBalance",user,"0x3635c9adc5dea00000")
            price(target)
            val request = JSONObject().put("user",user).put("action","fx-base")
                .put("a","100000055").put("b","0").put("minA","1200000").put("minB","0")
            val offer = exchange(module,"/v1/quotes",request)
            val wallet = exchange(module,"/v1/wallet/$user")
            val data = offer.getString("data").removePrefix("0x")
            val nonce = wallet.getDouble("nonce")
            val gas = wallet.getString("gasPrice").toDouble()
            val changed = data.replaceRange(135,136, if (data[135] == '0') "1" else "0")
            check(runCatching { resolve { module.sign(domain,target.removePrefix("0x"),nonce,gas,changed,changed,it) } }.isFailure)
            check(runCatching { resolve { module.sign(domain,user.removePrefix("0x"),nonce,gas,data,data,it) } }.isFailure)
            check(runCatching { resolve { module.sign(other,target.removePrefix("0x"),nonce,gas,data,data,it) } }.isFailure)
            val raw = resolve { module.sign(domain,target.removePrefix("0x"),nonce,gas,data,data,it) } as String
            check(raw.matches(Regex("0x[0-9a-f]{500,1200}")))
            val broadcast = exchange(module,"/v1/transactions",JSONObject().put("raw",raw))
            check(broadcast.getString("state") == "submitted")
            val txHash = broadcast.getString("txHash")
            rpc("anvil_mine","0x04")
            val effect = exchange(module,"/v1/executions/$txHash/${offer.getString("digest")}")
            check(effect.getString("state") == "confirmed" && effect.getString("first") == "1200000" && effect.has("gasFee"))
            // The chain can be advanced independently of Android's trusted clock.
            rpc("evm_setNextBlockTimestamp", System.currentTimeMillis() / 1000 + 120)
            rpc("evm_mine")
            price(target)
            check(runCatching { exchange(module,"/v1/quotes",request) }.isFailure)

        } finally {
            check(resolve { module.deleteWallet(domain,it) } == true)
            check(runCatching { resolve { module.walletAddress(domain,false,it) } }.isFailure)
        }
    }
}
