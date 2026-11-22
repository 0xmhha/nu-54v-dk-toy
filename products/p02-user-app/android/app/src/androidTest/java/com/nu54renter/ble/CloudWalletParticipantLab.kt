package com.nu54renter.ble

import android.app.Activity
import android.app.Instrumentation
import android.os.Bundle
import android.util.Base64
import com.facebook.react.bridge.BridgeReactContext
import com.facebook.react.bridge.Callback
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.PromiseImpl
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit

class CloudWalletParticipantLab : Instrumentation() {
    private var flags: Bundle? = null

    override fun onCreate(arguments: Bundle?) {
        super.onCreate(arguments)
        flags = arguments
        start()
    }

    private fun call(work: (Promise) -> Unit): Any? {
        val done = CountDownLatch(1)
        var value: Any? = null
        work(PromiseImpl(
            Callback { values -> value = values.firstOrNull(); done.countDown() },
            Callback { done.countDown() },
        ))
        return if (done.await(15, TimeUnit.SECONDS)) value else null
    }

    override fun onStart() {
        val module = CloudWalletProfileModule(BridgeReactContext(targetContext))
        val subject = flags?.getString("subject").orEmpty()
        if (subject.isNotEmpty()) module.useLabScope(subject) else module.enableLabFixture()
        if (flags?.getString("mode") == "prepare") {
            val csr = call { module.prepareParticipant(subject, it) } as? String
            if (csr != null) {
                val encoded = Base64.encodeToString(csr.encodeToByteArray(), Base64.NO_WRAP)
                finish(Activity.RESULT_OK, Bundle().apply { putString("p03CSR", encoded) })
                return
            }
            finish(Activity.RESULT_CANCELED, Bundle())
            return
        }
        val roster = flags?.getString("roster")
        if (roster != null) {
            val publicEnvelope = Base64.decode(roster, Base64.NO_WRAP).decodeToString()
            if (call { module.installParticipant(subject, publicEnvelope, it) } != true) {
                finish(Activity.RESULT_CANCELED, Bundle())
                return
            }
        }
        if (call { module.startParticipant(subject, if (subject.isEmpty()) 0.0 else System.currentTimeMillis() / 1000.0 + 300.0, "", it) } != true) {
            finish(Activity.RESULT_CANCELED, Bundle().apply { putString("stream", "P03 participant did not bind") })
            return
        }
        sendStatus(1, Bundle().apply { putString("stream", "P03 participant is ready") })
        Thread.sleep(9 * 60 * 1000L)
        finish(Activity.RESULT_OK, Bundle())
    }
}
