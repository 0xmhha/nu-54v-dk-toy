// Turbo Module spec for the kiosk BLE central ([N09][N25]).
// Android implements it in Kotlin, iOS in Swift; codegen reads this file.
import type { TurboModule } from "react-native";
import { TurboModuleRegistry } from "react-native";

export interface Spec extends TurboModule {
  /** Scan for the NUS service UUID and resolve the first device address found. */
  scan(timeoutMs: number): Promise<string>;
  /** Connect, pair with LE Secure Connections and negotiate the ATT MTU; resolves the MTU. */
  connect(address: string): Promise<number>;
  /** Write one protocol fragment (base64) to the RX characteristic. */
  writeFragment(base64: string): Promise<void>;
  /** Subscribe to TX notifications; fragments arrive as `nusFragment` events. */
  startNotifications(): Promise<void>;
  disconnect(): Promise<void>;
  addListener(eventName: string): void;
  removeListeners(count: number): void;
}

export default TurboModuleRegistry.getEnforcing<Spec>("NusBle");
