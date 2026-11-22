import type { TurboModule } from 'react-native';
import { TurboModuleRegistry } from 'react-native';

export interface Spec extends TurboModule {
  scan(): Promise<string>;
}

export default TurboModuleRegistry.get<Spec>('ShopQr');
