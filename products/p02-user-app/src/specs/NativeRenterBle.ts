// Turbo Module spec for the renter app's bonded link to the device ([N26][N27]).
// Android implements it in Kotlin (android/app/src/main/java/com/nu54renter/ble/RenterBleModule.kt).
import type { CodegenTypes, TurboModule } from "react-native";
import { TurboModuleRegistry } from "react-native";

export interface Spec extends TurboModule {
  /**
   * Scans for devices in pairing mode: the payment service UUID with the LE Limited Discoverable
   * flag (payment-protocol.md 3). Each one found is reported on onScan as JSON
   * {address, name, rssi}. Resolves when the scan stops (after `seconds` or stopScan).
   */
  scan(service: string, seconds: number): Promise<void>;
  stopScan(): Promise<void>;
  /**
   * Bonds with the device at `address` (LE Secure Connections). With a passkey the pairing request
   * is answered with it (Passkey Entry); with "" the system asks the renter (Just Works consent for
   * a device without a key, or the reconnect code). Resolves true once the bond is stored. The
   * device must be in pairing mode.
   */
  bond(address: string, passkey: string): Promise<boolean>;
  isBonded(address: string): Promise<boolean>;
  /** Removes the phone's bond with the device; false when the system refused. */
  removeBond(address: string): Promise<boolean>;
  /** Connects to a bonded device only, subscribes to TX; resolves the ATT_MTU. */
  connect(address: string, service: string, rx: string, tx: string): Promise<number>;
  writeFragment(base64: string): Promise<void>;
  disconnect(): Promise<void>;
  /** n bytes from the platform's SecureRandom, base64. */
  randomBytes(n: number): Promise<string>;
  /** The registered devices (JSON text kept in app-private storage); "" when there are none. */
  loadDevices(): Promise<string>;
  saveDevices(json: string): Promise<void>;
  readonly onScan: CodegenTypes.EventEmitter<string>;
  readonly onFragment: CodegenTypes.EventEmitter<string>;
  readonly onDisconnect: CodegenTypes.EventEmitter<string>;
  /** The bond for the connected device was removed (for example after a return). */
  readonly onBondLost: CodegenTypes.EventEmitter<string>;
}

export default TurboModuleRegistry.getEnforcing<Spec>("RenterBle");
