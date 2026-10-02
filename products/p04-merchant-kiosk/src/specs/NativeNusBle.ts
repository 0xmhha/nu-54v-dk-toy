// Turbo Module spec for the kiosk BLE central ([N09][N25][N27]).
// Android implements it in Kotlin (android/app/src/main/java/com/nu54kiosk/ble/NusBleModule.kt);
// codegen reads this file. The kiosk does not pair: payment sessions run on an unpaired link.
import type { CodegenTypes, TurboModule } from "react-native";
import { TurboModuleRegistry } from "react-native";

export type FoundDevice = {
  address: string;
  name: string;
  rssi: number;
};

export interface Spec extends TurboModule {
  /** Scans for the payment service UUID; resolves the strongest device seen within timeoutMs. */
  scan(service: string, timeoutMs: number): Promise<FoundDevice>;
  /**
   * Connects, negotiates the ATT MTU, finds the service and subscribes to TX notifications.
   * Resolves the negotiated ATT_MTU.
   */
  connect(address: string, service: string, rx: string, tx: string): Promise<number>;
  /** Writes one fragment (base64) to RX with response; writes are queued in order. */
  writeFragment(base64: string): Promise<void>;
  disconnect(): Promise<void>;
  /** One TX notification: a fragment, base64. */
  readonly onFragment: CodegenTypes.EventEmitter<string>;
  /** The link dropped (reason text). */
  readonly onDisconnect: CodegenTypes.EventEmitter<string>;
}

export default TurboModuleRegistry.getEnforcing<Spec>("NusBle");
