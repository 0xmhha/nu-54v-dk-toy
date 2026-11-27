import type {TurboModule} from 'react-native';
import {TurboModuleRegistry} from 'react-native';

export interface Spec extends TurboModule {
  profile(): Promise<string>;
  startParticipant(subject: string, expires: number, token: string): Promise<boolean>;
  stopParticipant(): Promise<boolean>;
  prepareParticipant(subject: string): Promise<string>;
  installParticipant(subject: string, roster: string): Promise<boolean>;
  request(url: string, method: string, body: string, token: string): Promise<string>;
}

export default TurboModuleRegistry.get<Spec>('CloudWalletProfile');
