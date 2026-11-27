import type { TurboModule } from 'react-native';
import { TurboModuleRegistry } from 'react-native';

export interface Spec extends TurboModule {
  permission(ask: boolean): Promise<boolean>;
  schedule(invoice: string, epochMilliseconds: number, expiresMilliseconds: number): Promise<boolean>;
  remove(invoice: string): Promise<void>;
  clear(): Promise<void>;
  /** Shows at once that an order is ready, without naming it. False when notifications are off. */
  orderReady(order: string): Promise<boolean>;
}

export default TurboModuleRegistry.get<Spec>('SubscriptionAlerts');
