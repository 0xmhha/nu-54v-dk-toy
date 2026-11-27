#import <CoreBluetooth/CoreBluetooth.h>
#import <CommonCrypto/CommonDigest.h>
#import <Network/Network.h>
#import <Security/Security.h>
#if defined(NU54_MOBILE_LAB)
static const BOOL terminalProduct=NO;
#elif __has_include(<NU54RenterSpec/NU54RenterSpec.h>)
#import <NU54RenterSpec/NU54RenterSpec.h>
static const BOOL terminalProduct=NO;
#else
#import <NU54KioskSpec/NU54KioskSpec.h>
static const BOOL terminalProduct=YES;
#endif

static const char nearbyALPN[]="nu54-v2-nearby";
static const char paymentALPN[]="nu54-v2-payment";
static const char exporter[]="EXPORTER-NU54-wire2";
static const NSUInteger limit=49152;
static CBUUID *serviceID(BOOL payment) { return [CBUUID UUIDWithString:payment ? @"41ba05a0-e0dd-4fd7-901c-4c25d5010000" : @"41ba05a0-e0dd-4fd7-901c-4c25d5020000"]; }
static CBUUID *rxID(BOOL payment) { return [CBUUID UUIDWithString:payment ? @"41ba05a0-e0dd-4fd7-901c-4c25d5010001" : @"41ba05a0-e0dd-4fd7-901c-4c25d5020001"]; }
static CBUUID *txID(BOOL payment) { return [CBUUID UUIDWithString:payment ? @"41ba05a0-e0dd-4fd7-901c-4c25d5010002" : @"41ba05a0-e0dd-4fd7-901c-4c25d5020002"]; }
static NSTimeInterval now(void) { return NSDate.date.timeIntervalSince1970; }

static NSData *spki(SecKeyRef key) {
  if (!key) return nil;
  NSDictionary *attrs=CFBridgingRelease(SecKeyCopyAttributes(key));
  if (![attrs[(__bridge id)kSecAttrKeyType] isEqual:(__bridge id)kSecAttrKeyTypeECSECPrimeRandom] ||
      [attrs[(__bridge id)kSecAttrKeySizeInBits] intValue]!=256) return nil;
  NSData *point=CFBridgingRelease(SecKeyCopyExternalRepresentation(key,nullptr));
  if (point.length!=65) return nil;
  const uint8_t prefix[]={0x30,0x59,0x30,0x13,0x06,0x07,0x2a,0x86,0x48,0xce,0x3d,0x02,0x01,
      0x06,0x08,0x2a,0x86,0x48,0xce,0x3d,0x03,0x01,0x07,0x03,0x42,0x00};
  NSMutableData *encoded=[NSMutableData dataWithBytes:prefix length:sizeof(prefix)];
  [encoded appendData:point];
  return encoded;
}

static NSString *fingerprint(NSData *bytes) {
  if (!bytes) return nil;
  uint8_t digest[CC_SHA256_DIGEST_LENGTH];
  CC_SHA256(bytes.bytes,(CC_LONG)bytes.length,digest);
  NSMutableString *out=[NSMutableString string];
  for (uint8_t byte : digest) [out appendFormat:@"%02x",byte];
  return out;
}

static CFTypeRef nearbyKey(BOOL create,BOOL payment) CF_RETURNS_RETAINED {
  NSData *tag=[(payment ? @"nu54.payment.terminal.transport.v2" : @"nu54.nearby.transport.v2") dataUsingEncoding:NSUTF8StringEncoding];
  NSDictionary *query=@{(__bridge id)kSecClass:(__bridge id)kSecClassKey,
    (__bridge id)kSecAttrApplicationTag:tag,(__bridge id)kSecReturnRef:@YES};
  CFTypeRef key=nullptr;
  OSStatus rc=SecItemCopyMatching((__bridge CFDictionaryRef)query,&key);
  if (rc==errSecSuccess) return key;
  if (!create || rc!=errSecItemNotFound) return nullptr;
  id access=CFBridgingRelease(SecAccessControlCreateWithFlags(nullptr,
      kSecAttrAccessibleWhenUnlockedThisDeviceOnly,kSecAccessControlPrivateKeyUsage,nullptr));
  if (!access) return nullptr;
  NSDictionary *args=@{(__bridge id)kSecAttrKeyType:(__bridge id)kSecAttrKeyTypeECSECPrimeRandom,
      (__bridge id)kSecAttrKeySizeInBits:@256,(__bridge id)kSecAttrTokenID:(__bridge id)kSecAttrTokenIDSecureEnclave,
      (__bridge id)kSecPrivateKeyAttrs:@{(__bridge id)kSecAttrIsPermanent:@YES,
        (__bridge id)kSecAttrApplicationTag:tag,(__bridge id)kSecAttrAccessControl:access}};
  return SecKeyCreateRandomKey((__bridge CFDictionaryRef)args,nullptr);
}

static NSData *decode(id raw, NSUInteger max) {
  if (![raw isKindOfClass:NSString.class] || [raw length]>max*2) return nil;
  NSData *out=[[NSData alloc] initWithBase64EncodedString:raw options:0];
  return out.length<=max ? out : nil;
}

