package com.nu54renter.ble

import android.graphics.Bitmap
import android.graphics.BitmapFactory
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.RGBLuminanceSource
import com.google.zxing.common.HybridBinarizer
import com.google.zxing.multi.qrcode.QRCodeMultiReader

internal object OwnerPsbtQr {
    const val MaxFileBytes = 2 * 1024 * 1024
    private const val MaxDimension = 4096

    fun readImage(encoded: ByteArray, extension: String): String {
        require(encoded.size in 1..MaxFileBytes)
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(encoded, 0, encoded.size, bounds)
        val imageMime = if (extension == "png") "image/png" else "image/jpeg"
        require(bounds.outMimeType == imageMime)
        require(bounds.outWidth in 1..MaxDimension && bounds.outHeight in 1..MaxDimension)
        val bitmap = requireNotNull(BitmapFactory.decodeByteArray(encoded, 0, encoded.size, BitmapFactory.Options().apply {
            inSampleSize = if (maxOf(bounds.outWidth, bounds.outHeight) > 2048) 2 else 1
        }))
        try {
            return readBitmap(bitmap)
        } finally {
            bitmap.recycle()
        }
    }

    fun readBitmap(bitmap: Bitmap): String {
        val width = bitmap.width
        val height = bitmap.height
        require(width in 1..2048 && height in 1..2048)
        val pixels = IntArray(width * height)
        bitmap.getPixels(pixels, 0, width, 0, 0, width, height)
        val source = RGBLuminanceSource(width, height, pixels)
        val hints = mapOf(DecodeHintType.TRY_HARDER to true)
        val codes = QRCodeMultiReader().decodeMultiple(BinaryBitmap(HybridBinarizer(source)), hints)
        require(codes.size == 1)
        return codes[0].text
    }
}
