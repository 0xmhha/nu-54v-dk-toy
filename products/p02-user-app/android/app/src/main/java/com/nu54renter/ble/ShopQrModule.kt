package com.nu54renter.ble

import android.app.Activity
import android.content.Intent
import android.graphics.Bitmap
import android.provider.MediaStore
import com.facebook.react.bridge.BaseActivityEventListener
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext

class ShopQrModule(private val context: ReactApplicationContext) : NativeShopQrSpec(context) {
    private val request = 5404
    private var pending: Promise? = null
    private val camera = object : BaseActivityEventListener() {
        override fun onActivityResult(activity: Activity, requestCode: Int, resultCode: Int, data: Intent?) {
            if (requestCode != request) return
            val promise = pending ?: return
            pending = null
            if (resultCode != Activity.RESULT_OK) {
                promise.resolve("")
                return
            }
            @Suppress("DEPRECATION")
            val image = data?.extras?.get("data") as? Bitmap
            try {
                val value = OwnerPsbtQr.readBitmap(requireNotNull(image))
                promise.resolve(when {
                    value.startsWith("wc:") -> value.takeIf { it.length <= 1200 }.orEmpty()
                    // The operator's answer to a reset request: prefix 15, nonce 64, colon, signature 128.
                    value.startsWith("NU54-RESET1-OK:") -> value.takeIf { it.length == 208 }.orEmpty()
                    else -> value.take(97)
                })
            } catch (_: Exception) {
                unavailable(promise)
            } finally {
                image?.recycle()
            }
        }
    }

    init {
        context.addActivityEventListener(camera)
    }

    private fun unavailable(promise: Promise) =
        promise.reject("SHOP_CAMERA", "Could not read a single QR from the camera image")

    override fun scan(promise: Promise) {
        val activity = context.currentActivity
        if (activity == null) {
            unavailable(promise)
            return
        }
        activity.runOnUiThread {
            if (pending != null || activity.isFinishing) {
                unavailable(promise)
                return@runOnUiThread
            }
            pending = promise
            try {
                @Suppress("DEPRECATION")
                activity.startActivityForResult(Intent(MediaStore.ACTION_IMAGE_CAPTURE), request)
            } catch (_: Exception) {
                pending = null
                unavailable(promise)
            }
        }
    }

    override fun invalidate() {
        context.removeActivityEventListener(camera)
        context.currentActivity?.runOnUiThread {
            pending?.let(::unavailable)
            pending = null
        }
        super.invalidate()
    }
}
