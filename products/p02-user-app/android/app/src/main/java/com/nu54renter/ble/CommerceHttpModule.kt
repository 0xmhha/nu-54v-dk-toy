package com.nu54renter.ble

import android.Manifest
import android.content.pm.PackageManager
import android.media.MediaRecorder
import android.os.Build
import android.util.Base64
import java.io.File
import java.net.ConnectException
import java.net.NoRouteToHostException
import java.net.UnknownHostException
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.nu54.protocol.CommerceHttp
import java.util.concurrent.Executors
import org.json.JSONObject

class CommerceHttpModule(context: ReactApplicationContext) : NativeCommerceHttpSpec(context) {
    private val implementation=CommerceHttp(context,setOf("customer","travel"))
    private val worker=Executors.newFixedThreadPool(2)
    private val recorder=TravelRecorder(context)
    private fun submit(promise:Promise, catalogRead:Boolean = false, call:()->Any?) {
        worker.execute {
            try { promise.resolve(call()) }
            catch (error:Exception) {
                val disconnected = catalogRead &&
                    (error is ConnectException || error is NoRouteToHostException || error is UnknownHostException)
                // A failed check means nothing was sent, or the server answered outside the protocol (a plain 404).
                val refused = error is IllegalArgumentException
                promise.reject(if (disconnected) "COMMERCE_NETWORK_OFFLINE" else if (refused) "COMMERCE_REFUSED" else "COMMERCE_UNAVAILABLE",
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
    override fun startTravelRecording(promise:Promise)=submit(promise) { recorder.start();null }
    override fun finishTravelRecording(promise:Promise)=submit(promise) { recorder.finish() }
    override fun discardTravelRecording(promise:Promise)=submit(promise) { recorder.discard();null }
    override fun exchangeRenterReturn(profileDigest:String, assignment:String, account:String, promise:Promise)=
        submit(promise) { implementation.exchangeRenterReturn(profileDigest,assignment,account) }
    override fun exchangeTravel(method:String,path:String,envelope:String,promise:Promise)=submit(promise) { implementation.exchangeTravel(method,path,envelope) }
    override fun exchangeScoped(slot:String, profileDigest:String, envelope:String, promise:Promise) {
        val catalogRead = slot == "customer" && envelope.length <= 8192 && runCatching {
            val command = JSONObject(envelope).getJSONObject("command")
            command.getString("contract") == "C3" && command.getString("operation") == "catalog.read"
        }.getOrDefault(false)
        submit(promise, catalogRead = catalogRead) { implementation.exchangeScoped(slot,profileDigest,envelope) }
    }
    override fun exchange(slot:String, envelope:String, promise:Promise)=submit(promise) { implementation.exchange(slot,envelope) }
    override fun invalidate() {
        recorder.discard()
        worker.shutdownNow()
        super.invalidate()
    }
}


private class TravelRecorder(private val context: ReactApplicationContext) {
    private var recorder:MediaRecorder?=null
    private var sample:File?=null

    init {
        context.cacheDir.listFiles { item -> item.name.startsWith("nu54-travel-") && item.extension=="m4a" }
            ?.forEach { it.delete() }
    }

    @Synchronized fun start() {
        require(recorder==null && context.checkSelfPermission(Manifest.permission.RECORD_AUDIO)==PackageManager.PERMISSION_GRANTED)
        val file=File.createTempFile("nu54-travel-",".m4a",context.cacheDir)
        val recording=if(Build.VERSION.SDK_INT>=31) MediaRecorder(context) else {
            @Suppress("DEPRECATION")
            MediaRecorder()
        }
        try {
            recording.setAudioSource(MediaRecorder.AudioSource.MIC)
            recording.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4)
            recording.setAudioEncoder(MediaRecorder.AudioEncoder.AAC)
            recording.setAudioEncodingBitRate(32_000)
            recording.setAudioChannels(1)
            recording.setMaxDuration(20_000)
            recording.setMaxFileSize(95_000)
            recording.setOutputFile(file.path)
            recording.prepare()
            recording.start()
            sample=file
            recorder=recording
        } catch (error:Exception) {
            recording.release()
            file.delete()
            throw error
        }
    }

    @Synchronized fun finish():String {
        val recording=requireNotNull(recorder)
        val file=requireNotNull(sample)
        return try {
            recording.stop()
            require(file.length() in 1L..95_000L)
            Base64.encodeToString(file.readBytes(),Base64.NO_WRAP)
        } finally {
            discard()
        }
    }

    @Synchronized fun discard() {
        recorder?.release()
        sample?.delete()
        recorder=null
        sample=null
    }
}
