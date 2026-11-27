import type {CodegenTypes, TurboModule} from 'react-native';
import {TurboModuleRegistry} from 'react-native';

export interface Spec extends TurboModule {
  publicIdentity(): Promise<string>;
  configure(enrollment: string): Promise<void>;
  scan(seconds: number): Promise<string>;
  connect(handle: string): Promise<string>;
  start(): Promise<void>;
  stop(): Promise<void>;
  close(link: string): Promise<void>;
  write(link: string, clear: string): Promise<void>;
  paymentPublicIdentity(): Promise<string>;
  configurePayment(enrollment: string): Promise<void>;
  scanPayment(seconds: number): Promise<string>;
  connectPayment(handle: string): Promise<string>;
  stopPayment(): Promise<void>;
  closePayment(link: string): Promise<void>;
  writePayment(link: string, clear: string): Promise<void>;
  readonly onPayment: CodegenTypes.EventEmitter<string>;
  readonly onNearby: CodegenTypes.EventEmitter<string>;
}

export default TurboModuleRegistry.get<Spec>('Nearby');
