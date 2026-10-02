package com.nu54kiosk.ble

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import java.io.File
import java.security.KeyStore
import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Key storage for the kiosk (N32): the secp256k1 gas key and merchant signing key are wrapped
 * with an AES-256-GCM key that lives in Android Keystore and never leaves it, and the wrapped
 * bytes (IV and ciphertext) are kept in the app's private preferences.
 */
class KioskVaultModule(private val context: ReactApplicationContext) : NativeKioskVaultSpec(context) {
  private val random = SecureRandom()
  private val keys get() = context.getSharedPreferences("nu54-keys", Context.MODE_PRIVATE)
  private val settings get() = context.getSharedPreferences("nu54-settings", Context.MODE_PRIVATE)

  private fun wrappingKey(): SecretKey {
    val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
    (ks.getKey(WRAP_ALIAS, null) as? SecretKey)?.let { return it }
    val gen = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
    gen.init(
      KeyGenParameterSpec.Builder(WRAP_ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
        .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
        .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
        .setKeySize(256)
        .build(),
    )
    return gen.generateKey()
  }

  override fun randomBytes(n: Double, promise: Promise) {
    val out = ByteArray(n.toInt())
    random.nextBytes(out)
    promise.resolve(Base64.encodeToString(out, Base64.NO_WRAP))
  }

  override fun wrapKey(alias: String, base64: String, promise: Promise) {
    try {
      val raw = Base64.decode(base64, Base64.NO_WRAP)
      val cipher = Cipher.getInstance("AES/GCM/NoPadding")
      cipher.init(Cipher.ENCRYPT_MODE, wrappingKey()) // Keystore picks the IV
      val sealed = cipher.iv + cipher.doFinal(raw)
      raw.fill(0)
      keys.edit().putString(alias, Base64.encodeToString(sealed, Base64.NO_WRAP)).apply()
      promise.resolve(null)
    } catch (e: Exception) {
      promise.reject("WRAP_FAILED", e.message, e)
    }
  }

  override fun unwrapKey(alias: String, promise: Promise) {
    try {
      val stored = keys.getString(alias, null) ?: return promise.resolve(null)
      val sealed = Base64.decode(stored, Base64.NO_WRAP)
      val cipher = Cipher.getInstance("AES/GCM/NoPadding")
      cipher.init(Cipher.DECRYPT_MODE, wrappingKey(), GCMParameterSpec(128, sealed, 0, IV_LEN))
      val raw = cipher.doFinal(sealed, IV_LEN, sealed.size - IV_LEN)
      promise.resolve(Base64.encodeToString(raw, Base64.NO_WRAP))
      raw.fill(0)
    } catch (e: Exception) {
      promise.reject("UNWRAP_FAILED", e.message, e)
    }
  }

  override fun putSetting(name: String, value: String, promise: Promise) {
    settings.edit().putString(name, value).apply()
    promise.resolve(null)
  }

  override fun getSetting(name: String, promise: Promise) {
    promise.resolve(settings.getString(name, null))
  }

  override fun takeFile(name: String, promise: Promise) {
    // Only plain names inside the private files directory.
    if (!name.matches(Regex("[a-z0-9-]+\\.json"))) return promise.reject("BAD_NAME", name)
    val f = File(context.filesDir, name)
    if (!f.exists()) return promise.resolve(null)
    val text = f.readText()
    f.delete()
    promise.resolve(text)
  }

  companion object {
    private const val WRAP_ALIAS = "nu54-kiosk-wrap"
    private const val IV_LEN = 12
  }
}
