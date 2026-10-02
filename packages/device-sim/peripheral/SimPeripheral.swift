// BLE peripheral on the Mac for the software payment device (payment-protocol.md 3).
//
// It advertises the payment GATT service and passes raw fragments between a central (the
// kiosk app on a phone) and the program that started it (packages/device-sim/scripts/
// serve-ble.ts), which does the envelope, reassembly and the device itself. This file only
// knows GATT: it never parses a fragment.
//
// Several centrals may be subscribed at once (the kiosk and the renter's phone app); each event
// names its central, and a fragment goes to one central.
//
//   stdin   {"tx": "<fragment hex>", "central": "<id>"}   notify one fragment to that central
//   stdout  {"event": "advertising", "name": "..."}
//           {"event": "connected", "central": "...", "mtu": 185}   (on TX subscription)
//           {"event": "disconnected", "central": "..."}
//           {"rx": "<fragment hex>", "central": "..."}  one write to the RX characteristic
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
    var centrals: [String: CBCentral] = [:]
    /// Fragments waiting for the notification queue to drain (updateValue returned false), in order.
    var pending: [(Data, CBCentral)] = []

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
        let id = c.identifier.uuidString
        centrals[id] = c
        pending.removeAll { $0.1.identifier == c.identifier }
        emit(["event": "connected", "central": id, "mtu": c.maximumUpdateValueLength + 3])
    }

    func peripheralManager(_ p: CBPeripheralManager, central c: CBCentral, didUnsubscribeFrom ch: CBCharacteristic) {
        let id = c.identifier.uuidString
        centrals[id] = nil
        pending.removeAll { $0.1.identifier == c.identifier }
        emit(["event": "disconnected", "central": id])
    }

    func peripheralManager(_ p: CBPeripheralManager, didReceiveWrite requests: [CBATTRequest]) {
        // CoreBluetooth answers the whole batch with the first request's result.
        for r in requests where r.characteristic.uuid == rxUUID {
            emit(["rx": hex(r.value ?? Data()), "central": r.central.identifier.uuidString])
        }
        p.respond(to: requests[0], withResult: .success)
    }

    func notify(_ fragment: Data, to id: String) {
        guard let c = centrals[id] else { emit(["error": "central \(id) is not subscribed; fragment dropped"]); return }
        if fragment.count > c.maximumUpdateValueLength {
            emit(["error": "fragment of \(fragment.count) bytes exceeds the notification size \(c.maximumUpdateValueLength)"])
            return
        }
        pending.append((fragment, c))
        drain()
    }

    func drain() {
        while let (f, c) = pending.first {
            guard manager.updateValue(f, for: tx, onSubscribedCentrals: [c]) else { return } // resumes below
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
        if let t = obj["tx"] as? String, let f = unhex(t), let id = obj["central"] as? String {
            DispatchQueue.main.async { peripheral.notify(f, to: id) }
        }
    }
    DispatchQueue.main.async { exit(0) } // the parent closed stdin
}.start()

dispatchMain()
