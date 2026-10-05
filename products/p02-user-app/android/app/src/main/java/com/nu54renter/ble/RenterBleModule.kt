package com.nu54renter.ble

import android.annotation.SuppressLint
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothGatt
import android.bluetooth.BluetoothGattCallback
import android.bluetooth.BluetoothGattCharacteristic
import android.bluetooth.BluetoothGattDescriptor
import android.bluetooth.BluetoothManager
import android.bluetooth.BluetoothProfile
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanFilter
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.ParcelUuid
import android.util.Base64
import androidx.core.content.ContextCompat
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import org.json.JSONObject
import java.security.SecureRandom
import java.util.ArrayDeque
import java.util.UUID

/**
 * The renter app's link to the device (P02 design 1): a scan for devices in pairing mode, LE Secure
 * Connections bonding (Just Works for a device without a key, Passkey Entry after setup), and a
 * GATT connection to bonded devices only (P02-FR-02). The device sends confirm.show and the
 * forwarded payment.outcome over TX notifications; envelope, reassembly and CBOR are in
 * TypeScript. The passkey is used for one bonding and not kept.
 */
@SuppressLint("MissingPermission")
class RenterBleModule(private val context: ReactApplicationContext) : NativeRenterBleSpec(context) {
  private val manager get() = context.getSystemService(Context.BLUETOOTH_SERVICE) as BluetoothManager

  private var gatt: BluetoothGatt? = null
  private var address: String? = null
  private var rx: BluetoothGattCharacteristic? = null
  private var mtu = 23
  private var connecting: Promise? = null
  private var ids: Triple<UUID, UUID, UUID>? = null
  private val writes = ArrayDeque<Pair<ByteArray, Promise>>()
  private var writing: Promise? = null

  // ------------------------------------------------------------------ bonding

  private var bonding: Pair<String, Promise>? = null
  private var passkey: String? = null

  private val pairing = object : BroadcastReceiver() {
    override fun onReceive(c: Context, intent: Intent) {
      val device = deviceOf(intent) ?: return
      val target = bonding?.first ?: return
      if (!device.address.equals(target, ignoreCase = true)) return
      when (intent.action) {
        BluetoothDevice.ACTION_PAIRING_REQUEST -> {
          // Passkey Entry: answer with the passkey the app holds. Without one (or if the stack
          // refuses) the system dialog stays up and the renter confirms or types the code.
          passkey?.takeIf { it.isNotEmpty() }?.let { device.setPin(it.toByteArray()) }
        }
        BluetoothDevice.ACTION_BOND_STATE_CHANGED -> {
          val state = intent.getIntExtra(BluetoothDevice.EXTRA_BOND_STATE, BluetoothDevice.ERROR)
          val prev = intent.getIntExtra(BluetoothDevice.EXTRA_PREVIOUS_BOND_STATE, BluetoothDevice.ERROR)
          if (state == BluetoothDevice.BOND_BONDED) finishBond(true)
          else if (state == BluetoothDevice.BOND_NONE && prev == BluetoothDevice.BOND_BONDING) finishBond(false)
          else if (state == BluetoothDevice.BOND_NONE && device.address.equals(address, ignoreCase = true)) emitOnBondLost(device.address)
        }
      }
    }
  }

  private fun deviceOf(intent: Intent): BluetoothDevice? =
    if (Build.VERSION.SDK_INT >= 33) intent.getParcelableExtra(BluetoothDevice.EXTRA_DEVICE, BluetoothDevice::class.java)
    else @Suppress("DEPRECATION") intent.getParcelableExtra(BluetoothDevice.EXTRA_DEVICE)

  init {
    val filter = IntentFilter().apply {
      addAction(BluetoothDevice.ACTION_PAIRING_REQUEST)
      addAction(BluetoothDevice.ACTION_BOND_STATE_CHANGED)
      priority = IntentFilter.SYSTEM_HIGH_PRIORITY - 1
    }
    ContextCompat.registerReceiver(context, pairing, filter, ContextCompat.RECEIVER_EXPORTED)
  }

  private fun finishBond(ok: Boolean) {
    val p = synchronized(this) { bonding.also { bonding = null; passkey = null } }
    p?.second?.resolve(ok)
  }

  override fun bond(address: String, passkey: String, promise: Promise) {
    val device = try {
      manager.adapter.getRemoteDevice(address)
    } catch (e: IllegalArgumentException) {
      return promise.reject("BAD_ADDRESS", e.message)
    }
    if (device.bondState == BluetoothDevice.BOND_BONDED) return promise.resolve(true)
    synchronized(this) {
      bonding?.second?.reject("REPLACED", "another bonding started")
      bonding = address to promise
      this.passkey = passkey
    }
    if (!device.createBond()) finishBond(false)
  }

