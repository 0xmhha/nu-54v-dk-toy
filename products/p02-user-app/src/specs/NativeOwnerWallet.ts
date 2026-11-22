import type {CodegenTypes, TurboModule} from 'react-native';
import {TurboModuleRegistry} from 'react-native';

export interface Spec extends TurboModule {
  getNetworkLink(): Promise<string>;
  readonly onNetworkLink: CodegenTypes.EventEmitter<string>;
  setHomeProtection(enabled: boolean): Promise<void>;
  loadProfile(): Promise<string>;
  saveProfile(value: string): Promise<void>;
  compareAndSaveProfile(expected: string, value: string): Promise<boolean>;
  publicIdentity(): Promise<string>;
  signChallenge(record: string): Promise<string>;
  randomBytes(size: number): Promise<string>;
  installationClock(): Promise<string>;
  pickInstallationFile(): Promise<string>;
  pickPsbtFile(): Promise<string>;
  speak(text: string, language: string): Promise<void>;
  startTls(caDer: string, clientChain: ReadonlyArray<string>, boardSPKI: string): Promise<string>;
  feedTls(input: string): Promise<string>;
  sendTls(cleartext: string): Promise<string>;
  stopTls(): Promise<void>;
  evmRpc(endpoint: string, packet: string): Promise<string>;
  readonly onTls: CodegenTypes.EventEmitter<string>;
}

export default TurboModuleRegistry.get<Spec>('OwnerWallet');
