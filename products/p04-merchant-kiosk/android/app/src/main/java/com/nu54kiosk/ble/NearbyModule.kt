package com.nu54kiosk.ble

import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.nu54.protocol.NearbyBluetooth

class NearbyModule(context: ReactApplicationContext) : NativeNearbySpec(context) {
    private val nearby = NearbyBluetooth(context, true) { emitOnNearby(it) }

    private val payment = NearbyBluetooth(context, false, payment = true) { emitOnPayment(it) }

    private fun <T> result(promise: Promise): (Result<T>) -> Unit = { outcome ->
        outcome.fold({ promise.resolve(if (it == Unit) null else it) },
            { promise.reject("NEARBY_UNAVAILABLE", "Nearby operation unavailable") })
    }

    override fun publicIdentity(promise: Promise) = nearby.dispatch(result(promise), nearby::publicIdentity)
    override fun configure(enrollment: String, promise: Promise) = nearby.dispatch(result(promise)) { nearby.configure(enrollment) }
    override fun scan(seconds: Double, promise: Promise) = nearby.scan(seconds, result(promise))
    override fun connect(handle: String, promise: Promise) = nearby.dispatch(result(promise)) { nearby.connect(handle) }
    override fun start(promise: Promise) = nearby.start(result(promise))
    override fun stop(promise: Promise) = nearby.dispatch(result(promise), nearby::stop)
    override fun close(link: String, promise: Promise) = nearby.dispatch(result(promise)) { nearby.close(link) }
    override fun write(link: String, clear: String, promise: Promise) = nearby.dispatch(result(promise)) { nearby.write(link, clear) }
    private fun <T> paymentCall(promise: Promise, work: (NearbyBluetooth) -> T) {
        val active = payment
        if (active == null) promise.reject("NEARBY_UNAVAILABLE", "Board payment is unavailable")
        else active.dispatch(result(promise)) { work(active) }
    }
    override fun paymentPublicIdentity(promise: Promise) = paymentCall(promise, NearbyBluetooth::publicIdentity)
    override fun configurePayment(enrollment: String, promise: Promise) = paymentCall(promise) { it.configure(enrollment) }
    override fun scanPayment(seconds: Double, promise: Promise) {
        val active = payment
        if (active == null) promise.reject("NEARBY_UNAVAILABLE", "Board payment is unavailable")
        else active.scan(seconds, result(promise))
    }
    override fun connectPayment(handle: String, promise: Promise) = paymentCall(promise) { it.connect(handle) }
    override fun stopPayment(promise: Promise) = paymentCall(promise, NearbyBluetooth::stop)
    override fun closePayment(link: String, promise: Promise) = paymentCall(promise) { it.close(link) }
    override fun writePayment(link: String, clear: String, promise: Promise) = paymentCall(promise) { it.write(link, clear) }
    override fun invalidate() {
        nearby.dispatch<Unit>({}, nearby::stop)
        payment?.let { it.dispatch<Unit>({}, it::stop) }
        super.invalidate()
    }
}
