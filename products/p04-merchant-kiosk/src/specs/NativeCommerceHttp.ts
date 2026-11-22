import type { TurboModule } from 'react-native';
import { TurboModuleRegistry } from 'react-native';

export interface Spec extends TurboModule {
  randomBytes(size: number): Promise<string>;
  loadJournal(name: string): Promise<string>;
  saveJournal(name: string, value: string): Promise<void>;
  publicIdentity(slot: string): Promise<string>;
  configure(slot: string, profile: string): Promise<void>;
  loadProfile(slot: string): Promise<string>;
  exchange(slot: string, envelope: string): Promise<string>;
  exchangeRenterV1Origin(operation:string, profileDigest:string, body:string): Promise<string>;
  exchangePrint(
    operation: string,
    profileDigest: string,
    reference: string,
    body: string,
  ): Promise<string>;
  exchangeScoped(
    slot: string,
    profileDigest: string,
    envelope: string,
  ): Promise<string>;
}

export default TurboModuleRegistry.get<Spec>('CommerceHttp');
