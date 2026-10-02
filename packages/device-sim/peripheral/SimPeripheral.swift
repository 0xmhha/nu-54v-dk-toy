// BLE peripheral on the Mac for the software payment device (payment-protocol.md 3).
//
// It advertises the payment GATT service and passes raw fragments between a central (the
// kiosk app on a phone) and the program that started it (packages/device-sim/scripts/
// serve-ble.ts), which does the envelope, reassembly and the device itself. This file only
// knows GATT: it never parses a fragment.
//
//   stdin   {"tx": "<fragment hex>"}                 notify one fragment on the TX characteristic
//   stdout  {"event": "advertising", "name": "..."}
//           {"event": "connected", "central": "...", "mtu": 185}   (on TX subscription)
//           {"event": "disconnected", "central": "..."}
//           {"rx": "<fragment hex>"}                  one write to the RX characteristic
//           {"error": "..."}
//
// mtu is the ATT_MTU: CoreBluetooth reports the largest notification value (ATT_MTU - 3).
//
// Usage: swift SimPeripheral.swift <name> <service uuid> <rx uuid> <tx uuid>

import CoreBluetooth
import Foundation

func emit(_ obj: [String: Any]) {
    let data = try! JSONSerialization.data(withJSONObject: obj)
    FileHandle.standardOutput.write(data + Data("\n".utf8))
}

func hex(_ d: Data) -> String { d.map { String(format: "%02x", $0) }.joined() }

func unhex(_ s: String) -> Data? {
    guard s.count % 2 == 0 else { return nil }
    var out = Data(capacity: s.count / 2)
    var i = s.startIndex
    while i < s.endIndex {
        let j = s.index(i, offsetBy: 2)
        guard let b = UInt8(s[i..<j], radix: 16) else { return nil }
        out.append(b)
        i = j
    }
    return out
}

final class Peripheral: NSObject, CBPeripheralManagerDelegate {
    let name: String
    let service: CBUUID, rxUUID: CBUUID, txUUID: CBUUID
    var manager: CBPeripheralManager!
    var tx: CBMutableCharacteristic!
    var central: CBCentral?
    /// Fragments waiting for the notification queue to drain (updateValue returned false).
    var pending: [Data] = []

    init(name: String, service: String, rx: String, tx: String) {
        self.name = name
        self.service = CBUUID(string: service)
        self.rxUUID = CBUUID(string: rx)
        self.txUUID = CBUUID(string: tx)
        super.init()
        manager = CBPeripheralManager(delegate: self, queue: DispatchQueue.main)
    }

    func peripheralManagerDidUpdateState(_ p: CBPeripheralManager) {
        guard p.state == .poweredOn else {
            if p.state == .unauthorized || p.state == .unsupported || p.state == .poweredOff {
                emit(["error": "Bluetooth is not available (state \(p.state.rawValue)); check that it is on and the terminal may use it"])
            }
            return
        }
        // Payment sessions run on an unpaired link (week 7); RX takes writes with response.
        let rx = CBMutableCharacteristic(type: rxUUID, properties: [.write], value: nil, permissions: [.writeable])
        tx = CBMutableCharacteristic(type: txUUID, properties: [.notify], value: nil, permissions: [.readable])
        let s = CBMutableService(type: service, primary: true)
        s.characteristics = [rx, tx]
        p.add(s)
    }

    func peripheralManager(_ p: CBPeripheralManager, didAdd s: CBService, error: Error?) {
        if let error { emit(["error": "add service: \(error.localizedDescription)"]); return }
        p.startAdvertising([CBAdvertisementDataLocalNameKey: name, CBAdvertisementDataServiceUUIDsKey: [service]])
    }

    func peripheralManagerDidStartAdvertising(_ p: CBPeripheralManager, error: Error?) {
        if let error { emit(["error": "advertise: \(error.localizedDescription)"]); return }
        emit(["event": "advertising", "name": name])
    }

    func peripheralManager(_ p: CBPeripheralManager, central c: CBCentral, didSubscribeTo ch: CBCharacteristic) {
        central = c
        pending.removeAll()
        emit(["event": "connected", "central": c.identifier.uuidString, "mtu": c.maximumUpdateValueLength + 3])
    }

    func peripheralManager(_ p: CBPeripheralManager, central c: CBCentral, didUnsubscribeFrom ch: CBCharacteristic) {
        if central?.identifier == c.identifier { central = nil }
        pending.removeAll()
        emit(["event": "disconnected", "central": c.identifier.uuidString])
    }

    func peripheralManager(_ p: CBPeripheralManager, didReceiveWrite requests: [CBATTRequest]) {
        // CoreBluetooth answers the whole batch with the first request's result.
        for r in requests where r.characteristic.uuid == rxUUID {
            emit(["rx": hex(r.value ?? Data())])
        }
        p.respond(to: requests[0], withResult: .success)
    }

    func notify(_ fragment: Data) {
        guard let c = central else { emit(["error": "no central subscribed; fragment dropped"]); return }
        if fragment.count > c.maximumUpdateValueLength {
            emit(["error": "fragment of \(fragment.count) bytes exceeds the notification size \(c.maximumUpdateValueLength)"])
            return
        }
        pending.append(fragment)
        drain()
    }

    func drain() {
        while let f = pending.first {
            guard manager.updateValue(f, for: tx, onSubscribedCentrals: nil) else { return } // resumes below
            pending.removeFirst()
        }
    }

    func peripheralManagerIsReady(toUpdateSubscribers p: CBPeripheralManager) { drain() }
}

let args = CommandLine.arguments
guard args.count == 5 else {
    FileHandle.standardError.write(Data("usage: SimPeripheral.swift <name> <service> <rx> <tx>\n".utf8))
    exit(2)
}
let peripheral = Peripheral(name: args[1], service: args[2], rx: args[3], tx: args[4])

// Commands arrive on stdin; CoreBluetooth calls must happen on the main queue.
Thread {
    while let line = readLine() {
        guard let obj = (try? JSONSerialization.jsonObject(with: Data(line.utf8))) as? [String: Any] else {
            emit(["error": "bad command: \(line)"])
            continue
        }
        if obj["stop"] != nil { DispatchQueue.main.async { exit(0) } }
        if let t = obj["tx"] as? String, let f = unhex(t) {
            DispatchQueue.main.async { peripheral.notify(f) }
        }
    }
    DispatchQueue.main.async { exit(0) } // the parent closed stdin
}.start()

dispatchMain()