  override fun removeBond(address: String, promise: Promise) {
    val device = try {
      manager.adapter.getRemoteDevice(address)
    } catch (e: IllegalArgumentException) {
      return promise.reject("BAD_ADDRESS", e.message)
    }
    if (device.bondState == BluetoothDevice.BOND_NONE) return promise.resolve(true)
    // No public API removes a bond; the hidden one is what the system settings call. When it is
    // not reachable the app sends the renter to the Bluetooth settings instead.
    val ok = try {
      device.javaClass.getMethod("removeBond").invoke(device) as Boolean
    } catch (e: Exception) {
      false
    }
    promise.resolve(ok)
  }

  override fun isBonded(address: String, promise: Promise) {
    promise.resolve(try {
      manager.adapter.getRemoteDevice(address).bondState == BluetoothDevice.BOND_BONDED
    } catch (e: IllegalArgumentException) {
      false
    })
  }

  // ------------------------------------------------------------------ scan

  private var scanning: Promise? = null
  private val main = Handler(Looper.getMainLooper())
  private val stopLater = Runnable { stopScanning() }

  private val scanCallback = object : ScanCallback() {
    override fun onScanResult(type: Int, result: ScanResult) {
      val record = result.scanRecord ?: return
      // Pairing mode only: the device sets LE Limited Discoverable there and not in payment mode.
      if (record.advertiseFlags < 0 || record.advertiseFlags and 0x01 == 0) return
      val json = JSONObject()
        .put("address", result.device.address)
        .put("name", record.deviceName ?: result.device.name ?: "")
        .put("rssi", result.rssi)
      emitOnScan(json.toString())
    }

    override fun onScanFailed(errorCode: Int) {
      val p = synchronized(this@RenterBleModule) { scanning.also { scanning = null } }
      p?.reject("SCAN_FAILED", "scan failed ($errorCode)")
    }
  }

  override fun scan(service: String, seconds: Double, promise: Promise) {
    val scanner = manager.adapter?.bluetoothLeScanner ?: return promise.reject("NO_BLUETOOTH", "Bluetooth is off")
    stopScanning()
    synchronized(this) { scanning = promise }
    val filter = ScanFilter.Builder().setServiceUuid(ParcelUuid(UUID.fromString(service))).build()
    val settings = ScanSettings.Builder().setScanMode(ScanSettings.SCAN_MODE_LOW_LATENCY).build()
    scanner.startScan(listOf(filter), settings, scanCallback)
    main.postDelayed(stopLater, (seconds * 1000).toLong())
  }

  override fun stopScan(promise: Promise) {
    stopScanning()
    promise.resolve(null)
  }

  private fun stopScanning() {
    main.removeCallbacks(stopLater)
    val p = synchronized(this) { scanning.also { scanning = null } } ?: return
    try {
      manager.adapter?.bluetoothLeScanner?.stopScan(scanCallback)
    } catch (e: IllegalStateException) {
      // Bluetooth went off: the scan is over anyway
    }
    p.resolve(null)
  }

  // ------------------------------------------------------------------ randomness and storage

  private val random = SecureRandom()

  override fun randomBytes(n: Double, promise: Promise) {
    val b = ByteArray(n.toInt())
    random.nextBytes(b)
    promise.resolve(Base64.encodeToString(b, Base64.NO_WRAP))
  }

  // The registered devices: BLE address, name and wallet address. Nothing secret.
  private val prefs get() = context.getSharedPreferences("nu54-renter", Context.MODE_PRIVATE)

  override fun loadDevices(promise: Promise) {
    promise.resolve(prefs.getString("devices", "") ?: "")
  }

  override fun saveDevices(json: String, promise: Promise) {
    prefs.edit().putString("devices", json).apply()
    promise.resolve(null)
  }

  // ------------------------------------------------------------------ link

  override fun connect(address: String, service: String, rx: String, tx: String, promise: Promise) {
    val device = try {
      manager.adapter.getRemoteDevice(address)
    } catch (e: IllegalArgumentException) {
      return promise.reject("BAD_ADDRESS", e.message)
    }
    if (device.bondState != BluetoothDevice.BOND_BONDED) return promise.reject("NOT_BONDED", "bond with the device first")
    close()
    connecting = promise
    this.address = address
    ids = Triple(UUID.fromString(service), UUID.fromString(rx), UUID.fromString(tx))
    gatt = device.connectGatt(context, true, callback, BluetoothDevice.TRANSPORT_LE) // auto-reconnect
  }

  override fun writeFragment(base64: String, promise: Promise) {
    val data = Base64.decode(base64, Base64.NO_WRAP)
    synchronized(this) {
      if (gatt == null || rx == null) return promise.reject("NOT_CONNECTED", "no device connected")
      writes.add(data to promise)
    }
    pump()
  }

  override fun disconnect(promise: Promise) {
    close()
    promise.resolve(null)
  }

