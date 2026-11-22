package com.nu54.protocol

import android.annotation.SuppressLint
import android.bluetooth.*
import android.bluetooth.le.*
import android.content.Context
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import org.json.JSONArray
import org.json.JSONObject
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.spec.ECGenParameterSpec
import java.util.ArrayDeque
import java.util.UUID

/** BLE carries only bounded, acknowledged TLS stream fragments. State lives on one handler. */
@SuppressLint("MissingPermission")
class NearbyBluetooth(private val context: Context, private val terminal: Boolean, private val payment: Boolean = false,
                      private val event: (String) -> Unit) {
    private val handler = Handler(Looper.getMainLooper())
    private val manager get() = context.getSystemService(BluetoothManager::class.java)
    private val adapter get() = requireNotNull(manager.adapter)
    private val role get() = if (terminal || payment) "terminal" else "customer"
    private val service get() = if (payment) BOARD_SERVICE else SERVICE
    private val rxUUID get() = if (payment) BOARD_RX else RX
    private val txUUID get() = if (payment) BOARD_TX else TX
    private var config: Enrollment? = null
    private var server: BluetoothGattServer? = null
    private var advert: AdvertiseCallback? = null
    private var scanner: ScanCallback? = null
    private var scanGeneration = 0
    private var startFinish: ((Result<Unit>) -> Unit)? = null
    private var notification: Link? = null
    private var scanFinish: ((Result<String>) -> Unit)? = null
    private val candidates = NearbyCandidates<BluetoothDevice>()
    private val links = mutableMapOf<String, Link>()
    private val nearbyServerLinks = mutableMapOf<BluetoothDevice, Link>()
    private var subscribe: BluetoothGattCharacteristic? = null

    private data class Enrollment(val ca: String, val chain: List<String>, val peers: Map<String, NearbyPeer>, val expires: Long)
    private class Link(val id: String, val device: BluetoothDevice, val tls: NearbyTls) {
        var gatt: BluetoothGatt? = null
        var rx: BluetoothGattCharacteristic? = null
        var mtu = 23
        var subscribed = false
        var ready = false
        var busy = false
        var bytes = 0
        var retry = 0
        var first = true
        val outgoing = ArrayDeque<ByteArray>()
    }

    fun <T> dispatch(result: (Result<T>) -> Unit, work: () -> T) {
        handler.post { result(runCatching(work)) }
    }

    private fun identity(create: Boolean): KeyStore.PrivateKeyEntry {
        val alias = if (payment) "nu54-payment-terminal-v3" else "nu54-nearby-$role-v3"
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        if (create && !store.containsAlias(alias)) {
            KeyPairGenerator.getInstance("EC", "AndroidKeyStore").apply {
                initialize(KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_SIGN)
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_NONE, KeyProperties.DIGEST_SHA256, KeyProperties.DIGEST_SHA384, KeyProperties.DIGEST_SHA512)
                    .build())
            }.generateKeyPair()
        }
        return store.getEntry(alias, null) as KeyStore.PrivateKeyEntry
    }

    fun publicIdentity() = base64(identity(true).certificate.publicKey.encoded)

    fun configure(raw: String) {
        require(raw.length in 1..49_152)
        val profile = JSONObject(raw)
        val branch = uuid(profile.getString("branch"))
        val expiry = profile.getLong("expiresAt")
        require(expiry > System.currentTimeMillis() / 1000)
        val list = profile.getJSONArray("peers")
        require(list.length() in 1..64)
        val peers = (0 until list.length()).map {
            val entry = list.getJSONObject(it)
            val pin = entry.getString("pin")
            require(pin.matches(Regex("[0-9a-f]{64}")) && uuid(entry.getString("branch")) == branch)
            NearbyPeer(pin, uuid(entry.getString("principal")), branch, minOf(expiry, entry.optLong("expiresAt", expiry)))
        }
        require(peers.map { it.pin }.distinct().size == peers.size)
        val certificates = profile.getJSONArray("certificates")
        require(certificates.length() in 1..4)
        val next = Enrollment(profile.getString("ca"), (0 until certificates.length()).map(certificates::getString),
            peers.associateBy { it.pin }, expiry)
        makeTls(next)
        stop()
        config = next
    }

    private fun makeTls(value: Enrollment = requireNotNull(config)) =
        NearbyTls(identity(false), value.ca, value.chain, role, value.peers, value.expires, payment)

    private fun uuid(value: String): String {
        require(value == UUID.fromString(value).toString() && value != "00000000-0000-0000-0000-000000000000")
        return value
    }

    fun scan(seconds: Double, result: (Result<String>) -> Unit) {
        handler.post {
            val failure = runCatching {
                require(!terminal && config != null && seconds in 1.0..10.0)
                stopScan()
                candidates.reset()
                val generation = ++scanGeneration
                val scan = object : ScanCallback() {
                    override fun onScanResult(type: Int, data: ScanResult) {
                        handler.post {
                            if (scanner === this) candidates.add(data.device)
                        }
                    }
                    override fun onScanFailed(code: Int) {
                        handler.post { if (scanner === this) stopScan(IllegalStateException("Nearby scan unavailable")) }
                    }
                }
                scanner = scan
                scanFinish = result
                requireNotNull(adapter.bluetoothLeScanner).startScan(listOf(ScanFilter.Builder().setServiceUuid(android.os.ParcelUuid(service)).build()),
                    ScanSettings.Builder().setScanMode(ScanSettings.SCAN_MODE_LOW_LATENCY).build(), scan)
                handler.postDelayed({
                    if (scanner === scan) {
                        val output = JSONArray(candidates.finish().map { JSONObject().put("handle", it) }).toString()
                        stopScan(output = output)
                        handler.postDelayed({ if (scanner == null && scanGeneration == generation) candidates.reset() }, 20_000)
                    }
                }, (seconds * 1000).toLong())
            }.exceptionOrNull()
            if (failure != null) {
                if (scanFinish === result) stopScan(failure) else result(Result.failure(failure))
            }
        }
    }

    private fun stopScan(error: Throwable = IllegalStateException("Nearby scan ended"), output: String? = null) {
        val current = scanner
        val callback = scanFinish
        scanner = null
        scanFinish = null
        if (current != null) runCatching { adapter.bluetoothLeScanner?.stopScan(current) }
        callback?.invoke(if (output == null) Result.failure(error) else Result.success(output))
    }

    fun connect(handle: String): String {
        require(!terminal && scanner == null && links.isEmpty())
        val device = requireNotNull(candidates.take(handle))
        val link = Link(UUID.randomUUID().toString(), device, makeTls())
        links[link.id] = link
        try {
            link.gatt = requireNotNull(device.connectGatt(context, false, client(link), BluetoothDevice.TRANSPORT_LE))
            deadline(link)
            return link.id
        } catch (failure: Throwable) {
            close(link.id)
            throw failure
        }
    }

    private fun client(link: Link) = object : BluetoothGattCallback() {
        override fun onConnectionStateChange(gatt: BluetoothGatt, status: Int, state: Int) = guarded(link) {
            require(status == BluetoothGatt.GATT_SUCCESS && state == BluetoothProfile.STATE_CONNECTED)
            if (!gatt.requestMtu(517)) require(gatt.discoverServices())
        }
        override fun onMtuChanged(gatt: BluetoothGatt, mtu: Int, status: Int) = guarded(link) {
            if (status == BluetoothGatt.GATT_SUCCESS) link.mtu = mtu.coerceIn(23, 517)
            require(gatt.discoverServices())
        }
        override fun onServicesDiscovered(gatt: BluetoothGatt, status: Int) = guarded(link) {
            require(status == BluetoothGatt.GATT_SUCCESS)
            val service = requireNotNull(gatt.getService(this@NearbyBluetooth.service))
            link.rx = requireNotNull(service.getCharacteristic(rxUUID))
            val tx = requireNotNull(service.getCharacteristic(txUUID))
            val ccc = requireNotNull(tx.getDescriptor(CCC))
            require(gatt.setCharacteristicNotification(tx, true))
            if (Build.VERSION.SDK_INT >= 33) require(gatt.writeDescriptor(ccc, BluetoothGattDescriptor.ENABLE_NOTIFICATION_VALUE) == BluetoothStatusCodes.SUCCESS)
            else {
                @Suppress("DEPRECATION")
                ccc.value = BluetoothGattDescriptor.ENABLE_NOTIFICATION_VALUE
                @Suppress("DEPRECATION")
                require(gatt.writeDescriptor(ccc))
            }
        }
        override fun onDescriptorWrite(gatt: BluetoothGatt, descriptor: BluetoothGattDescriptor, status: Int) = guarded(link) {
            require(status == BluetoothGatt.GATT_SUCCESS && descriptor.uuid == CCC && !link.subscribed)
            link.subscribed = true
            received(link, link.tls.begin())
        }
        override fun onCharacteristicWrite(gatt: BluetoothGatt, characteristic: BluetoothGattCharacteristic, result: Int) = guarded(link) {
            require(result == BluetoothGatt.GATT_SUCCESS && characteristic.uuid == rxUUID)
            acknowledged(link)
        }
        override fun onCharacteristicChanged(gatt: BluetoothGatt, characteristic: BluetoothGattCharacteristic, value: ByteArray) =
            encrypted(link, characteristic.uuid, value.copyOf())
        @Deprecated("Android 12 notification callback")
        override fun onCharacteristicChanged(gatt: BluetoothGatt, characteristic: BluetoothGattCharacteristic) {
            @Suppress("DEPRECATION")
            encrypted(link, characteristic.uuid, characteristic.value.copyOf())
        }
    }

    private fun encrypted(link: Link, uuid: UUID, value: ByteArray) = guarded(link) {
        require(uuid == txUUID && link.subscribed && value.size in 1..244)
        received(link, link.tls.feed(value))
    }

    fun start(result: (Result<Unit>) -> Unit) {
        handler.post {
            val failure = runCatching { startServer(result) }.exceptionOrNull()
            if (failure != null) {
                if (startFinish === result) stop() else result(Result.failure(failure))
            }
        }
    }

    private fun startServer(result: (Result<Unit>) -> Unit) {
        require(terminal && server == null && config != null && adapter.isMultipleAdvertisementSupported)
        require(requireNotNull(config).expires > System.currentTimeMillis() / 1000)
        startFinish = result
        handler.postDelayed({ if (startFinish === result) stop() }, 8000)
        val database = requireNotNull(manager.openGattServer(context, peripheral))
        server = database
        val service = BluetoothGattService(SERVICE, BluetoothGattService.SERVICE_TYPE_PRIMARY)
        service.addCharacteristic(BluetoothGattCharacteristic(RX, BluetoothGattCharacteristic.PROPERTY_WRITE,
            BluetoothGattCharacteristic.PERMISSION_WRITE))
        subscribe = BluetoothGattCharacteristic(TX, BluetoothGattCharacteristic.PROPERTY_NOTIFY, 0).also {
            it.addDescriptor(BluetoothGattDescriptor(CCC, BluetoothGattDescriptor.PERMISSION_WRITE))
            service.addCharacteristic(it)
        }
        if (!database.addService(service)) {
            stop()
            error("Nearby service unavailable")
        }
    }

    private val peripheral = object : BluetoothGattServerCallback() {
        override fun onServiceAdded(status: Int, service: BluetoothGattService) {
            handler.post {
                try {
                    require(server != null && status == BluetoothGatt.GATT_SUCCESS && service.uuid == SERVICE)
                    val callback = object : AdvertiseCallback() {
                        override fun onStartSuccess(settingsInEffect: AdvertiseSettings) {
                            handler.post {
                                if (advert === this) {
                                    val finish = startFinish
                                    startFinish = null
                                    finish?.invoke(Result.success(Unit))
                                }
                            }
                        }
                        override fun onStartFailure(errorCode: Int) {
                            handler.post { if (advert === this) stop() }
                        }
                    }
                    advert = callback
                    requireNotNull(adapter.bluetoothLeAdvertiser).startAdvertising(
                        AdvertiseSettings.Builder().setConnectable(true).setAdvertiseMode(AdvertiseSettings.ADVERTISE_MODE_LOW_LATENCY).build(),
                        AdvertiseData.Builder().setIncludeDeviceName(false).setIncludeTxPowerLevel(false)
                            .addServiceUuid(android.os.ParcelUuid(this@NearbyBluetooth.service)).build(), callback)
                } catch (_: Exception) { stop() }
            }
        }
        override fun onConnectionStateChange(device: BluetoothDevice, status: Int, state: Int) {
            handler.post {
                if (server == null) return@post
                if (state != BluetoothProfile.STATE_CONNECTED || status != BluetoothGatt.GATT_SUCCESS) {
                    nearbyServerLinks[device]?.let { close(it.id) }
                    return@post
                }
                if (nearbyServerLinks.containsKey(device)) return@post
                if (links.size >= 2) { server?.cancelConnection(device); return@post }
                try {
                    val link = Link(UUID.randomUUID().toString(), device, makeTls())
                    nearbyServerLinks[device] = link
                    links[link.id] = link
                    deadline(link)
                } catch (_: Exception) { server?.cancelConnection(device) }
            }
        }
        override fun onMtuChanged(device: BluetoothDevice, mtu: Int) {
            handler.post { nearbyServerLinks[device]?.mtu = mtu.coerceIn(23, 517) }
        }
        override fun onDescriptorWriteRequest(device: BluetoothDevice, request: Int, descriptor: BluetoothGattDescriptor,
                                               prepared: Boolean, response: Boolean, offset: Int, value: ByteArray) {
            handler.post {
                val link = nearbyServerLinks[device]
                val accepted = link != null && !prepared && response && offset == 0 && descriptor.uuid == CCC &&
                    descriptor.characteristic.uuid == TX && value.contentEquals(BluetoothGattDescriptor.ENABLE_NOTIFICATION_VALUE)
                server?.sendResponse(device, request, if (accepted) BluetoothGatt.GATT_SUCCESS else BluetoothGatt.GATT_FAILURE, 0, null)
                if (accepted) link?.subscribed = true else link?.let { close(it.id) }
            }
        }
        override fun onCharacteristicWriteRequest(device: BluetoothDevice, request: Int, characteristic: BluetoothGattCharacteristic,
                                                   prepared: Boolean, response: Boolean, offset: Int, value: ByteArray) {
            val payload = value.copyOf()
            handler.post {
                val link = nearbyServerLinks[device]
                var acked = false
                try {
                    require(link != null && !prepared && response && offset == 0 && characteristic.uuid == rxUUID &&
                        link.subscribed && payload.size in 1..244)
                    val decoded = link.tls.feed(payload)
                    acked = server?.sendResponse(device, request, BluetoothGatt.GATT_SUCCESS, 0, null) == true
                    require(acked)
                    received(link, decoded)
                } catch (_: Exception) {
                    if (response && !acked) server?.sendResponse(device, request, BluetoothGatt.GATT_FAILURE, 0, null)
                    link?.let { close(it.id) }
                }
            }
        }
        override fun onNotificationSent(device: BluetoothDevice, status: Int) {
            handler.post {
                val link = nearbyServerLinks[device] ?: return@post
                if (notification !== link) return@post
                notification = null
                if (status != BluetoothGatt.GATT_SUCCESS) close(link.id)
                else try { acknowledged(link) } catch (_: Exception) { close(link.id) }
                otherLink(link)?.let { guarded(it) { pump(it) } }
            }
        }
    }

    private fun deadline(link: Link) {
        handler.postDelayed({ if (links[link.id] === link && !link.ready) close(link.id) }, 12_000)
        handler.postDelayed({ if (links[link.id] === link) close(link.id) }, 180_000)
    }

    private fun guarded(link: Link, work: () -> Unit) {
        handler.post { if (links[link.id] === link) try { work() } catch (_: Exception) { close(link.id) } }
    }

    private fun received(link: Link, packet: NearbyPacket) {
        require(!packet.closed)
        if (packet.encrypted.isNotEmpty()) {
            val encrypted = if (payment && link.first) byteArrayOf(2) + packet.encrypted else packet.encrypted
            link.first = false
            require(link.bytes + encrypted.size <= 49_152)
            val unit = minOf(link.mtu - 3, 244)
            for (offset in encrypted.indices step unit)
                link.outgoing.add(encrypted.copyOfRange(offset, minOf(offset + unit, encrypted.size)))
            link.bytes += encrypted.size
            pump(link)
        }
        val peer = packet.peer
        if (!link.ready && packet.epoch != null && peer != null) {
            link.ready = true
            sendEvent(link.id, "ready", JSONObject().put("epoch", base64(packet.epoch)).put("peer", peer.pin)
                .put("branch", peer.branch).put("principal", peer.principal).put("expiresAt", peer.expiresAt))
        }
        if (packet.clear.isNotEmpty()) {
            require(link.ready)
            sendEvent(link.id, "data", JSONObject().put("clear", base64(packet.clear)))
        }
    }

    private fun otherLink(link: Link) = links.values.firstOrNull { it !== link && it.outgoing.isNotEmpty() }

    private fun pump(link: Link) {
        if (link.busy || !link.subscribed || (terminal && notification != null)) return
        val body = link.outgoing.peek() ?: return
        val accepted = if (terminal) {
            val database = requireNotNull(server)
            val characteristic = requireNotNull(subscribe)
            if (Build.VERSION.SDK_INT >= 33)
                database.notifyCharacteristicChanged(link.device, characteristic, false, body) == BluetoothStatusCodes.SUCCESS
            else {
                @Suppress("DEPRECATION")
                characteristic.value = body
                @Suppress("DEPRECATION")
                database.notifyCharacteristicChanged(link.device, characteristic, false)
            }
        } else {
            val gatt = requireNotNull(link.gatt)
            val characteristic = requireNotNull(link.rx)
            if (Build.VERSION.SDK_INT >= 33)
                gatt.writeCharacteristic(characteristic, body, BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT) == BluetoothStatusCodes.SUCCESS
            else {
                characteristic.writeType = BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT
                @Suppress("DEPRECATION")
                characteristic.value = body
                @Suppress("DEPRECATION")
                gatt.writeCharacteristic(characteristic)
            }
        }
        if (!accepted) {
            require(++link.retry <= 20)
            handler.postDelayed({ guarded(link) { pump(link) } }, 25)
            return
        }
        link.retry = 0
        link.busy = true
        if (terminal) notification = link
        handler.postDelayed({
            if (links[link.id] === link && link.busy && link.outgoing.peek() === body) close(link.id)
        }, 3000)
    }

    private fun acknowledged(link: Link) {
        require(link.busy)
        link.bytes -= link.outgoing.remove().size
        link.busy = false
        val waiting = if (terminal) otherLink(link) else null
        if (waiting != null) {
            guarded(waiting) { pump(waiting) }
            guarded(link) { pump(link) }
        } else pump(link)
    }

    fun write(id: String, clear: String) {
        val link = requireNotNull(links[id])
        try {
            require(clear.length <= 4096)
            // Retire the untouched scan handles at the application-write boundary.
            if (payment) candidates.reset()
            received(link, link.tls.send(Base64.decode(clear, Base64.NO_WRAP)))
        } catch (failure: Throwable) { close(id); throw failure }
    }

    fun close(id: String) {
        val link = links.remove(id) ?: return
        nearbyServerLinks.remove(link.device)
        if (notification === link) {
            notification = null
            otherLink(link)?.let { guarded(it) { pump(it) } }
        }
        if (terminal) runCatching { server?.cancelConnection(link.device) }
        else runCatching { link.gatt?.disconnect(); link.gatt?.close() }
        link.outgoing.clear()
        sendEvent(id, "closed")
    }

    fun stop() {
        val started = startFinish
        startFinish = null
        started?.invoke(Result.failure(IllegalStateException("Nearby advertising unavailable")))
        stopScan()
        scanGeneration++
        candidates.reset()
        links.keys.toList().forEach(::close)
        advert?.let { runCatching { adapter.bluetoothLeAdvertiser?.stopAdvertising(it) } }
        advert = null
        runCatching { server?.close() }
        server = null
        subscribe = null
    }

    private fun sendEvent(link: String, kind: String, data: JSONObject = JSONObject()) =
        event(data.put("link", link).put("kind", kind).toString())

    private fun base64(data: ByteArray) = Base64.encodeToString(data, Base64.NO_WRAP)

    companion object {
        val SERVICE: UUID = UUID.fromString("41ba05a0-e0dd-4fd7-901c-4c25d5020000")
        val RX: UUID = UUID.fromString("41ba05a0-e0dd-4fd7-901c-4c25d5020001")
        val TX: UUID = UUID.fromString("41ba05a0-e0dd-4fd7-901c-4c25d5020002")
        val BOARD_SERVICE: UUID = UUID.fromString("41ba05a0-e0dd-4fd7-901c-4c25d5010000")
        val BOARD_RX: UUID = UUID.fromString("41ba05a0-e0dd-4fd7-901c-4c25d5010001")
        val BOARD_TX: UUID = UUID.fromString("41ba05a0-e0dd-4fd7-901c-4c25d5010002")
        val CCC: UUID = UUID.fromString("00002902-0000-1000-8000-00805f9b34fb")
    }
}
