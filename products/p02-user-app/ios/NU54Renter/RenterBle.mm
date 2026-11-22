#import <CoreBluetooth/CoreBluetooth.h>
#import "OwnerRegistrationQA.h"
#if defined(NU54_OWNER_REGISTRATION_QA)
#import <Network/Network.h>
static NSString *const registrationHandle = @"owner-registration-host";
static NSString *const otherRegistrationHandle = @"owner-registration-other";
#endif
#import <Security/Security.h>
#import <NU54RenterSpec/NU54RenterSpec.h>

@interface RenterBle : NativeRenterBleSpecBase <NativeRenterBleSpec, CBCentralManagerDelegate, CBPeripheralDelegate>
@end

@implementation RenterBle {
  CBCentralManager *_central;
  NSMutableDictionary<NSString *, CBPeripheral *> *_nearby;
  CBPeripheral *_peer;
  CBCharacteristic *_input;
  CBUUID *_service;
  CBUUID *_writeID;
  CBUUID *_notifyID;
  RCTPromiseResolveBlock _scanResolve;
  RCTPromiseRejectBlock _scanReject;
  RCTPromiseResolveBlock _connectResolve;
  RCTPromiseRejectBlock _connectReject;
  RCTPromiseResolveBlock _writeResolve;
  RCTPromiseRejectBlock _writeReject;
  NSUInteger _scanToken;
#if defined(NU54_OWNER_REGISTRATION_QA)
  nw_connection_t _ownerTestConnection;
#endif
}

RCT_EXPORT_MODULE(RenterBle)

- (std::shared_ptr<facebook::react::TurboModule>)getTurboModule:
    (const facebook::react::ObjCTurboModule::InitParams &)params {
  return std::make_shared<facebook::react::NativeRenterBleSpecJSI>(params);
}

- (void)scan:(NSString *)service seconds:(double)seconds resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  reject(@"LEGACY_UNAVAILABLE", @"The legacy rental pairing flow requires a supported Android installation.", nil);
}

- (void)scanOwner:(NSString *)service seconds:(double)seconds resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(), ^{
#if defined(NU54_OWNER_REGISTRATION_QA)
    if (NU54OwnerRegistrationQA()) {
      NSString *fixtureOrder = [NSHomeDirectory() stringByAppendingPathComponent:@"Documents/w04-reverse-scan"];
      BOOL reverse = [[NSFileManager defaultManager] fileExistsAtPath:fixtureOrder];
      NSArray<NSString *> *handles = reverse ? @[registrationHandle, otherRegistrationHandle] : @[otherRegistrationHandle, registrationHandle];
      for (NSString *handle in handles) {
        NSData *discovery = [NSJSONSerialization dataWithJSONObject:@{
          @"address":handle, @"name":@"NU54 W04 QA", @"rssi":@(-40),
        } options:0 error:nil];
        [self emitOnScan:[[NSString alloc] initWithData:discovery encoding:NSUTF8StringEncoding]];
      }
      resolve(nil);
      return;
    }
#endif
    [self finishScan];
    self->_nearby = [NSMutableDictionary new];
    self->_service = [CBUUID UUIDWithString:service];
    self->_scanResolve = resolve;
    self->_scanReject = reject;
    if (!self->_central) self->_central = [[CBCentralManager alloc] initWithDelegate:self queue:nil];
    [self startIfReady];
    NSUInteger token = ++self->_scanToken;
    NSTimeInterval bounded = MAX(1, MIN(15, seconds));
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(bounded*NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
      if (self->_scanToken == token) [self finishScan];
    });
  });
}

- (void)startIfReady {
  if (!_scanResolve) return;
  if (_central.state == CBManagerStatePoweredOn) {
    [_central scanForPeripheralsWithServices:@[_service] options:nil];
  } else if (_central.state != CBManagerStateUnknown && _central.state != CBManagerStateResetting) {
    RCTPromiseRejectBlock reject = _scanReject;
    _scanResolve = nil;
    _scanReject = nil;
    reject(@"BLUETOOTH_UNAVAILABLE", @"Check Bluetooth access and power in Settings.", nil);
  }
}

