package com.nu54kiosk.ble

import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.nu54.protocol.CommerceHttp
import java.net.ConnectException
import java.net.NoRouteToHostException
import java.net.UnknownHostException
import java.util.concurrent.Executors

class CommerceHttpModule(context: ReactApplicationContext) : NativeCommerceHttpSpec(context) {
    private val implementation=CommerceHttp(context,setOf("terminal","operator","finance"))
    private val worker=Executors.newFixedThreadPool(2)
    private fun submit(promise:Promise, terminalScoped:Boolean = false, call:()->Any?) {
        worker.execute {
            try { promise.resolve(call()) }
            catch (error:Exception) {
                val disconnected = terminalScoped &&
                    (error is ConnectException || error is NoRouteToHostException || error is UnknownHostException)
                promise.reject(if (disconnected) "COMMERCE_NETWORK_OFFLINE" else "COMMERCE_UNAVAILABLE",
                    "The enrolled commerce connection is unavailable")
            }
        }
    }
    override fun randomBytes(size:Double,promise:Promise)=submit(promise) { implementation.randomBytes(size) }
    override fun loadJournal(name:String,promise:Promise)=submit(promise) { implementation.loadJournal(name) }
    override fun saveJournal(name:String,value:String,promise:Promise)=submit(promise) { implementation.saveJournal(name,value); null }
    override fun publicIdentity(slot:String, promise:Promise)=submit(promise) { implementation.publicIdentity(slot) }
    override fun configure(slot:String, profile:String, promise:Promise)=submit(promise) { implementation.configure(slot,profile); null }
    override fun loadProfile(slot:String, promise:Promise)=submit(promise) { implementation.load(slot) }
    override fun exchange(slot:String, envelope:String, promise:Promise)=submit(promise) { implementation.exchange(slot,envelope) }
    override fun exchangeScoped(slot:String, profileDigest:String, envelope:String, promise:Promise)=
        submit(promise, terminalScoped = slot == "terminal") { implementation.exchangeScoped(slot,profileDigest,envelope) }
    override fun exchangeRenterV1Origin(operation:String, profileDigest:String, body:String, promise:Promise)=
        submit(promise, terminalScoped = true) { implementation.exchangeRenterV1Origin(operation,profileDigest,body) }
    override fun exchangePrint(operation:String, profileDigest:String, reference:String, body:String, promise:Promise)=
        submit(promise) { implementation.exchangePrint(operation,profileDigest,reference,body) }
    override fun invalidate() {
        worker.shutdownNow()
        super.invalidate()
    }
}
