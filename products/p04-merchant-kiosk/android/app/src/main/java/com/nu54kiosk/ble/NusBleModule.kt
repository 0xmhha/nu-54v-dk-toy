package com.nu54kiosk.ble

import android.annotation.SuppressLint
import android.bluetooth.BluetoothDevice
import android.bluetooth.BluetoothGatt
import android.bluetooth.BluetoothGattCallback
import android.bluetooth.BluetoothGattCharacteristic
import android.bluetooth.BluetoothGattDescriptor
import android.bluetooth.BluetoothManager
import android.bluetooth.BluetoothProfile
import android.bluetooth.le.ScanCallback
import android.bluetooth.le.ScanResult
import android.bluetooth.le.ScanSettings
import android.content.Context
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.ParcelUuid
import android.util.Base64
import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import java.util.ArrayDeque
import java.util.UUID

/**
 * BLE central for the kiosk (payment-protocol.md 3): scan for the payment service, connect
 * without pairing (N27), negotiate the ATT MTU, subscribe to TX and write RX fragments.
 *
 * This module moves raw fragments only; envelope, reassembly and CBOR are in TypeScript
 * (src/ble/framing.ts). Android allows one GATT operation at a time, so writes are queued.
 * Runtime permissions (BLUETOOTH_SCAN, BLUETOOTH_CONNECT) are requested by the app before use.
 */
@SuppressLint("MissingPermission")
class NusBleModule(private val context: ReactApplicationContext) : NativeNusBleSpec(context) {
  private val main = Handler(Looper.getMainLooper())
  private val manager get() = context.getSystemService(Context.BLUETOOTH_SERVICE) as BluetoothManager

  private var gatt: BluetoothGatt? = null
  private var rx: BluetoothGattCharacteristic? = null
  private var mtu = 23
  private var connecting: Promise? = null
  private var ids: Triple<UUID, UUID, UUID>? = null
  private val writes = ArrayDeque<Pair<ByteArray, Promise>>()
  private var writing: Promise? = null

  override fun scan(service: String, timeoutMs: Double, promise: Promise) {
    val scanner = manager.adapter?.bluetoothLeScanner
    if (scanner == null || manager.adapter?.isEnabled != true) {
      promise.reject("BLE_OFF", "Bluetooth is off or unavailable")
      return
    }
    val wanted = ParcelUuid(UUID.fromString(service))
    var best: ScanResult? = null
    val callback = object : ScanCallback() {
      override fun onScanResult(type: Int, r: ScanResult) {
        // Match on the advertised service, or on the name when the UUID went to the scan
        // response a peripheral (for example macOS) did not fit into the advertisement.
        val uuids = r.scanRecord?.serviceUuids ?: emptyList()
        val name = r.scanRecord?.deviceName ?: ""
        if (wanted in uuids || name.startsWith("NU54")) {
          synchronized(this@NusBleModule) { if (best == null || r.rssi > best!!.rssi) best = r }
        }
      }

      override fun onScanFailed(code: Int) {
        promise.reject("SCAN_FAILED", "scan failed ($code)")
      }
    }
    val settings = ScanSettings.Builder().setScanMode(ScanSettings.SCAN_MODE_LOW_LATENCY).build()
    scanner.startScan(null, settings, callback)
    main.postDelayed({
      scanner.stopScan(callback)
      val r = synchronized(this) { best }
      if (r == null) {
        promise.reject("NOT_FOUND", "no device in payment mode nearby")
      } else {
        promise.resolve(Arguments.createMap().apply {
          putString("address", r.device.address)
          putString("name", r.scanRecord?.deviceName ?: r.device.name ?: "")
          putInt("rssi", r.rssi)
        })
      }
    }, timeoutMs.toLong())
  }

  override fun connect(address: String, service: String, rx: String, tx: String, promise: Promise) {
    val device: BluetoothDevice = try {
      manager.adapter.getRemoteDevice(address)
    } catch (e: IllegalArgumentException) {
      promise.reject("BAD_ADDRESS", e.message)
      return
    }
    close()
    connecting = promise
    ids = Triple(UUID.fromString(service), UUID.fromString(rx), UUID.fromString(tx))
    gatt = device.connectGatt(context, false, callback, BluetoothDevice.TRANSPORT_LE)
  }

  override fun writeFragment(base64: String, promise: Promise) {
    val data = Base64.decode(base64, Base64.NO_WRAP)
    synchronized(this) {
      if (gatt == null || this.rx == null) {
        promise.reject("NOT_CONNECTED", "no device connected")
        return
      }
      writes.add(data to promise)
    }
    pump()
  }

  override fun disconnect(promise: Promise) {
    close()
    promise.resolve(null)
  }

  /** Starts the next queued write when none is in flight. */
  private fun pump() {
    val (data, promise) = synchronized(this) {
      if (writing != null || writes.isEmpty()) return
      writes.poll()!!.also { writing = it.second }
    }
    val g = gatt
    val c = rx
    val started = if (g == null || c == null) {
      false
    } else if (Build.VERSION.SDK_INT >= 33) {
      g.writeCharacteristic(c, data, BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT) == BluetoothGatt.GATT_SUCCESS
    } else {
      @Suppress("DEPRECATION")
      c.writeType = BluetoothGattCharacteristic.WRITE_TYPE_DEFAULT
      @Suppress("DEPRECATION")
      c.value = data
      @Suppress("DEPRECATION")
      g.writeCharacteristic(c)
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
        g.requestMtu(247) // the largest the protocol uses (payment-protocol.md 4)
      } else if (state == BluetoothProfile.STATE_DISCONNECTED) {
        if (connecting != null) {
          failConnect("disconnected while connecting (status $status)")
        } else if (g == gatt) {
          close()
          emitOnDisconnect("status $status")
        }
      }
    }

    override fun onMtuChanged(g: BluetoothGatt, value: Int, status: Int) {
      mtu = if (status == BluetoothGatt.GATT_SUCCESS) value else 23
      if (!g.discoverServices()) failConnect("service discovery did not start")
    }

    override fun onServicesDiscovered(g: BluetoothGatt, status: Int) {
      val (s, r, t) = ids ?: return failConnect("no ids")
      val service = g.getService(s) ?: return failConnect("payment service not found")
      val rxChar = service.getCharacteristic(r) ?: return failConnect("RX characteristic not found")
      val txChar = service.getCharacteristic(t) ?: return failConnect("TX characteristic not found")
      rx = rxChar
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
      val p = synchronized(this@NusBleModule) { connecting.also { connecting = null } }
      p?.resolve(mtu)
    }

    override fun onCharacteristicWrite(g: BluetoothGatt, c: BluetoothGattCharacteristic, status: Int) {
      val p = synchronized(this@NusBleModule) { writing.also { writing = null } }
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
    close()
    super.invalidate()
  }

  companion object {
    private val CCCD: UUID = UUID.fromString("00002902-0000-1000-8000-00805f9b34fb")
  }
}
