import type {TurboModule} from 'react-native';
import {TurboModuleRegistry} from 'react-native';

export interface Spec extends TurboModule {
  walletAddress(scope: string, create: boolean): Promise<string>;
  deleteWallet(scope: string): Promise<boolean>;
  sign(scope: string, target: string, nonce: number, gasPrice: number, data: string, quote: string): Promise<string>;
  deploymentPolicy(): Promise<string>;
  operatorOrigin(): Promise<string>;
  exchange(path: string, json: string): Promise<string>;
}

export default TurboModuleRegistry.get<Spec>('MarketLab');
