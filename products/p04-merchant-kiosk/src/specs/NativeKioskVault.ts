// Turbo Module spec for the kiosk's key storage and secure random ([N32]).
// Android implements it in Kotlin (android/app/src/main/java/com/nu54kiosk/ble/KioskVaultModule.kt).
//
// Android Keystore does not hold secp256k1 keys, so the gas key and the merchant signing key
// are wrapped with a Keystore AES-GCM key and kept in app storage; signing happens in the app.
import type { TurboModule } from "react-native";
import { TurboModuleRegistry } from "react-native";

export interface Spec extends TurboModule {
  /** n bytes from SecureRandom, base64. */
  randomBytes(n: number): Promise<string>;
  /** Wraps a raw key (base64) with the Keystore key and stores it under alias. */
  wrapKey(alias: string, base64: string): Promise<void>;
  /** The raw key (base64) stored under alias, or null. */
  unwrapKey(alias: string): Promise<string | null>;
  /** Non-secret settings (JSON text). */
  putSetting(name: string, value: string): Promise<void>;
  getSetting(name: string): Promise<string | null>;
  /**
   * Reads and deletes a file the development provisioning put into the app's private files
   * directory (adb run-as). Null when there is none.
   */
  takeFile(name: string): Promise<string | null>;
}

export default TurboModuleRegistry.getEnforcing<Spec>("KioskVault");
