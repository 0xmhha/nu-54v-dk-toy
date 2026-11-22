import type { TurboModule } from 'react-native';
import { TurboModuleRegistry } from 'react-native';

export interface Spec extends TurboModule {
  permission(ask: boolean): Promise<boolean>;
  schedule(invoice: string, epochMilliseconds: number, expiresMilliseconds: number): Promise<boolean>;
  remove(invoice: string): Promise<void>;
  clear(): Promise<void>;
}

export default TurboModuleRegistry.get<Spec>('SubscriptionAlerts');