- (void)centralManagerDidUpdateState:(CBCentralManager *)central {
  [self startIfReady];
  if (central.state != CBManagerStatePoweredOn && _peer) [self closePeer];
}

- (void)centralManager:(CBCentralManager *)central didDiscoverPeripheral:(CBPeripheral *)peripheral
     advertisementData:(NSDictionary<NSString *,id> *)advertisementData RSSI:(NSNumber *)RSSI {
  if (!_scanResolve || _nearby.count>=32) return;
  NSString *handle = peripheral.identifier.UUIDString;
  _nearby[handle] = peripheral;
  NSData *data = [NSJSONSerialization dataWithJSONObject:@{
    @"address":handle, @"name":advertisementData[CBAdvertisementDataLocalNameKey] ?: @"NU54", @"rssi":RSSI
  } options:0 error:nil];
  if (data) [self emitOnScan:[[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding]];
}

- (void)finishScan {
  [_central stopScan];
  RCTPromiseResolveBlock resolve = _scanResolve;
  _scanResolve = nil;
  _scanReject = nil;
  if (resolve) resolve(nil);
}

- (void)stopScan:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(), ^{ [self finishScan]; resolve(nil); });
}

- (void)bond:(NSString *)address passkey:(NSString *)passkey resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  reject(@"LEGACY_UNAVAILABLE", @"Bluetooth pairing is managed by iOS.", nil);
}
- (void)isBonded:(NSString *)address resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject { resolve(@NO); }
- (void)removeBond:(NSString *)address resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject { resolve(@NO); }
- (void)connect:(NSString *)address service:(NSString *)service rx:(NSString *)rx tx:(NSString *)tx
        resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  reject(@"LEGACY_UNAVAILABLE", @"Use the supported Android installation for existing rental pairing.", nil);
}

- (void)connectOwner:(NSString *)address service:(NSString *)service rx:(NSString *)rx tx:(NSString *)tx
             resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(), ^{
    [self closePeer];
#if defined(NU54_OWNER_REGISTRATION_QA)
    if (NU54OwnerRegistrationQA()) {
      [self connectRegistrationHost:address resolve:resolve reject:reject];
      return;
    }
#endif
    CBPeripheral *target = self->_nearby[address];
    if (!target || self->_central.state!=CBManagerStatePoweredOn) {
      reject(@"BOARD_UNAVAILABLE", @"Scan for the board again before connecting.", nil); return;
    }
    self->_peer = target;
    self->_peer.delegate = self;
    self->_service = [CBUUID UUIDWithString:service];
    self->_notifyID = [CBUUID UUIDWithString:tx];
    self->_writeID = [CBUUID UUIDWithString:rx];
    self->_connectResolve = resolve;
    self->_connectReject = reject;
    [self->_central connectPeripheral:target options:nil];
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 10*NSEC_PER_SEC), dispatch_get_main_queue(), ^{
      if (self->_peer == target && self->_connectResolve) [self closePeer];
    });
  });
}
#if defined(NU54_OWNER_REGISTRATION_QA)
- (void)readRegistrationHost:(nw_connection_t)connection handle:(NSString *)handle {
  __weak RenterBle *weakSelf = self;
  nw_connection_receive(connection, 1, 8192, ^(dispatch_data_t content, nw_content_context_t context, bool done, nw_error_t error) {
    RenterBle *adapter = weakSelf;
    if (!adapter || adapter->_ownerTestConnection != connection) return;
    if (content && dispatch_data_get_size(content)) {
      const void *mapping = nullptr;
      size_t length = 0;
      dispatch_data_t mapped = dispatch_data_create_map(content, &mapping, &length);
      NSData *packet = [NSData dataWithBytes:mapping length:length];
      (void)mapped;
      [adapter emitOnFragment:[packet base64EncodedStringWithOptions:0]];
    }
    if (done || error) {
      [adapter closePeer];
      [adapter emitOnDisconnect:handle];
      return;
    }
    [adapter readRegistrationHost:connection handle:handle];
  });
}

