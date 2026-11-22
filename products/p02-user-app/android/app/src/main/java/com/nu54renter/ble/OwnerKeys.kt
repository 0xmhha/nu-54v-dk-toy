package com.nu54renter.ble

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyInfo
import android.security.keystore.KeyProperties
import android.util.AtomicFile
import java.io.File
import java.io.FileNotFoundException
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

internal class OwnerKeys(context: Context) {
    private companion object {
        const val PROFILE_LIMIT = 32768
        const val ENVELOPE_BYTES = 28
        val profileLock = Any()
    }
    private val storage = AtomicFile(File(context.noBackupFilesDir, "nu54-owner-profile.v2"))
    private val provider = "AndroidKeyStore"
    private val signing = "com.nu54renter.owner.identity.v2"
    private val encryption = "com.nu54renter.owner.profile.v2"
    private val aad = "nu54:owner-profile:v2".encodeToByteArray()
    private val store get() = KeyStore.getInstance(provider).apply { load(null) }

    fun identity(): KeyStore.PrivateKeyEntry {
        val keys = store
        if (!keys.containsAlias(signing)) {
            KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, provider).apply {
                initialize(
                    KeyGenParameterSpec.Builder(signing, KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY)
                        .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                        // Conscrypt hashes a TLS handshake and uses the platform's raw signing API.
                        .setDigests(KeyProperties.DIGEST_NONE, KeyProperties.DIGEST_SHA256)
                        .build()
                )
            }.generateKeyPair()
        }
        val entry = store.getEntry(signing, null) as KeyStore.PrivateKeyEntry
        val info = KeyFactory.getInstance(entry.privateKey.algorithm, provider)
            .getKeySpec(entry.privateKey, KeyInfo::class.java)
        @Suppress("DEPRECATION")
        require(info.isInsideSecureHardware) { "hardware-backed owner identity unavailable" }
        return entry
    }

    fun sign(record: ByteArray, expected: ByteArray): ByteArray {
        require(record.size == 130 && record[0] == 1.toByte() && record[1] in 1..5 &&
            record.copyOfRange(42, 74).contentEquals(expected))
        return Signature.getInstance("SHA256withECDSA").run {
            initSign(identity().privateKey)
            update(record)
            sign()
        }
    }

    private fun secret(create: Boolean): SecretKey {
        val keys = store
        if (!keys.containsAlias(encryption)) {
            require(create && !storage.baseFile.exists()) { "wallet profile key is unavailable" }
            KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, provider).apply {
                init(KeyGenParameterSpec.Builder(encryption, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                    .setKeySize(256).setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
            }.generateKey()
        }
        return store.getKey(encryption, null) as SecretKey
    }

    fun load(): String = synchronized(profileLock) { loadProtected() }

    private fun loadProtected(): String {
        val contents = try {
            storage.openRead().use { input ->
                val length = input.channel.size()
                require(length in (ENVELOPE_BYTES + 1L)..(PROFILE_LIMIT + ENVELOPE_BYTES).toLong())
                input.readBytes()
            }
        } catch (error: FileNotFoundException) {
            if (storage.baseFile.exists() || File(storage.baseFile.path + ".bak").exists()) throw error
            return ""
        }
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, secret(false), GCMParameterSpec(128, contents.copyOfRange(0, 12)))
        cipher.updateAAD(aad)
        return cipher.doFinal(contents, 12, contents.size - 12).decodeToString(throwOnInvalidSequence = true)
    }

    fun save(value: String) = synchronized(profileLock) {
        require(loadProtected().isEmpty()) { "wallet profile already exists" }
        writeProtected(value)
    }

    fun compareAndSave(expected: String, value: String): Boolean = synchronized(profileLock) {
        require(expected.encodeToByteArray().size <= PROFILE_LIMIT)
        if (loadProtected() != expected) return@synchronized false
        writeProtected(value)
        true
    }

    private fun writeProtected(value: String) {
        val clear = value.encodeToByteArray()
        require(clear.size in 1..PROFILE_LIMIT)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, secret(true))
        require(cipher.iv.size == 12)
        cipher.updateAAD(aad)
        val record = cipher.iv + cipher.doFinal(clear)
        val out = storage.startWrite()
        try {
            out.write(record)
            out.fd.sync()
        } catch (error: Exception) {
            storage.failWrite(out)
            throw error
        }
        storage.finishWrite(out)
        require(loadProtected() == value) { "wallet profile commit unavailable" }
    }
}