static id cert(id raw) {
  NSData *der=decode(raw,12000);
  return der ? CFBridgingRelease(SecCertificateCreateWithData(nullptr,(__bridge CFDataRef)der)) : nil;
}

struct DER { const uint8_t *data; size_t size; };
static bool take(DER &input, uint8_t &tag, DER &value) {
  if (input.size<2) return false;
  tag=input.data[0];
  size_t length=input.data[1], head=2;
  if (length&0x80) {
    size_t count=length&0x7f;
    if (!count || count>3 || input.size<2+count || !input.data[2]) return false;
    head+=count;
    length=0;
    for (size_t i=2;i<head;i++) length=(length<<8)|input.data[i];
    if (length<128) return false;
  }
  if (length>input.size-head) return false;
  value={input.data+head,length};
  input={input.data+head+length,input.size-head-length};
  return true;
}
static bool role(SecCertificateRef certificate, const char *expected) {
  NSData *bytes=CFBridgingRelease(SecCertificateCopyData(certificate));
  DER input={(const uint8_t *)bytes.bytes,bytes.length}, outer,tbs,field,extensions;
  uint8_t tag;
  if (!take(input,tag,outer) || tag!=0x30 || input.size || !take(outer,tag,tbs) || tag!=0x30) return false;
  if (tbs.size && tbs.data[0]==0xa0 && !take(tbs,tag,field)) return false;
  const uint8_t required[]={0x02,0x30,0x30,0x30,0x30,0x30};
  for (uint8_t type:required) if (!take(tbs,tag,field) || tag!=type) return false;
  while (take(tbs,tag,field)) {
    if (tag!=0xa3) continue;
    if (tbs.size || !take(field,tag,extensions) || tag!=0x30 || field.size) return false;
    bool found=false;
    while (take(extensions,tag,field)) {
      DER oid,value;
      if (tag!=0x30 || !take(field,tag,oid) || tag!=0x06) return false;
      const uint8_t san[]={0x55,0x1d,0x11};
      if (oid.size!=sizeof(san) || memcmp(oid.data,san,sizeof(san))) continue;
      if (found || !take(field,tag,value)) return false;
      if (tag==0x01 && !take(field,tag,value)) return false;
      DER names,name;
      if (tag!=0x04 || field.size || !take(value,tag,names) || tag!=0x30 || value.size ||
          !take(names,tag,name) || tag!=0x86 || names.size || name.size!=strlen(expected) ||
          memcmp(name.data,expected,name.size)) return false;
      found=true;
    }
    return found && !extensions.size;
  }
  return false;
}

static BOOL uuid(id value) {
  return [value isKindOfClass:NSString.class] && [value length]==36 &&
      [[[NSUUID alloc] initWithUUIDString:value].UUIDString lowercaseString] &&
      [[[[NSUUID alloc] initWithUUIDString:value].UUIDString lowercaseString] isEqual:value] &&
      ![value isEqual:@"00000000-0000-0000-0000-000000000000"];
}

@interface NearbyEnrollment : NSObject
@property id ca;
@property sec_identity_t identity;
@property NSDictionary<NSString *,NSDictionary *> *peers;
@property NSTimeInterval expiry;
@end
@implementation NearbyEnrollment
@end

@interface NearbyPipe : NSObject
@property NSString *identifier;
@property BOOL payment;
@property BOOL server;
@property BOOL sentFirst;
@property CBPeripheral *peripheral;
@property CBCentral *central;
@property CBCharacteristic *rx;
@property (copy) void (^encrypted)(NSData *);
@property (copy) void (^clear)(NSData *);
@property (copy) void (^ready)(NSData *,NSDictionary *);
@property (copy) void (^failed)(void);
@property NSMutableArray<NSData *> *fragments;
@property NSUInteger bytes;
@property BOOL busy;
- (void)open:(NearbyEnrollment *)profile;
- (BOOL)feed:(NSData *)data;
- (BOOL)write:(NSData *)data;
- (void)stop;
@end

@implementation NearbyPipe {
  nw_listener_t _listener;
  nw_connection_t _wire;
  nw_connection_t _secure;
  NSMutableData *_early;
  NSUInteger _wireQueued,_secureQueued;
  NearbyEnrollment *_profile;
  NSDictionary *_peer;
  BOOL _bound,_wireReady,_stopped;
}

- (BOOL)isServer { return _server; }
- (const char *)protocolName { return _payment ? paymentALPN : nearbyALPN; }

- (BOOL)send:(NSData *)data on:(nw_connection_t)conn isWire:(BOOL)wire {
  NSUInteger *queued=wire ? &_wireQueued : &_secureQueued;
  if (!conn || _stopped || *queued+data.length>limit) return NO;
  *queued+=data.length;
  dispatch_data_t packet=dispatch_data_create(data.bytes,data.length,dispatch_get_main_queue(),^{ (void)data; });
  __weak NearbyPipe *weak=self;
  nw_connection_send(conn,packet,NW_CONNECTION_DEFAULT_MESSAGE_CONTEXT,true,^(nw_error_t error){
    NearbyPipe *pipe=weak;
    if (!pipe || pipe->_stopped) return;
    if (wire) pipe->_wireQueued-=data.length; else pipe->_secureQueued-=data.length;
    if (error && pipe.failed) pipe.failed();
  });
  return YES;
}