- (void)connectRegistrationHost:(NSString *)handle resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  BOOL expected = [handle isEqualToString:registrationHandle];
  BOOL sameName = [handle isEqualToString:otherRegistrationHandle];
  NSString *relativePath = expected ? @"Documents/w04-host-port" : @"Documents/w04-other-host-port";
  NSString *portFile = [NSHomeDirectory() stringByAppendingPathComponent:relativePath];
  NSString *port = [NSString stringWithContentsOfFile:portFile encoding:NSUTF8StringEncoding error:nil];
  int value = port.intValue;
  if ((!expected && !sameName) || value < 1 || value > 65535 ||
      ![port isEqualToString:[NSString stringWithFormat:@"%d", value]]) {
    reject(@"W04_HOST", @"The simulator registration host is unavailable.", nil);
    return;
  }
  nw_parameters_t tcp = nw_parameters_create_secure_tcp(NW_PARAMETERS_DISABLE_PROTOCOL, NW_PARAMETERS_DEFAULT_CONFIGURATION);
  nw_connection_t connection = nw_connection_create(nw_endpoint_create_host("127.0.0.1", port.UTF8String), tcp);
  _ownerTestConnection = connection;
  _connectReject = reject;
  nw_connection_set_queue(connection, dispatch_get_main_queue());
  __weak RenterBle *weakSelf = self;
  nw_connection_set_state_changed_handler(connection, ^(nw_connection_state_t state, nw_error_t error) {
    RenterBle *adapter = weakSelf;
    if (!adapter || adapter->_ownerTestConnection != connection) return;
    if (state == nw_connection_state_ready) {
      adapter->_connectReject = nil;
      resolve(@247);
      [adapter readRegistrationHost:connection handle:handle];
    } else if (state == nw_connection_state_failed) {
      [adapter closePeer];
      [adapter emitOnDisconnect:handle];
    }
  });
  nw_connection_start(connection);
  dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC), dispatch_get_main_queue(), ^{
    RenterBle *adapter = weakSelf;
    if (adapter && adapter->_ownerTestConnection == connection && adapter->_connectReject) [adapter closePeer];
  });
}
#endif

