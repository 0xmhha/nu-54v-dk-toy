import type { TurboModule } from 'react-native';
import { TurboModuleRegistry } from 'react-native';

export interface Spec extends TurboModule {
  randomBytes(size: number): Promise<string>;
  loadJournal(name: string): Promise<string>;
  saveJournal(name: string, value: string): Promise<void>;
  publicIdentity(slot: string): Promise<string>;
  configure(slot: string, profile: string): Promise<void>;
  loadProfile(slot: string): Promise<string>;
  startTravelRecording(): Promise<void>;
  finishTravelRecording(): Promise<string>;
  discardTravelRecording(): Promise<void>;
  exchangeRenterReturn(profileDigest: string, assignment: string, account: string): Promise<string>;
  exchangeTravel(method: string, path: string, envelope: string): Promise<string>;
  exchangeScoped(slot: string, profileDigest: string, envelope: string): Promise<string>;
  exchange(slot: string, envelope: string): Promise<string>;
  /** One POST to the customer entrance: no client certificate, trusted by the enrolled customer profile's CA and pin only. Answers `{"status":n,"body":"…"}`. */
  customerRequest(origin: string, path: string, body: string): Promise<string>;
}

export default TurboModuleRegistry.get<Spec>('CommerceHttp');