- (void)receive:(nw_connection_t)conn isWire:(BOOL)wire {
  __weak NearbyPipe *weak=self;
  nw_connection_receive(conn,1,8192,^(dispatch_data_t content,nw_content_context_t ctx,bool done,nw_error_t err){
    NearbyPipe *pipe=weak;
    if (!pipe || pipe->_stopped) return;
    if (done || err || (!wire && (!pipe->_bound || now() >= [pipe->_peer[@"expiresAt"] doubleValue]))) {
      if (pipe.failed) pipe.failed(); return;
    }
    if (content && dispatch_data_get_size(content)) {
      const void *mapping=nullptr; size_t length=0;
      dispatch_data_t mapped=dispatch_data_create_map(content,&mapping,&length);
      NSData *packet=[NSData dataWithBytes:mapping length:length]; (void)mapped;
      if (wire) { if (pipe.encrypted) pipe.encrypted(packet); }
      else { if (pipe.clear) pipe.clear(packet); }
    }
    [pipe receive:conn isWire:wire];
  });
}

- (void)bind:(nw_connection_t)conn wire:(BOOL)wire {
  if (wire) _wire=conn; else _secure=conn;
  __weak NearbyPipe *weak=self;
  nw_connection_set_queue(conn,dispatch_get_main_queue());
  nw_connection_set_state_changed_handler(conn,^(nw_connection_state_t state,nw_error_t err){
    NearbyPipe *pipe=weak;
    if (!pipe || pipe->_stopped) return;
    if (state==nw_connection_state_failed || state==nw_connection_state_cancelled) {
      if (pipe.failed) pipe.failed(); return;
    }
    if (state!=nw_connection_state_ready) return;
    if (wire) {
      pipe->_wireReady=YES;
      if (pipe->_early.length && ![pipe send:[pipe->_early copy] on:conn isWire:YES]) {
        if (pipe.failed) pipe.failed(); return;
      }
      [pipe->_early setLength:0];
    } else {
      nw_protocol_metadata_t tls=nw_connection_copy_protocol_metadata(conn,nw_protocol_copy_tls_definition());
      sec_protocol_metadata_t metadata=tls ? nw_tls_copy_sec_protocol_metadata(tls) : nil;
      const char *protocol=metadata ? sec_protocol_metadata_get_negotiated_protocol(metadata) : nullptr;
      const char *expected=[pipe protocolName];
      dispatch_data_t secret=protocol && !strcmp(protocol,expected) && pipe->_peer &&
          sec_protocol_metadata_get_negotiated_tls_protocol_version(metadata)==tls_protocol_version_TLSv13 ?
          sec_protocol_metadata_create_secret_with_context(metadata,strlen(exporter),exporter,strlen(expected),
              (const uint8_t *)expected,16) : nil;
      if (!secret || dispatch_data_get_size(secret)!=16 || now() >= [pipe->_peer[@"expiresAt"] doubleValue]) {
        if (pipe.failed) pipe.failed(); return;
      }
      const void *mapping=nullptr; size_t length=0;
      dispatch_data_t mapped=dispatch_data_create_map(secret,&mapping,&length);
      pipe->_bound=YES;
      if (pipe.ready) pipe.ready([NSData dataWithBytes:mapping length:length],pipe->_peer);
      (void)mapped;
    }
    [pipe receive:conn isWire:wire];
  });
  nw_connection_start(conn);
}

- (nw_parameters_t)encryptedParameters {
  __weak NearbyPipe *weak=self;
  return nw_parameters_create_secure_tcp(^(nw_protocol_options_t options) {
    NearbyPipe *pipe=weak;
    if (!pipe) return;
    sec_protocol_options_t security=nw_tls_copy_sec_protocol_options(options);
    sec_protocol_options_set_min_tls_protocol_version(security,tls_protocol_version_TLSv13);
    sec_protocol_options_set_max_tls_protocol_version(security,tls_protocol_version_TLSv13);
    sec_protocol_options_set_tls_resumption_enabled(security,false);
    sec_protocol_options_set_tls_tickets_enabled(security,false);
    sec_protocol_options_set_peer_authentication_required(security,true);
    sec_protocol_options_add_tls_application_protocol(security,[pipe protocolName]);
    sec_protocol_options_set_local_identity(security,pipe->_profile.identity);
    sec_protocol_options_set_verify_block(security,^(sec_protocol_metadata_t metadata,sec_trust_t trust,sec_protocol_verify_complete_t complete) {
      NearbyPipe *active=weak;
      if (!active || active->_stopped) { complete(false); return; }
      id object=CFBridgingRelease(sec_trust_copy_ref(trust));
      SecTrustRef check=(__bridge SecTrustRef)object;
      id policy=CFBridgingRelease(SecPolicyCreateSSL(![active isServer],nullptr));
      BOOL accepted=check && SecTrustSetPolicies(check,(__bridge CFTypeRef)policy)==errSecSuccess &&
          SecTrustSetAnchorCertificates(check,(__bridge CFArrayRef)@[active->_profile.ca])==errSecSuccess &&
          SecTrustSetAnchorCertificatesOnly(check,true)==errSecSuccess && SecTrustEvaluateWithError(check,nullptr);
      NSArray *chain=accepted ? CFBridgingRelease(SecTrustCopyCertificateChain(check)) : nil;
      SecCertificateRef leaf=chain.count ? (__bridge SecCertificateRef)chain[0] : nullptr;
      id key=leaf && role(leaf,active.payment?"urn:nu54:role:board":[active isServer]?"urn:nu54:role:customer":"urn:nu54:role:terminal") ?
          CFBridgingRelease(SecCertificateCopyKey(leaf)) : nil;
      NSString *pin=key ? fingerprint(spki((__bridge SecKeyRef)key)) : nil;
      NSDictionary *enrolled=pin ? active->_profile.peers[pin] : nil;
      if (enrolled && now()<[enrolled[@"expiresAt"] doubleValue]) active->_peer=enrolled;
      complete(active->_peer!=nil);
    },dispatch_get_main_queue());
  },NW_PARAMETERS_DEFAULT_CONFIGURATION);
}