- (void)centralManager:(CBCentralManager *)central didConnectPeripheral:(CBPeripheral *)peripheral {
  if (_peer==peripheral) [peripheral discoverServices:@[_service]];
}
- (void)centralManager:(CBCentralManager *)central didFailToConnectPeripheral:(CBPeripheral *)p error:(NSError *)error {
  if (_peer==p) [self closePeer];
}
- (void)centralManager:(CBCentralManager *)central didDisconnectPeripheral:(CBPeripheral *)p error:(NSError *)error {
  if (_peer==p) { [self closePeer]; [self emitOnDisconnect:p.identifier.UUIDString]; }
}
- (void)peripheral:(CBPeripheral *)p didDiscoverServices:(NSError *)error {
  if (_peer!=p) return;
  if (!error) for (CBService *service in p.services) {
    if ([service.UUID isEqual:_service]) {
      [p discoverCharacteristics:@[_writeID,_notifyID] forService:service]; return;
    }
  }
  [self closePeer];
}
- (void)peripheral:(CBPeripheral *)p didDiscoverCharacteristicsForService:(CBService *)service error:(NSError *)error {
  if (_peer!=p) return;
  CBCharacteristic *notification = nil;
  if (!error) for (CBCharacteristic *c in service.characteristics) {
    if ([c.UUID isEqual:_writeID] && (c.properties & CBCharacteristicPropertyWrite)) _input = c;
    if ([c.UUID isEqual:_notifyID] && (c.properties & CBCharacteristicPropertyNotify)) notification = c;
  }
  if (_input && notification) [p setNotifyValue:YES forCharacteristic:notification];
  else [self closePeer];
}
- (void)peripheral:(CBPeripheral *)p didUpdateNotificationStateForCharacteristic:(CBCharacteristic *)c error:(NSError *)error {
  if (_peer!=p || ![c.UUID isEqual:_notifyID]) return;
  if (error || !c.isNotifying) { [self closePeer]; return; }
  RCTPromiseResolveBlock resolve = _connectResolve;
  _connectResolve = nil; _connectReject = nil;
  if (resolve) resolve(@([p maximumWriteValueLengthForType:CBCharacteristicWriteWithResponse]+3));
}
- (void)peripheral:(CBPeripheral *)p didUpdateValueForCharacteristic:(CBCharacteristic *)c error:(NSError *)error {
  if (_peer==p && [c.UUID isEqual:_notifyID] && !error && c.value.length)
    [self emitOnFragment:[c.value base64EncodedStringWithOptions:0]];
}
- (void)writeFragment:(NSString *)base64 resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(), ^{
    NSData *data = [[NSData alloc] initWithBase64EncodedString:base64 options:0];
#if defined(NU54_OWNER_REGISTRATION_QA)
    if (self->_ownerTestConnection && data.length && data.length <= 244) {
      dispatch_data_t packet = dispatch_data_create(data.bytes, data.length, dispatch_get_main_queue(), ^{ (void)data; });
      nw_connection_send(self->_ownerTestConnection, packet, NW_CONNECTION_DEFAULT_MESSAGE_CONTEXT, true, ^(nw_error_t error) {
        if (error) reject(@"W04_HOST", @"The simulator registration host disconnected.", nil);
        else resolve(nil);
      });
      return;
    }
#endif
    if (!self->_peer || !self->_input || self->_writeResolve || !data.length ||
        data.length>[self->_peer maximumWriteValueLengthForType:CBCharacteristicWriteWithResponse]) {
      reject(@"GATT_WRITE", @"The Bluetooth write cannot be sent.", nil); return;
    }
    self->_writeResolve = resolve; self->_writeReject = reject;
    [self->_peer writeValue:data forCharacteristic:self->_input type:CBCharacteristicWriteWithResponse];
  });
}
- (void)peripheral:(CBPeripheral *)p didWriteValueForCharacteristic:(CBCharacteristic *)c error:(NSError *)error {
  if (_peer!=p || _input!=c) return;
  RCTPromiseResolveBlock resolve = _writeResolve; RCTPromiseRejectBlock reject = _writeReject;
  _writeResolve = nil; _writeReject = nil;
  if (error && reject) reject(@"GATT_WRITE", @"Bluetooth write failed.", nil);
  else if (resolve) resolve(nil);
}

- (void)closePeer {
#if defined(NU54_OWNER_REGISTRATION_QA)
  if (_ownerTestConnection) {
    nw_connection_t connection = _ownerTestConnection;
    _ownerTestConnection = nil;
    nw_connection_cancel(connection);
  }
#endif
  CBPeripheral *old = _peer; _peer=nil; _input=nil;
  if (old) [_central cancelPeripheralConnection:old];
  RCTPromiseRejectBlock reject = _connectReject, writeReject = _writeReject;
  _connectResolve=nil; _connectReject=nil; _writeResolve=nil; _writeReject=nil;
  if (reject) reject(@"GATT_CLOSED", @"Board connection closed.", nil);
  if (writeReject) writeReject(@"GATT_CLOSED", @"Board connection closed.", nil);
}
- (void)disconnect:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(), ^{ [self closePeer]; resolve(nil); });
}
- (void)randomBytes:(double)size resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  if (size<1 || size>64 || size!=(NSUInteger)size) { reject(@"RANDOM", @"Invalid length", nil); return; }
  NSMutableData *bytes = [NSMutableData dataWithLength:(NSUInteger)size];
  if (SecRandomCopyBytes(kSecRandomDefault, bytes.length, bytes.mutableBytes)==errSecSuccess)
    resolve([bytes base64EncodedStringWithOptions:0]);
  else reject(@"RANDOM", @"Device randomness unavailable.", nil);
}
- (void)loadDevices:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject { resolve(@""); }
- (void)saveDevices:(NSString *)json resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  reject(@"LEGACY_UNAVAILABLE", @"Legacy enrollment is not available on this platform.", nil);
}
@end
