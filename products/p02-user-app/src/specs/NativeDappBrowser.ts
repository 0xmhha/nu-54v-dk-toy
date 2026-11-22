import type {CodegenTypes,TurboModule} from 'react-native';
import {TurboModuleRegistry} from 'react-native';

export interface Spec extends TurboModule {
  open(url:string,session:string,provider:string):Promise<void>;
  confirm(session:string,document:string,title:string,message:string):Promise<boolean>;
  confirmAllowance(session:string,document:string,title:string,message:string):Promise<string>;
  close():Promise<void>;
  respond(session:string,document:string,reply:string):Promise<void>;
  readonly onRequest:CodegenTypes.EventEmitter<string>;
  readonly onClosed:CodegenTypes.EventEmitter<string>;
}

export default TurboModuleRegistry.get<Spec>('DappBrowser');