  private fun pump() {
    val (data, promise) = synchronized(this) {
      if (writing != null || writes.isEmpty()) return
      writes.poll()!!.also { writing = it.second }
    }
    val g = gatt
    val c = rx
    val started = when {
      g == null || c == null -> false
      Build.VERSION.SDK_INT >= 33 -> g.writeCharacteristic(c, data, BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT) == BluetoothGatt.GATT_SUCCESS
      else -> {
        @Suppress("DEPRECATION")
        c.value = data
        @Suppress("DEPRECATION")
        g.writeCharacteristic(c)
      }
    }
    if (!started) {
      synchronized(this) { writing = null }
      promise.reject("WRITE_FAILED", "could not start the write")
      pump()
    }
  }

  @Synchronized
  private fun close() {
    gatt?.disconnect()
    gatt?.close()
    gatt = null
    rx = null
    writing?.reject("DISCONNECTED", "link closed")
    writing = null
    while (writes.isNotEmpty()) writes.poll()!!.second.reject("DISCONNECTED", "link closed")
    connecting?.reject("DISCONNECTED", "link closed")
    connecting = null
  }

  private fun failConnect(reason: String) {
    val p = synchronized(this) { connecting.also { connecting = null } }
    close()
    p?.reject("CONNECT_FAILED", reason)
  }

  private val callback = object : BluetoothGattCallback() {
    override fun onConnectionStateChange(g: BluetoothGatt, status: Int, state: Int) {
      if (state == BluetoothProfile.STATE_CONNECTED && status == BluetoothGatt.GATT_SUCCESS) {
        g.requestMtu(247)
      } else if (state == BluetoothProfile.STATE_DISCONNECTED) {
        if (connecting != null) failConnect("disconnected while connecting (status $status)")
        else emitOnDisconnect("status $status") // connectGatt(autoConnect) reconnects by itself
      }
    }

    override fun onMtuChanged(g: BluetoothGatt, value: Int, status: Int) {
      mtu = if (status == BluetoothGatt.GATT_SUCCESS) value else 23
      if (!g.discoverServices()) failConnect("service discovery did not start")
    }

    override fun onServicesDiscovered(g: BluetoothGatt, status: Int) {
      val (s, r, t) = ids ?: return failConnect("no ids")
      val service = g.getService(s) ?: return failConnect("payment service not found")
      rx = service.getCharacteristic(r) ?: return failConnect("RX characteristic not found")
      val txChar = service.getCharacteristic(t) ?: return failConnect("TX characteristic not found")
      g.setCharacteristicNotification(txChar, true)
      val cccd = txChar.getDescriptor(CCCD) ?: return failConnect("TX has no notification descriptor")
      val ok = if (Build.VERSION.SDK_INT >= 33) {
        g.writeDescriptor(cccd, BluetoothGattDescriptor.ENABLE_NOTIFICATION_VALUE) == BluetoothGatt.GATT_SUCCESS
      } else {
        @Suppress("DEPRECATION")
        cccd.value = BluetoothGattDescriptor.ENABLE_NOTIFICATION_VALUE
        @Suppress("DEPRECATION")
        g.writeDescriptor(cccd)
      }
      if (!ok) failConnect("could not subscribe to TX")
    }

    override fun onDescriptorWrite(g: BluetoothGatt, d: BluetoothGattDescriptor, status: Int) {
      if (status != BluetoothGatt.GATT_SUCCESS) return failConnect("TX subscription refused ($status)")
      val p = synchronized(this@RenterBleModule) { connecting.also { connecting = null } }
      p?.resolve(mtu)
    }

    override fun onCharacteristicWrite(g: BluetoothGatt, c: BluetoothGattCharacteristic, status: Int) {
      val p = synchronized(this@RenterBleModule) { writing.also { writing = null } }
      if (status == BluetoothGatt.GATT_SUCCESS) p?.resolve(null) else p?.reject("WRITE_FAILED", "status $status")
      pump()
    }

    override fun onCharacteristicChanged(g: BluetoothGatt, c: BluetoothGattCharacteristic, value: ByteArray) {
      emitOnFragment(Base64.encodeToString(value, Base64.NO_WRAP))
    }

    @Deprecated("Called before API 33")
    override fun onCharacteristicChanged(g: BluetoothGatt, c: BluetoothGattCharacteristic) {
      @Suppress("DEPRECATION")
      if (Build.VERSION.SDK_INT < 33) emitOnFragment(Base64.encodeToString(c.value, Base64.NO_WRAP))
    }
  }

  override fun invalidate() {
    stopScanning()
    close()
    try {
      context.unregisterReceiver(pairing)
    } catch (e: IllegalArgumentException) {
      // not registered
    }
    super.invalidate()
  }

  companion object {
    private val CCCD: UUID = UUID.fromString("00002902-0000-1000-8000-00805f9b34fb")
  }
}
