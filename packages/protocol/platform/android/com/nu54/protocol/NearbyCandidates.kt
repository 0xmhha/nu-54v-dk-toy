package com.nu54.protocol

import android.os.SystemClock
import java.util.UUID

/** Scan-local handles expire on a monotonic clock and can be consumed once before the next scan. */
internal class NearbyCandidates<T>(private val clock: () -> Long = SystemClock::elapsedRealtime) {
    private val values = linkedMapOf<String, T>()
    private var expires: Long? = null
    fun reset() { values.clear(); expires = null }
    fun add(device: T) {
        if (expires == null && values.size < 16 && device !in values.values)
            values[UUID.randomUUID().toString()] = device
    }
    fun finish(): Set<String> {
        expires = clock() + 20_000
        return values.keys.toSet()
    }
    fun take(handle: String, clearBatch: Boolean = false): T? {
        val deadline = expires ?: return null
        if (clock() >= deadline) { reset(); return null }
        val device = values.remove(handle) ?: return null
        if (clearBatch) reset()
        return device
    }
}