- (void)open:(NearbyEnrollment *)profile {
  _profile=profile; _early=[NSMutableData new]; _fragments=[NSMutableArray new];
  nw_parameters_t local=[self isServer] ? [self encryptedParameters] :
      nw_parameters_create_secure_tcp(NW_PARAMETERS_DISABLE_PROTOCOL,NW_PARAMETERS_DEFAULT_CONFIGURATION);
  nw_parameters_set_local_endpoint(local,nw_endpoint_create_host("127.0.0.1","0"));
  _listener=nw_listener_create(local);
  if (!_listener) { if (_failed) _failed(); return; }
  nw_listener_set_queue(_listener,dispatch_get_main_queue());
  __weak NearbyPipe *weak=self;
  nw_listener_set_new_connection_handler(_listener,^(nw_connection_t incoming){
    NearbyPipe *pipe=weak;
    if (!pipe || pipe->_stopped || ([pipe isServer]?pipe->_secure:pipe->_wire)) nw_connection_cancel(incoming);
    else [pipe bind:incoming wire:![pipe isServer]];
  });
  nw_listener_set_state_changed_handler(_listener,^(nw_listener_state_t state,nw_error_t failure){
    NearbyPipe *pipe=weak;
    if (!pipe || pipe->_stopped) return;
    if (state==nw_listener_state_failed) { if (pipe.failed) pipe.failed(); return; }
    if (state!=nw_listener_state_ready || ([pipe isServer]?pipe->_wire:pipe->_secure)) return;
    NSString *port=[NSString stringWithFormat:@"%u",nw_listener_get_port(pipe->_listener)];
    nw_parameters_t parameters=[pipe isServer] ? nw_parameters_create_secure_tcp(NW_PARAMETERS_DISABLE_PROTOCOL,NW_PARAMETERS_DEFAULT_CONFIGURATION) :
        [pipe encryptedParameters];
    nw_connection_t outgoing=nw_connection_create(nw_endpoint_create_host("127.0.0.1",port.UTF8String),parameters);
    if (outgoing) [pipe bind:outgoing wire:[pipe isServer]];
    else if (pipe.failed) pipe.failed();
  });
  nw_listener_start(_listener);
  dispatch_after(dispatch_time(DISPATCH_TIME_NOW,12*NSEC_PER_SEC),dispatch_get_main_queue(),^{
    NearbyPipe *pipe=weak;
    if (pipe && !pipe->_bound && pipe.failed) pipe.failed();
  });
}

- (BOOL)feed:(NSData *)data {
  if (!data.length || data.length>244 || _stopped) return NO;
  if (_wireReady) return [self send:data on:_wire isWire:YES];
  if (_early.length+data.length>limit) return NO;
  [_early appendData:data];
  return YES;
}
- (BOOL)write:(NSData *)data {
  return _bound && data.length && data.length<=3072 && now()<[_peer[@"expiresAt"] doubleValue] &&
      [self send:data on:_secure isWire:NO];
}
- (void)stop {
  _stopped=YES;
  if (_wire) nw_connection_cancel(_wire);
  if (_secure) nw_connection_cancel(_secure);
  if (_listener) nw_listener_cancel(_listener);
  _wire=nil; _secure=nil; _listener=nil;
  [_fragments removeAllObjects];
}
@end

