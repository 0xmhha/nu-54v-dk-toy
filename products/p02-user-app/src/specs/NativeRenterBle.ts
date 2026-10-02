// Turbo Module spec for the renter app's bonded link to the device ([N26][N27]).
// Android implements it in Kotlin (android/app/src/main/java/com/nu54renter/ble/RenterBleModule.kt).
import type { CodegenTypes, TurboModule } from "react-native";
import { TurboModuleRegistry } from "react-native";

export interface Spec extends TurboModule {
  /**
   * Bonds with the device at `address` using LE Secure Connections Passkey Entry, answering the
   * pairing request with the label passkey. Resolves true once the bond is stored. The device
   * must be in pairing mode (long press).
   */
  bond(address: string, passkey: string): Promise<boolean>;
  isBonded(address: string): Promise<boolean>;
  /** Connects to a bonded device only, subscribes to TX; resolves the ATT_MTU. */
  connect(address: string, service: string, rx: string, tx: string): Promise<number>;
  writeFragment(base64: string): Promise<void>;
  disconnect(): Promise<void>;
  readonly onFragment: CodegenTypes.EventEmitter<string>;
  readonly onDisconnect: CodegenTypes.EventEmitter<string>;
  /** The bond for the connected device was removed (for example after a return). */
  readonly onBondLost: CodegenTypes.EventEmitter<string>;
}

export default TurboModuleRegistry.getEnforcing<Spec>("RenterBle");