#if !defined(NU54_MOBILE_LAB)
@interface Nearby : NativeNearbySpecBase <NativeNearbySpec,CBCentralManagerDelegate,CBPeripheralDelegate,CBPeripheralManagerDelegate>
@end
@implementation Nearby {
  BOOL _payment;
  Nearby *_paymentBridge;
  void (^_forward)(NSString *);
  NearbyEnrollment *_profile;
  CBCentralManager *_scanner;
  CBPeripheralManager *_advertiser;
  NSMutableDictionary<NSString *,CBPeripheral *> *_candidates;
  NSMutableDictionary<NSString *,NearbyPipe *> *_links;
  CBMutableCharacteristic *_notify;
  NearbyPipe *_client;
  RCTPromiseResolveBlock _scanResolve;
  RCTPromiseRejectBlock _scanReject;
  NSUInteger _scanToken;
  NSTimeInterval _scanExpires;
  RCTPromiseResolveBlock _startResolve;
  RCTPromiseRejectBlock _startReject;
}
RCT_EXPORT_MODULE(Nearby)
- (std::shared_ptr<facebook::react::TurboModule>)getTurboModule:
    (const facebook::react::ObjCTurboModule::InitParams &)params {
  return std::make_shared<facebook::react::NativeNearbySpecJSI>(params);
}
- (BOOL)isServer { return terminalProduct && !_payment; }
- (Nearby *)paymentBridge {
  if (!_paymentBridge && terminalProduct && !_payment) {
    _paymentBridge=[Nearby new];
    _paymentBridge->_payment=YES;
    __weak Nearby *weak=self;
    _paymentBridge->_forward=^(NSString *message){ [weak emitOnPayment:message]; };
  }
  return _paymentBridge;
}
- (void)error:(RCTPromiseRejectBlock)reject { if (reject) reject(@"NEARBY_UNAVAILABLE",@"Nearby operation unavailable.",nil); }
- (void)publicIdentity:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{
    id key=CFBridgingRelease(nearbyKey(YES,self->_payment));
    id publicKey=key ? CFBridgingRelease(SecKeyCopyPublicKey((__bridge SecKeyRef)key)) : nil;
    NSData *value=spki((__bridge SecKeyRef)publicKey);
    if (value) resolve([value base64EncodedStringWithOptions:0]); else [self error:reject];
  });
}
- (void)configure:(NSString *)raw resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{
    if (!raw.length || raw.length>limit) { [self error:reject]; return; }
    id json=[NSJSONSerialization JSONObjectWithData:[raw dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nullptr];
    if (![json isKindOfClass:NSDictionary.class]) { [self error:reject]; return; }
    id branch=json[@"branch"], expiry=json[@"expiresAt"], peers=json[@"peers"], chain=json[@"certificates"];
    id ca=cert(json[@"ca"]);
    if (!uuid(branch) || ![expiry isKindOfClass:NSNumber.class] || [expiry doubleValue]<=now() || !ca ||
        ![peers isKindOfClass:NSArray.class] || ![peers count] || [peers count]>64 ||
        ![chain isKindOfClass:NSArray.class] || ![chain count] || [chain count]>4) { [self error:reject]; return; }
    NSMutableDictionary *pins=[NSMutableDictionary new];
    NSCharacterSet *invalid=[[NSCharacterSet characterSetWithCharactersInString:@"0123456789abcdef"] invertedSet];
    for (id peer in peers) {
      if (![peer isKindOfClass:NSDictionary.class]) { [self error:reject]; return; }
      id pin=peer[@"pin"], principal=peer[@"principal"], end=peer[@"expiresAt"] ?: expiry;
      if (![pin isKindOfClass:NSString.class] || [pin length]!=64 || [pin rangeOfCharacterFromSet:invalid].location!=NSNotFound ||
          pins[pin] || !uuid(principal) || ![peer[@"branch"] isEqual:branch] || ![end isKindOfClass:NSNumber.class]) {
        [self error:reject]; return;
      }
      pins[pin]=@{@"peer":pin,@"principal":principal,@"branch":branch,@"expiresAt":@(MIN([expiry doubleValue],[end doubleValue]))};
    }
    NSMutableArray *certs=[NSMutableArray new];
    for (id entry in chain) {
      id certificate=cert(entry);
      if (!certificate) { [self error:reject]; return; }
      [certs addObject:certificate];
    }
    id key=CFBridgingRelease(nearbyKey(NO,self->_payment));
    if (!key || !role((__bridge SecCertificateRef)certs[0],terminalProduct?"urn:nu54:role:terminal":"urn:nu54:role:customer")) {
      [self error:reject]; return;
    }
    id identity=CFBridgingRelease(SecIdentityCreate(nullptr,(__bridge SecCertificateRef)certs[0],(__bridge SecKeyRef)key));
    if (!identity) { [self error:reject]; return; }
    NSArray *additional=[certs subarrayWithRange:NSMakeRange(1,certs.count-1)];
    sec_identity_t signer=sec_identity_create_with_certificates((__bridge SecIdentityRef)identity,(__bridge CFArrayRef)additional);
    if (!signer) { [self error:reject]; return; }
    [self teardown];
    NearbyEnrollment *profile=[NearbyEnrollment new];
    profile.ca=ca; profile.identity=signer; profile.peers=[pins copy]; profile.expiry=[expiry doubleValue];
    self->_profile=profile;
    resolve(nil);
  });
}
- (void)emit:(NSString *)kind pipe:(NearbyPipe *)pipe extra:(NSDictionary *)extra {
  NSMutableDictionary *record=extra ? [extra mutableCopy] : [NSMutableDictionary new];
  record[@"kind"]=kind; record[@"link"]=pipe.identifier;
  NSData *json=[NSJSONSerialization dataWithJSONObject:record options:0 error:nullptr];
  if (json) {
    NSString *text=[[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding];
    if (_forward) _forward(text); else [self emitOnNearby:text];
  }
}
- (void)retire:(NearbyPipe *)pipe {
  if (!pipe || _links[pipe.identifier]!=pipe) return;
  [_links removeObjectForKey:pipe.identifier];
  [pipe stop];
  if (pipe.peripheral) [_scanner cancelPeripheralConnection:pipe.peripheral];
  if (_client==pipe) _client=nil;
  [self emit:@"closed" pipe:pipe extra:nil];
}
- (void)teardown {
  [self finishScan:NO];
  [_candidates removeAllObjects];
  for (NearbyPipe *pipe in _links.allValues) [self retire:pipe];
  [_advertiser stopAdvertising]; [_advertiser removeAllServices];
  _advertiser.delegate=nil; _advertiser=nil; _notify=nil;
  if (_startReject) [self error:_startReject];
  _startResolve=nil; _startReject=nil;
}
- (void)scan:(double)seconds resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{
    if ([self isServer] || !self->_profile || now()>=self->_profile.expiry || !(seconds>=1 && seconds<=10)) {
      [self error:reject]; return;
    }
    [self finishScan:NO];
    self->_candidates=[NSMutableDictionary new];
    self->_scanExpires=0;
    self->_scanResolve=resolve; self->_scanReject=reject;
    if (!self->_scanner) self->_scanner=[[CBCentralManager alloc] initWithDelegate:self queue:nil];
    [self startScanIfReady];
    NSUInteger token=++self->_scanToken;
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW,(int64_t)(seconds*NSEC_PER_SEC)),dispatch_get_main_queue(),^{
      if (token!=self->_scanToken || !self->_scanResolve) return;
      [self finishScan:self->_scanner.state==CBManagerStatePoweredOn];
      dispatch_after(dispatch_time(DISPATCH_TIME_NOW,20*NSEC_PER_SEC),dispatch_get_main_queue(),^{
        if (token==self->_scanToken) [self->_candidates removeAllObjects];
      });
    });
  });
}
- (void)finishScan:(BOOL)success {
  [_scanner stopScan];
  RCTPromiseResolveBlock resolve=_scanResolve; RCTPromiseRejectBlock reject=_scanReject;
  _scanResolve=nil; _scanReject=nil;
  if (!resolve) return;
  if (!success) { [_candidates removeAllObjects]; [self error:reject]; return; }
  _scanExpires=NSProcessInfo.processInfo.systemUptime+20;
  NSMutableArray *output=[NSMutableArray new];
  for (NSString *handle in _candidates) [output addObject:@{@"handle":handle}];
  NSData *data=[NSJSONSerialization dataWithJSONObject:output options:0 error:nullptr];
  resolve([[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding]);
}
- (void)startScanIfReady {
  if (!_scanResolve) return;
  if (_scanner.state==CBManagerStatePoweredOn) [_scanner scanForPeripheralsWithServices:@[serviceID(_payment)] options:nil];
  else if (_scanner.state!=CBManagerStateUnknown && _scanner.state!=CBManagerStateResetting) [self finishScan:NO];
}
- (void)centralManagerDidUpdateState:(CBCentralManager *)central {
  [self startScanIfReady];
  if (central.state!=CBManagerStatePoweredOn) [self retire:_client];
}
- (void)centralManager:(CBCentralManager *)central didDiscoverPeripheral:(CBPeripheral *)peripheral
     advertisementData:(NSDictionary *)data RSSI:(NSNumber *)rssi {
  if (_scanResolve && _candidates.count<16 && ![_candidates.allValues containsObject:peripheral])
    _candidates[NSUUID.UUID.UUIDString.lowercaseString]=peripheral;
}
- (void)connect:(NSString *)handle resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{
    CBPeripheral *peer=self->_candidates[handle];
    if ([self isServer] || !peer || self->_client || self->_scanResolve ||
        NSProcessInfo.processInfo.systemUptime>=self->_scanExpires ||
        self->_scanner.state!=CBManagerStatePoweredOn || now()>=self->_profile.expiry) { [self error:reject]; return; }
    [self->_candidates removeObjectForKey:handle];
    NearbyPipe *pipe=[self createPipe];
    pipe.peripheral=peer; self->_client=pipe;
    [self->_scanner connectPeripheral:peer options:nil];
    resolve(pipe.identifier);
  });
}
- (NearbyPipe *)createPipe {
  if (!_links) _links=[NSMutableDictionary new];
  NearbyPipe *pipe=[NearbyPipe new]; pipe.payment=_payment; pipe.server=[self isServer]; pipe.identifier=NSUUID.UUID.UUIDString.lowercaseString;
  _links[pipe.identifier]=pipe;
  __weak Nearby *weak=self; __weak NearbyPipe *weakPipe=pipe;
  pipe.failed=^{ [weak retire:weakPipe]; };
  pipe.ready=^(NSData *epoch,NSDictionary *enrollment){
    NSMutableDictionary *value=[enrollment mutableCopy];
    value[@"epoch"]=[epoch base64EncodedStringWithOptions:0];
    [weak emit:@"ready" pipe:weakPipe extra:value];
  };
  pipe.clear=^(NSData *data){ [weak emit:@"data" pipe:weakPipe extra:@{@"clear":[data base64EncodedStringWithOptions:0]}]; };
  pipe.encrypted=^(NSData *data){ [weak queue:data pipe:weakPipe]; };
  dispatch_after(dispatch_time(DISPATCH_TIME_NOW,180*NSEC_PER_SEC),dispatch_get_main_queue(),^{ [weak retire:weakPipe]; });
  return pipe;
}
- (void)centralManager:(CBCentralManager *)central didConnectPeripheral:(CBPeripheral *)peripheral {
  if (_client.peripheral!=peripheral) { [central cancelPeripheralConnection:peripheral]; return; }
  peripheral.delegate=self; [peripheral discoverServices:@[serviceID(_payment)]];
}
- (void)centralManager:(CBCentralManager *)central didFailToConnectPeripheral:(CBPeripheral *)peer error:(NSError *)e {
  if (_client.peripheral==peer) [self retire:_client];
}
- (void)centralManager:(CBCentralManager *)central didDisconnectPeripheral:(CBPeripheral *)peer error:(NSError *)e {
  if (_client.peripheral==peer) [self retire:_client];
}
- (void)peripheral:(CBPeripheral *)peer didDiscoverServices:(NSError *)error {
  if (_client.peripheral!=peer) return;
  for (CBService *endpoint in peer.services) {
    if (!error && [endpoint.UUID isEqual:serviceID(_payment)]) { [peer discoverCharacteristics:@[rxID(_payment),txID(_payment)] forService:endpoint]; return; }
  }
  [self retire:_client];
}
- (void)peripheral:(CBPeripheral *)peer didDiscoverCharacteristicsForService:(CBService *)endpoint error:(NSError *)error {
  if (_client.peripheral!=peer) return;
  CBCharacteristic *tx=nil;
  for (CBCharacteristic *property in endpoint.characteristics) {
    if ([property.UUID isEqual:rxID(_payment)]) _client.rx=property;
    if ([property.UUID isEqual:txID(_payment)]) tx=property;
  }
  if (error || !_client.rx || !tx) [self retire:_client];
  else [peer setNotifyValue:YES forCharacteristic:tx];
}
- (void)peripheral:(CBPeripheral *)peer didUpdateNotificationStateForCharacteristic:(CBCharacteristic *)property error:(NSError *)error {
  if (_client.peripheral!=peer) return;
  if (error || !property.isNotifying || ![property.UUID isEqual:txID(_payment)]) [self retire:_client];
  else [_client open:_profile];
}
- (void)peripheral:(CBPeripheral *)peer didUpdateValueForCharacteristic:(CBCharacteristic *)property error:(NSError *)error {
  if (_client.peripheral!=peer) return;
  if (error || ![property.UUID isEqual:txID(_payment)] || ![_client feed:property.value]) [self retire:_client];
}
- (void)peripheral:(CBPeripheral *)peer didWriteValueForCharacteristic:(CBCharacteristic *)property error:(NSError *)error {
  NearbyPipe *pipe=_client;
  if (pipe.peripheral!=peer) return;
  if (error || !pipe.busy || property!=pipe.rx) { [self retire:pipe]; return; }
  pipe.bytes-=pipe.fragments.firstObject.length;
  [pipe.fragments removeObjectAtIndex:0]; pipe.busy=NO;
  [self pump:pipe];
}
- (void)start:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{
    if (![self isServer] || !self->_profile || now()>=self->_profile.expiry || self->_advertiser) { [self error:reject]; return; }
    self->_startResolve=resolve; self->_startReject=reject;
    self->_advertiser=[[CBPeripheralManager alloc] initWithDelegate:self queue:nil];
  });
}
- (void)peripheralManagerDidUpdateState:(CBPeripheralManager *)manager {
  if (manager!=_advertiser) return;
  if (manager.state!=CBManagerStatePoweredOn) {
    if (manager.state!=CBManagerStateUnknown && manager.state!=CBManagerStateResetting) [self teardown];
    return;
  }
  CBMutableService *endpoint=[[CBMutableService alloc] initWithType:serviceID(_payment) primary:YES];
  CBMutableCharacteristic *rx=[[CBMutableCharacteristic alloc] initWithType:rxID(_payment) properties:CBCharacteristicPropertyWrite
      value:nil permissions:CBAttributePermissionsWriteable];
  _notify=[[CBMutableCharacteristic alloc] initWithType:txID(_payment) properties:CBCharacteristicPropertyNotify value:nil permissions:0];
  endpoint.characteristics=@[rx,_notify];
  [manager addService:endpoint];
}
- (void)peripheralManager:(CBPeripheralManager *)manager didAddService:(CBService *)endpoint error:(NSError *)error {
  if (manager!=_advertiser) return;
  if (error || ![endpoint.UUID isEqual:serviceID(_payment)]) [self teardown];
  else [manager startAdvertising:@{CBAdvertisementDataServiceUUIDsKey:@[serviceID(_payment)]}];
}
- (void)peripheralManagerDidStartAdvertising:(CBPeripheralManager *)manager error:(NSError *)error {
  if (manager!=_advertiser) return;
  if (error) [self teardown];
  else if (_startResolve) { _startResolve(nil); _startResolve=nil; _startReject=nil; }
}
- (NearbyPipe *)forCentral:(CBCentral *)central {
  for (NearbyPipe *pipe in _links.allValues) if ([pipe.central.identifier isEqual:central.identifier]) return pipe;
  return nil;
}
- (void)peripheralManager:(CBPeripheralManager *)manager central:(CBCentral *)central didSubscribeToCharacteristic:(CBCharacteristic *)characteristic {
  if (manager!=_advertiser || ![characteristic.UUID isEqual:txID(_payment)] || _links.count>=2 || [self forCentral:central] ||
      now()>=_profile.expiry) return;
  NearbyPipe *pipe=[self createPipe]; pipe.central=central;
  [pipe open:_profile];
}
- (void)peripheralManager:(CBPeripheralManager *)manager central:(CBCentral *)central didUnsubscribeFromCharacteristic:(CBCharacteristic *)characteristic {
  if (manager==_advertiser) [self retire:[self forCentral:central]];
}
- (void)peripheralManager:(CBPeripheralManager *)manager didReceiveWriteRequests:(NSArray<CBATTRequest *> *)requests {
  CBATTRequest *request=requests.firstObject;
  if (!request) return;
  NearbyPipe *pipe=[self forCentral:request.central];
  BOOL accepted=manager==_advertiser && requests.count==1 && pipe && !request.offset &&
      [request.characteristic.UUID isEqual:rxID(_payment)] && [pipe feed:request.value];
  [manager respondToRequest:request withResult:accepted?CBATTErrorSuccess:CBATTErrorWriteNotPermitted];
  if (!accepted) [self retire:pipe];
}
- (void)peripheralManagerIsReadyToUpdateSubscribers:(CBPeripheralManager *)manager {
  if (manager==_advertiser) for (NearbyPipe *pipe in _links.allValues) [self pump:pipe];
}
- (void)queue:(NSData *)data pipe:(NearbyPipe *)pipe {
  if (!pipe || _links[pipe.identifier]!=pipe) return;
  if (_payment && !pipe.sentFirst) {
    const uint8_t route=2;
    NSMutableData *first=[NSMutableData dataWithBytes:&route length:1];
    [first appendData:data];
    data=first;
  }
  pipe.sentFirst=YES;
  NSUInteger unit=[self isServer] ? pipe.central.maximumUpdateValueLength : [pipe.peripheral maximumWriteValueLengthForType:CBCharacteristicWriteWithResponse];
  unit=MIN(unit,(NSUInteger)244);
  if (!unit || pipe.bytes+data.length>limit) { [self retire:pipe]; return; }
  for (NSUInteger offset=0;offset<data.length;offset+=unit)
    [pipe.fragments addObject:[data subdataWithRange:NSMakeRange(offset,MIN(unit,data.length-offset))]];
  pipe.bytes+=data.length;
  [self pump:pipe];
}
- (void)pump:(NearbyPipe *)pipe {
  if (!pipe || _links[pipe.identifier]!=pipe || !pipe.fragments.count || pipe.busy) return;
  if ([self isServer]) {
    while (pipe.fragments.count) {
      NSData *part=pipe.fragments.firstObject;
      if (![_advertiser updateValue:part forCharacteristic:_notify onSubscribedCentrals:@[pipe.central]]) return;
      pipe.bytes-=part.length; [pipe.fragments removeObjectAtIndex:0];
    }
  } else {
    pipe.busy=YES;
    [pipe.peripheral writeValue:pipe.fragments.firstObject forCharacteristic:pipe.rx type:CBCharacteristicWriteWithResponse];
    NSData *sent=pipe.fragments.firstObject;
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW,3*NSEC_PER_SEC),dispatch_get_main_queue(),^{
      if (pipe.busy && pipe.fragments.firstObject==sent) [self retire:pipe];
    });
  }
}
- (void)write:(NSString *)link clear:(NSString *)clear resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{
    NearbyPipe *pipe=self->_links[link];
    // Keep other one-use handles only while every connection is a TLS probe.
    if (pipe && self->_payment) [self->_candidates removeAllObjects];
    if (pipe && [pipe write:decode(clear,3072)]) resolve(nil);
    else { [self retire:pipe]; [self error:reject]; }
  });
}
- (void)close:(NSString *)link resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{ [self retire:self->_links[link]]; resolve(nil); });
}
- (void)stop:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{ [self teardown]; resolve(nil); });
}
- (void)paymentPublicIdentity:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{ Nearby *bridge=[self paymentBridge]; if (bridge) [bridge publicIdentity:resolve reject:reject]; else [self error:reject]; });
}
- (void)configurePayment:(NSString *)profile resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{ Nearby *bridge=[self paymentBridge]; if (bridge) [bridge configure:profile resolve:resolve reject:reject]; else [self error:reject]; });
}
- (void)scanPayment:(double)seconds resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{ Nearby *bridge=[self paymentBridge]; if (bridge) [bridge scan:seconds resolve:resolve reject:reject]; else [self error:reject]; });
}
- (void)connectPayment:(NSString *)handle resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{ Nearby *bridge=[self paymentBridge]; if (bridge) [bridge connect:handle resolve:resolve reject:reject]; else [self error:reject]; });
}
- (void)writePayment:(NSString *)link clear:(NSString *)clear resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{ Nearby *bridge=[self paymentBridge]; if (bridge) [bridge write:link clear:clear resolve:resolve reject:reject]; else [self error:reject]; });
}
- (void)closePayment:(NSString *)link resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{ Nearby *bridge=[self paymentBridge]; if (bridge) [bridge close:link resolve:resolve reject:reject]; else [self error:reject]; });
}
- (void)stopPayment:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{ Nearby *bridge=[self paymentBridge]; if (bridge) [bridge stop:resolve reject:reject]; else [self error:reject]; });
}
@end

#endif
