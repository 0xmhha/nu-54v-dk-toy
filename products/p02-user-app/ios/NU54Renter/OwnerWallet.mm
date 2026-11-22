#import <AVFoundation/AVFoundation.h>
#import <UIKit/UIKit.h>
#import <React/RCTUtils.h>
#import "InstallationDocument.h"
#import "OwnerRegistrationQA.h"
#import <CommonCrypto/CommonDigest.h>
#import <Network/Network.h>
#import <Security/Security.h>
#import <sys/sysctl.h>
#import <mach/mach_time.h>
#import <NU54RenterSpec/NU54RenterSpec.h>

static NSString *const serviceName = @"nu54.personal.profile.v2";
static const char *protocolName = "nu54-v2-management";
static const char *exporterLabel = "EXPORTER-NU54-wire2";

static NSData *bytes(NSString *base64) {
  if (base64.length>65536) return nil;
  return [[NSData alloc] initWithBase64EncodedString:base64 options:0];
}
static NSData *sha(NSData *data) {
  if (!data) return nil;
  uint8_t hash[CC_SHA256_DIGEST_LENGTH];
  CC_SHA256(data.bytes, (CC_LONG)data.length, hash);
  return [NSData dataWithBytes:hash length:sizeof(hash)];
}
static NSString *hex(NSData *data) {
  NSMutableString *result = [NSMutableString string];
  const uint8_t *source = static_cast<const uint8_t *>(data.bytes);
  for (NSUInteger i=0; i<data.length; ++i) [result appendFormat:@"%02x", source[i]];
  return result;
}
static NSData *spki(SecKeyRef publicKey) {
  if (!publicKey) return nil;
  NSDictionary *attributes = CFBridgingRelease(SecKeyCopyAttributes(publicKey));
  if (![attributes[(__bridge id)kSecAttrKeyType] isEqual:(__bridge id)kSecAttrKeyTypeECSECPrimeRandom] ||
      [attributes[(__bridge id)kSecAttrKeySizeInBits] intValue] != 256) return nil;
  NSData *point = CFBridgingRelease(SecKeyCopyExternalRepresentation(publicKey, nullptr));
  if (point.length != 65) return nil;
  const uint8_t prefix[] = {0x30,0x59,0x30,0x13,0x06,0x07,0x2a,0x86,0x48,0xce,0x3d,0x02,0x01,
      0x06,0x08,0x2a,0x86,0x48,0xce,0x3d,0x03,0x01,0x07,0x03,0x42,0x00};
  NSMutableData *encoded = [NSMutableData dataWithBytes:prefix length:sizeof(prefix)];
  [encoded appendData:point];
  return encoded;
}
static CFTypeRef retainedOwnerKey() CF_RETURNS_RETAINED {
#if defined(NU54_OWNER_REGISTRATION_QA)
  if (NU54OwnerRegistrationQA()) return NU54RegistrationTestKey();
#endif
  NSData *tag = [@"com.nu54renter.owner.p256.v2" dataUsingEncoding:NSUTF8StringEncoding];
  NSDictionary *query = @{(__bridge id)kSecClass:(__bridge id)kSecClassKey,
    (__bridge id)kSecAttrApplicationTag:tag, (__bridge id)kSecAttrKeyType:(__bridge id)kSecAttrKeyTypeECSECPrimeRandom,
    (__bridge id)kSecReturnRef:@YES};
  CFTypeRef existing = nullptr;
  OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &existing);
  if (status==errSecSuccess) {
    NSDictionary *attrs = CFBridgingRelease(SecKeyCopyAttributes((SecKeyRef)existing));
    if ([attrs[(__bridge id)kSecAttrTokenID] isEqual:(__bridge id)kSecAttrTokenIDSecureEnclave]) return existing;
    CFRelease(existing);
    return nullptr;
  }
  if (status!=errSecItemNotFound) return nullptr;
  id control = CFBridgingRelease(SecAccessControlCreateWithFlags(kCFAllocatorDefault,
     kSecAttrAccessibleWhenUnlockedThisDeviceOnly,kSecAccessControlPrivateKeyUsage,nullptr));
  if (!control) return nullptr;
  NSDictionary *options=@{(__bridge id)kSecAttrKeyType:(__bridge id)kSecAttrKeyTypeECSECPrimeRandom,
     (__bridge id)kSecAttrKeySizeInBits:@256, (__bridge id)kSecAttrTokenID:(__bridge id)kSecAttrTokenIDSecureEnclave,
     (__bridge id)kSecPrivateKeyAttrs:@{(__bridge id)kSecAttrIsPermanent:@YES,(__bridge id)kSecAttrApplicationTag:tag,
       (__bridge id)kSecAttrAccessControl:control}};
  return SecKeyCreateRandomKey((__bridge CFDictionaryRef)options,nullptr);
}

@interface NU54WalletRPC : NSObject <NSURLSessionDataDelegate>
@property(nonatomic,copy) RCTPromiseResolveBlock resolve;
@property(nonatomic,copy) RCTPromiseRejectBlock reject;
@property(nonatomic,strong) NSMutableData *result;
@end

@implementation NU54WalletRPC
- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task
 willPerformHTTPRedirection:(NSHTTPURLResponse *)response newRequest:(NSURLRequest *)request
 completionHandler:(void (^)(NSURLRequest *))completionHandler {
  completionHandler(nil);
}
- (void)URLSession:(NSURLSession *)session dataTask:(NSURLSessionDataTask *)task
 didReceiveResponse:(NSURLResponse *)response completionHandler:(void (^)(NSURLSessionResponseDisposition))handler {
  NSHTTPURLResponse *http=(NSHTTPURLResponse *)response;
  handler([http isKindOfClass:NSHTTPURLResponse.class] && http.statusCode==200 && http.expectedContentLength<=256000
          ? NSURLSessionResponseAllow : NSURLSessionResponseCancel);
}
- (void)URLSession:(NSURLSession *)session dataTask:(NSURLSessionDataTask *)task didReceiveData:(NSData *)data {
  if (data.length>256000-self.result.length) { [task cancel]; return; }
  [self.result appendData:data];
}
- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task didCompleteWithError:(NSError *)error {
  NSString *answer=error ? nil : [[NSString alloc] initWithData:self.result encoding:NSUTF8StringEncoding];
  if (answer.length) self.resolve(answer);
  else self.reject(@"EVM_RPC", @"The requested network response is unavailable.", nil);
  [session finishTasksAndInvalidate];
}
@end

@interface OwnerWallet : NativeOwnerWalletSpecBase <NativeOwnerWalletSpec, UIDocumentPickerDelegate>
@end
@implementation OwnerWallet {
  dispatch_queue_t _work;
  AVSpeechSynthesizer *_speech;
  nw_listener_t _listener;
  nw_path_monitor_t _pathMonitor;
  NSString *_networkLink;
  nw_connection_t _client;
  nw_connection_t _relay;
  NSMutableData *_beforeRelay;
  NSString *_pin;
  NSData *_epoch;
  NSUInteger _session;
  NSUInteger _relayQueued;
  NSUInteger _clearQueued;
  BOOL _psbtDocument;
  RCTPromiseResolveBlock _fileResolve;
  RCTPromiseRejectBlock _fileReject;
}

RCT_EXPORT_MODULE(OwnerWallet)

- (instancetype)init {
  if ((self=[super init])) {
    _work=dispatch_queue_create("nu54.owner", DISPATCH_QUEUE_SERIAL);
    _networkLink=@"unknown";
  }
  return self;
}
- (std::shared_ptr<facebook::react::TurboModule>)getTurboModule:
    (const facebook::react::ObjCTurboModule::InitParams &)params {
  return std::make_shared<facebook::react::NativeOwnerWalletSpecJSI>(params);
}
- (void)dealloc {
  if (_pathMonitor) nw_path_monitor_cancel(_pathMonitor);
}

- (void)getNetworkLink:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work, ^{
    if (!self->_pathMonitor) {
      self->_pathMonitor=nw_path_monitor_create();
      nw_path_monitor_set_queue(self->_pathMonitor,self->_work);
      __weak OwnerWallet *weakOwner=self;
      nw_path_monitor_set_update_handler(self->_pathMonitor,^(nw_path_t path) {
        OwnerWallet *owner=weakOwner;
        if (!owner) return;
        NSString *value=nw_path_get_status(path)==nw_path_status_satisfied ? @"online" : @"offline";
        if ([value isEqualToString:owner->_networkLink]) return;
        owner->_networkLink=value;
        [owner emitOnNetworkLink:value];
      });
      nw_path_monitor_start(self->_pathMonitor);
    }
    resolve(self->_networkLink);
  });
}

- (void)setHomeProtection:(BOOL)enabled resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(), ^{
    [[NSNotificationCenter defaultCenter] postNotificationName:@"NU54HomeProtection" object:nil userInfo:@{@"enabled": @(enabled)}];
    resolve(nil);
  });
}
// The iOS app has no demo wallet build; the fixture screens exist only for the Android QA build.
- (void)demoWallet:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject { resolve(@NO); }
- (void)unavailable:(RCTPromiseRejectBlock)reject { reject(@"OWNER_UNAVAILABLE", @"The requested owner operation is unavailable.", nil); }

- (NSMutableDictionary *)profileQuery {
  return [@{(__bridge id)kSecClass:(__bridge id)kSecClassGenericPassword,
            (__bridge id)kSecAttrService:serviceName,(__bridge id)kSecAttrAccount:@"phone-local",
            (__bridge id)kSecAttrSynchronizable:@NO} mutableCopy];
}
static const NSUInteger walletProfileLimit = 32768;

- (NSString *)readProtectedProfile {
  NSMutableDictionary *query=[self profileQuery];
  query[(__bridge id)kSecReturnData]=@YES;
  CFTypeRef raw=nullptr;
  OSStatus rc=SecItemCopyMatching((__bridge CFDictionaryRef)query,&raw);
  if (rc==errSecItemNotFound) return @"";
  NSData *data=CFBridgingRelease(raw);
  if (rc!=errSecSuccess || !data.length || data.length>walletProfileLimit) return nil;
  return [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
}

- (BOOL)writeProtectedProfile:(NSString *)value {
  NSData *data=[value dataUsingEncoding:NSUTF8StringEncoding];
  if (!data.length || data.length>walletProfileLimit) return NO;
  NSMutableDictionary *query=[self profileQuery];
  NSDictionary *update=@{(__bridge id)kSecValueData:data};
  OSStatus rc=SecItemUpdate((__bridge CFDictionaryRef)query,(__bridge CFDictionaryRef)update);
  if (rc==errSecItemNotFound) {
    [query addEntriesFromDictionary:update];
    query[(__bridge id)kSecAttrAccessible]=(__bridge id)kSecAttrAccessibleWhenUnlockedThisDeviceOnly;
    rc=SecItemAdd((__bridge CFDictionaryRef)query,nullptr);
  }
  NSString *committed=rc==errSecSuccess ? [self readProtectedProfile] : nil;
  return committed && [committed compare:value options:NSLiteralSearch]==NSOrderedSame;
}

- (void)loadProfile:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    @synchronized (OwnerWallet.class) {
      NSString *value=[self readProtectedProfile];
      if (value) resolve(value); else [self unavailable:reject];
    }
  });
}

- (void)saveProfile:(NSString *)value resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    @synchronized (OwnerWallet.class) {
      NSString *current=[self readProtectedProfile];
      if (current && !current.length && [self writeProtectedProfile:value]) resolve(nil);
      else [self unavailable:reject];
    }
  });
}

- (void)compareAndSaveProfile:(NSString *)expected value:(NSString *)value
      resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    @synchronized (OwnerWallet.class) {
      if ([expected lengthOfBytesUsingEncoding:NSUTF8StringEncoding]>walletProfileLimit) { [self unavailable:reject]; return; }
      NSString *current=[self readProtectedProfile];
      if (!current) { [self unavailable:reject]; return; }
      if ([current compare:expected options:NSLiteralSearch]!=NSOrderedSame) { resolve(@NO); return; }
      if ([self writeProtectedProfile:value]) resolve(@YES); else [self unavailable:reject];
    }
  });
}
- (void)publicIdentity:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    id privateKey=CFBridgingRelease(retainedOwnerKey());
    if (!privateKey) { [self unavailable:reject]; return; }
    id publicKey=CFBridgingRelease(SecKeyCopyPublicKey((__bridge SecKeyRef)privateKey));
    NSData *identity=spki((__bridge SecKeyRef)publicKey);
    if (identity.length) resolve([identity base64EncodedStringWithOptions:0]);
    else [self unavailable:reject];
  });
}
- (void)signChallenge:(NSString *)record resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    NSData *data=bytes(record);
    if (!self->_epoch || data.length!=130) { [self unavailable:reject]; return; }
    const uint8_t *raw=static_cast<const uint8_t *>(data.bytes);
    if (raw[0]!=1 || raw[1]<1 || raw[1]>5) { [self unavailable:reject]; return; }
    id key=CFBridgingRelease(retainedOwnerKey());
    if (!key) { [self unavailable:reject]; return; }
    id pub=CFBridgingRelease(SecKeyCopyPublicKey((__bridge SecKeyRef)key));
    if (![[data subdataWithRange:NSMakeRange(42,32)] isEqual:sha(spki((__bridge SecKeyRef)pub))]) {
      [self unavailable:reject]; return;
    }
    NSData *proof=CFBridgingRelease(SecKeyCreateSignature((__bridge SecKeyRef)key,
        kSecKeyAlgorithmECDSASignatureMessageX962SHA256,(__bridge CFDataRef)data,nullptr));
    if (proof.length) resolve([proof base64EncodedStringWithOptions:0]); else [self unavailable:reject];
  });
}
- (void)evmRpc:(NSString *)endpoint packet:(NSString *)packet
       resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  NSURLComponents *url=[NSURLComponents componentsWithString:endpoint];
  NSData *body=[packet dataUsingEncoding:NSUTF8StringEncoding];
  NSDictionary *command=body.length && body.length<=6144 ? [NSJSONSerialization JSONObjectWithData:body options:0 error:nullptr] : nil;
  NSSet *allowed=[NSSet setWithArray:@[@"eth_chainId", @"eth_getBalance",@"eth_getBlockByNumber",@"eth_getTransactionReceipt",@"eth_sendRawTransaction",@"nu54_walletRegistry",@"nu54_walletQuote",@"nu54_marketBoardGrant",@"nu54_walletLogin",@"eth_call",@"nu54_walletRevokeAllowance",@"nu54_walletExternalSpend",@"nu54_walletDappAllowance",@"nu54_walletConnectPeer"]];
  NSSet *laboratory=[NSSet setWithArray:@[@"nu54_bip48Policy", @"nu54_bip48Prepare", @"nu54_bitcoinReplacementStatus", @"nu54_submitBitcoinReplacement", @"nu54_walletTime", @"nu54_prepareWalletTransfer", @"getblockhash",@"getblockchaininfo",@"getrawtransaction",@"testmempoolaccept", @"listunspent", @"getaddressinfo",@"sendrawtransaction",
    @"getGenesisHash",@"getSignatureStatuses",@"isBlockhashValid", @"getBalance",@"getBlockHeight",@"sendTransaction",
    @"server_info",@"tx",@"ledger_current", @"account_info", @"ledger",@"submit"]];
  NSString *method=[command isKindOfClass:NSDictionary.class] ? command[@"method"] : nil;
  NSSet *loopback=[NSSet setWithArray:@[@"localhost",@"127.0.0.1",@"::1",@"[::1]"]];
  NSSet *externalReads=[NSSet setWithArray:@[@"getblockchaininfo",@"getblockhash", @"gettxout",@"getaddressinfo", @"getreceivedbyaddress",@"listunspent",
    @"getGenesisHash",@"getBalance",@"server_info",@"account_info",@"ledger", @"getAccountInfo", @"getTokenAccountsByOwner", @"account_lines"]];
  NSString *signetRoute=@"^https://mempool\\.space/signet/api/(block-height/[0-9]{1,9}|blocks/tip/(height|hash)|address/tb1q[a-z0-9]{38}(/utxo)?|block/[a-f0-9]{64}/header|tx/[a-f0-9]{64}/(merkle-proof|hex))$";
  BOOL signet=[method isEqual:@"nu54_signetRead"] && command.count==1 &&
    [endpoint rangeOfString:signetRoute options:NSRegularExpressionSearch].location!=NSNotFound;
  id token=[command isKindOfClass:NSDictionary.class] ? command[@"walletToken"] : nil;
  NSDictionary *commissionedRoutes=@{
    @"/bitcoin":@[@"nu54_bip48Policy",@"nu54_bip48Prepare",@"getblockhash",@"getblockchaininfo",@"getrawtransaction",@"testmempoolaccept",@"getaddressinfo",@"listunspent"],
    @"/solana":@[@"getGenesisHash",@"getSignatureStatuses",@"isBlockhashValid",@"getBlockHeight",@"getBalance"],
    @"/xrpl":@[@"server_info",@"tx",@"ledger_current",@"account_info",@"ledger"]
  };
  NSArray *routeMethods=commissionedRoutes[url.percentEncodedPath ?: @""];
  NSString *dnsHost=@"^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\\.)+[a-z]{2,63}$";
  BOOL qaLoopback=NO;
#if defined(NU54_WALLET_SEND_QA)
  qaLoopback=[NSBundle.mainBundle.bundleIdentifier isEqualToString:@"com.nu54renter.wallet.sendqa"] &&
    [url.host isEqualToString:@"wallet.localhost"] && [url.port isEqualToNumber:@18449];
#endif
  BOOL reservedHost=[url.host hasSuffix:@".localhost"];
  BOOL commissioned=routeMethods && (!reservedHost || qaLoopback) && !url.query && [url.host rangeOfString:dnsHost options:NSRegularExpressionSearch].location!=NSNotFound &&
    [token isKindOfClass:NSString.class] && [token rangeOfString:@"^[a-f0-9]{64}$" options:NSRegularExpressionSearch].location!=NSNotFound &&
    method && ([routeMethods containsObject:method] ||
      [@[@"nu54_walletTime",@"nu54_prepareWalletTransfer",@"nu54_submitCommissionedTransfer"] containsObject:method]);
  BOOL allowedMethod=token ? commissioned : signet || (method && ((([allowed containsObject:method] || [externalReads containsObject:method]) && body.length<=2048) ||
    ([laboratory containsObject:method] && [loopback containsObject:url.host ?: NSNull.null] && !url.query)));
  if (endpoint.length>512 || ![url.scheme isEqualToString:@"https"] || !url.host.length || url.user || url.password ||
      url.fragment || !url.URL || !allowedMethod) {
    [self unavailable:reject]; return;
  }
  NSMutableURLRequest *request=[NSMutableURLRequest requestWithURL:url.URL];
  request.HTTPMethod=signet ? @"GET" : @"POST";
  if (!signet) {
    request.HTTPBody=body;
    [request setValue:@"application/json" forHTTPHeaderField:@"Content-Type"];
  }
  NSURLSessionConfiguration *configuration=NSURLSessionConfiguration.ephemeralSessionConfiguration;
  configuration.timeoutIntervalForRequest=8;
  configuration.timeoutIntervalForResource=8;
  configuration.HTTPCookieStorage=nil;
  NU54WalletRPC *exchange=[NU54WalletRPC new];
  exchange.resolve=resolve;
  exchange.reject=reject;
  exchange.result=[NSMutableData data];
  NSURLSession *session=[NSURLSession sessionWithConfiguration:configuration delegate:exchange delegateQueue:nil];
  [[session dataTaskWithRequest:request] resume];
}
- (void)randomBytes:(double)size resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  if (size<1 || size>64 || size!=(NSUInteger)size) { [self unavailable:reject]; return; }
  NSMutableData *random=[NSMutableData dataWithLength:(NSUInteger)size];
  if (SecRandomCopyBytes(kSecRandomDefault,random.length,random.mutableBytes)==errSecSuccess)
    resolve([random base64EncodedStringWithOptions:0]);
  else [self unavailable:reject];
}
- (void)pickWalletDocument:(BOOL)psbt resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(), ^{
    if (self->_fileResolve || !RCTPresentedViewController()) { [self unavailable:reject]; return; }
    self->_fileResolve = [resolve copy]; self->_fileReject = [reject copy]; self->_psbtDocument = psbt;
    UIDocumentPickerViewController *picker = [[UIDocumentPickerViewController alloc] initWithDocumentTypes:(psbt ? @[@"public.data"] : @[@"public.json"]) inMode:UIDocumentPickerModeImport];
    picker.delegate = self; picker.allowsMultipleSelection = NO;
#if defined(NU54_WALLET_SEND_QA)
    if (psbt && [NSBundle.mainBundle.bundleIdentifier isEqualToString:@"com.nu54renter.wallet.sendqa"]) {
      picker.directoryURL=[NSFileManager.defaultManager URLsForDirectory:NSDocumentDirectory inDomains:NSUserDomainMask].firstObject;
    }
#endif
#if defined(NU54_FIRMWARE_CANDIDATE_QA)
    if (!psbt && [NSBundle.mainBundle.bundleIdentifier isEqualToString:@"com.nu54renter.w04qa"]) {
      picker.directoryURL=[NSFileManager.defaultManager URLsForDirectory:NSDocumentDirectory inDomains:NSUserDomainMask].firstObject;
    }
#endif
    [RCTPresentedViewController() presentViewController:picker animated:YES completion:nil];
  });
}
- (void)pickInstallationFile:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  [self pickWalletDocument:NO resolve:resolve reject:reject];
}
- (void)pickPsbtFile:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  [self pickWalletDocument:YES resolve:resolve reject:reject];
}
- (void)documentPickerWasCancelled:(UIDocumentPickerViewController *)controller {
  RCTPromiseRejectBlock reject = _fileReject; _fileResolve = nil; _fileReject = nil;
  if (reject) reject(@"FILE_CANCELLED", @"No file selected.", nil);
}
- (void)documentPicker:(UIDocumentPickerViewController *)controller didPickDocumentsAtURLs:(NSArray<NSURL *> *)urls {
  RCTPromiseResolveBlock resolve = _fileResolve; RCTPromiseRejectBlock reject = _fileReject;
  BOOL psbt=_psbtDocument; _psbtDocument=NO;
  _fileResolve = nil; _fileReject = nil;
  NSURL *url = urls.count == 1 ? urls.firstObject : nil;
  if (!resolve || !reject || !url) { if (reject) [self unavailable:reject]; return; }
  dispatch_async(dispatch_get_global_queue(QOS_CLASS_USER_INITIATED,0), ^{
    NSString *text = psbt ? NU54ReadPsbtDocument(url) : NU54ReadInstallationDocument(url);
    if (!text) { [self unavailable:reject]; return; }
    resolve(text);
  });
}
- (void)installationClock:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  char boot[128] = {}; size_t length = sizeof(boot);
  mach_timebase_info_data_t scale; if (sysctlbyname("kern.bootsessionuuid",boot,&length,nullptr,0) != 0 ||
      length < 2 || length > sizeof(boot) || mach_timebase_info(&scale) != KERN_SUCCESS || scale.denom == 0) {
    [self unavailable:reject]; return;
  }
  NSString *identity = [NSString stringWithUTF8String:boot];
  if (!identity.length) { [self unavailable:reject]; return; }
  double milliseconds = ((long double)mach_continuous_time() * scale.numer / scale.denom) / 1000000.0L;
  NSDictionary *value = @{@"boot":identity.lowercaseString, @"uptimeMs":@((long long)milliseconds),
                          @"wallMs":@((long long)(NSDate.date.timeIntervalSince1970 * 1000.0))};
  NSData *json = [NSJSONSerialization dataWithJSONObject:value options:0 error:nil];
  NSString *text = [[NSString alloc] initWithData:json encoding:NSUTF8StringEncoding];
  if (!text) { [self unavailable:reject]; return; }
  resolve(text);
}
- (void)speak:(NSString *)text language:(NSString *)language resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  if ((! [language isEqualToString:@"ko"] && ![language isEqualToString:@"en"]) || !text.length || text.length>400) { [self unavailable:reject]; return; }
  dispatch_async(dispatch_get_main_queue(),^{
    AVSpeechSynthesisVoice *voice=[AVSpeechSynthesisVoice voiceWithLanguage:[language isEqualToString:@"ko"] ? @"ko-KR" : @"en-US"];
    if (!voice) { [self unavailable:reject]; return; }
    if (!self->_speech) self->_speech=[AVSpeechSynthesizer new];
    [self->_speech stopSpeakingAtBoundary:AVSpeechBoundaryImmediate];
    AVSpeechUtterance *phrase=[AVSpeechUtterance speechUtteranceWithString:text];
    phrase.voice=voice;
    [self->_speech speakUtterance:phrase]; resolve(nil);
  });
}

- (NSString *)message:(NSData *)cipher clear:(NSData *)clear error:(BOOL)error {
  NSDictionary *event=@{@"out":[cipher base64EncodedStringWithOptions:0] ?: @"",
    @"clear":[clear base64EncodedStringWithOptions:0] ?: @"",
    @"epoch":[_epoch base64EncodedStringWithOptions:0] ?: @"", @"peer":_epoch ? _pin : @"",
    @"closed":@(error)};
  return [[NSString alloc] initWithData:[NSJSONSerialization dataWithJSONObject:event options:0 error:nil] encoding:NSUTF8StringEncoding];
}
- (void)stop {
  ++_session;
  if (_listener) nw_listener_cancel(_listener);
  if (_relay) nw_connection_cancel(_relay);
  if (_client) nw_connection_cancel(_client);
  _listener=nil; _relay=nil; _client=nil; _epoch=nil; _pin=nil; _beforeRelay=nil;
  _relayQueued=0; _clearQueued=0;
}
- (void)abort:(NSUInteger)token {
  if (token!=_session) return;
  [self emitOnTls:[self message:nil clear:nil error:YES]];
  [self stop];
}
- (void)receive:(nw_connection_t)connection encrypted:(BOOL)encrypted token:(NSUInteger)token {
  __weak OwnerWallet *weakSelf=self;
  nw_connection_receive(connection,1,8192,^(dispatch_data_t content,nw_content_context_t context,bool done,nw_error_t error) {
    OwnerWallet *owner=weakSelf;
    if (!owner || owner->_session!=token) return;
    if (error || done) { [owner abort:token]; return; }
    if (content && dispatch_data_get_size(content)) {
      const void *mapping=nullptr; size_t length=0;
      dispatch_data_t mapped=dispatch_data_create_map(content,&mapping,&length);
      NSData *payload=[NSData dataWithBytes:mapping length:length];
      (void)mapped;
      if (!encrypted && !owner->_epoch) { [owner abort:token]; return; }
      [owner emitOnTls:[owner message:encrypted?payload:nil clear:encrypted?nil:payload error:NO]];
    }
    [owner receive:connection encrypted:encrypted token:token];
  });
}
- (void)send:(NSData *)data on:(nw_connection_t)connection encrypted:(BOOL)encrypted token:(NSUInteger)token {
  NSUInteger *queued=encrypted ? &_relayQueued : &_clearQueued;
  if (!connection || *queued+data.length>49152) { [self abort:token]; return; }
  *queued+=data.length;
  dispatch_data_t packet=dispatch_data_create(data.bytes,data.length,_work,^{ (void)data; });
  __weak OwnerWallet *weakSelf=self;
  nw_connection_send(connection,packet,NW_CONNECTION_DEFAULT_MESSAGE_CONTEXT,true,^(nw_error_t error){
    OwnerWallet *owner=weakSelf;
    if (!owner || token!=owner->_session) return;
    if (encrypted) owner->_relayQueued-=data.length; else owner->_clearQueued-=data.length;
    if (error) [owner abort:token];
  });
}

- (void)startTls:(NSString *)caDer clientChain:(NSArray *)clientChain boardSPKI:(NSString *)boardSPKI
         resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    [self stop];
    if (boardSPKI.length!=64 || [boardSPKI rangeOfCharacterFromSet:[[NSCharacterSet characterSetWithCharactersInString:@"0123456789abcdef"] invertedSet]].location!=NSNotFound ||
        clientChain.count<1 || clientChain.count>4) { [self unavailable:reject]; return; }
    NSData *rootData=bytes(caDer);
    id ca=rootData ? CFBridgingRelease(SecCertificateCreateWithData(nullptr,(__bridge CFDataRef)rootData)) : nil;
    id key=CFBridgingRelease(retainedOwnerKey());
    NSMutableArray *certs=[NSMutableArray array];
    for (NSString *item in clientChain) {
      NSData *encoded=bytes(item);
      id crt=encoded ? CFBridgingRelease(SecCertificateCreateWithData(nullptr,(__bridge CFDataRef)encoded)) : nil;
      if (!crt) { [self unavailable:reject]; return; }
      [certs addObject:crt];
    }
    id identity=key ? CFBridgingRelease(SecIdentityCreate(nullptr,(__bridge SecCertificateRef)certs[0],(__bridge SecKeyRef)key)) : nil;
    if (!ca || !identity) { [self unavailable:reject]; return; }
    NSArray *additional=[certs subarrayWithRange:NSMakeRange(1,certs.count-1)];
    sec_identity_t signer=sec_identity_create_with_certificates((__bridge SecIdentityRef)identity,(__bridge CFArrayRef)additional);
    if (!signer) { [self unavailable:reject]; return; }
    self->_pin=[boardSPKI copy];
    self->_beforeRelay=[NSMutableData new];
    NSUInteger token=self->_session;
    __weak OwnerWallet *weakSelf=self;
    nw_parameters_t local=nw_parameters_create_secure_tcp(NW_PARAMETERS_DISABLE_PROTOCOL,NW_PARAMETERS_DEFAULT_CONFIGURATION);
    nw_parameters_set_local_endpoint(local,nw_endpoint_create_host("127.0.0.1","0"));
    self->_listener=nw_listener_create(local);
    if (!self->_listener) { [self unavailable:reject]; return; }
    nw_listener_set_queue(self->_listener,self->_work);
    nw_listener_set_new_connection_handler(self->_listener,^(nw_connection_t relay){
      OwnerWallet *owner=weakSelf;
      if (!owner || token!=owner->_session || owner->_relay) { nw_connection_cancel(relay); return; }
      owner->_relay=relay; nw_connection_set_queue(relay,owner->_work); nw_connection_start(relay);
      [owner receive:relay encrypted:YES token:token];
      if (owner->_beforeRelay.length) [owner send:[owner->_beforeRelay copy] on:relay encrypted:YES token:token];
      [owner->_beforeRelay setLength:0];
    });
    nw_listener_set_state_changed_handler(self->_listener,^(nw_listener_state_t state,nw_error_t failure){
      OwnerWallet *owner=weakSelf;
      if (!owner || token!=owner->_session) return;
      if (state==nw_listener_state_failed) { [owner abort:token]; return; }
      if (state!=nw_listener_state_ready || owner->_client) return;
      NSString *port=[NSString stringWithFormat:@"%u",nw_listener_get_port(owner->_listener)];
      dispatch_queue_t queue=owner->_work;
      nw_parameters_t tls=nw_parameters_create_secure_tcp(^(nw_protocol_options_t options){
        sec_protocol_options_t security=nw_tls_copy_sec_protocol_options(options);
        sec_protocol_options_set_min_tls_protocol_version(security,tls_protocol_version_TLSv13);
        sec_protocol_options_set_max_tls_protocol_version(security,tls_protocol_version_TLSv13);
        sec_protocol_options_set_tls_resumption_enabled(security,false);
        sec_protocol_options_set_tls_tickets_enabled(security,false);
        sec_protocol_options_set_peer_authentication_required(security,true);
        sec_protocol_options_add_tls_application_protocol(security,protocolName);
        sec_protocol_options_set_local_identity(security,signer);
        sec_protocol_options_set_verify_block(security,^(sec_protocol_metadata_t metadata, sec_trust_t trust, sec_protocol_verify_complete_t complete) {
          id checked=CFBridgingRelease(sec_trust_copy_ref(trust));
          if (!checked) { complete(false); return; }
          SecTrustRef check=(__bridge SecTrustRef)checked;
          id policy=CFBridgingRelease(SecPolicyCreateSSL(true,nullptr));
          bool okay=SecTrustSetPolicies(check,(__bridge CFTypeRef)policy)==errSecSuccess &&
                    SecTrustSetAnchorCertificates(check,(__bridge CFArrayRef)@[ca])==errSecSuccess &&
                    SecTrustSetAnchorCertificatesOnly(check,true)==errSecSuccess &&
                    SecTrustEvaluateWithError(check,nullptr);
          id peer=okay ? CFBridgingRelease(SecTrustCopyKey(check)) : nil;
          complete(peer && [hex(sha(spki((__bridge SecKeyRef)peer))) isEqualToString:boardSPKI]);
        },queue);
      },NW_PARAMETERS_DEFAULT_CONFIGURATION);
      nw_connection_t client=nw_connection_create(nw_endpoint_create_host("127.0.0.1",port.UTF8String),tls);
      owner->_client=client;
      nw_connection_set_queue(client,queue);
      nw_connection_set_state_changed_handler(client,^(nw_connection_state_t link,nw_error_t failure){
        OwnerWallet *peer=weakSelf;
        if (!peer || token!=peer->_session) return;
        if (link==nw_connection_state_failed || link==nw_connection_state_cancelled) { [peer abort:token]; return; }
        if (link!=nw_connection_state_ready) return;
        nw_protocol_metadata_t data=nw_connection_copy_protocol_metadata(peer->_client,nw_protocol_copy_tls_definition());
        sec_protocol_metadata_t metadata=data ? nw_tls_copy_sec_protocol_metadata(data) : nil;
        const char *alpn=metadata ? sec_protocol_metadata_get_negotiated_protocol(metadata) : nullptr;
        if (!alpn || strcmp(alpn,protocolName) || sec_protocol_metadata_get_negotiated_tls_protocol_version(metadata)!=tls_protocol_version_TLSv13) {
          [peer abort:token]; return;
        }
        // P01 binds the wire2 epoch to both the TLS session and its negotiated profile.
        dispatch_data_t secret=sec_protocol_metadata_create_secret_with_context(metadata,strlen(exporterLabel),exporterLabel,
          strlen(alpn),reinterpret_cast<const uint8_t *>(alpn),16);
        if (!secret || dispatch_data_get_size(secret)!=16) { [peer abort:token]; return; }
        const void *mapping=nullptr; size_t length=0;
        dispatch_data_t mapped=dispatch_data_create_map(secret,&mapping,&length);
        peer->_epoch=[NSData dataWithBytes:mapping length:length]; (void)mapped;
        [peer emitOnTls:[peer message:nil clear:nil error:NO]];
        [peer receive:peer->_client encrypted:NO token:token];
      });
      nw_connection_start(client);
    });
    nw_listener_start(self->_listener);
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW,10*NSEC_PER_SEC),self->_work,^{
      if (self->_session==token && !self->_epoch) [self abort:token];
    });
    resolve([self message:nil clear:nil error:NO]);
  });
}
- (void)feedTls:(NSString *)input resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    NSData *data=bytes(input);
    if (!self->_listener || !data.length || data.length>8192) { [self unavailable:reject]; return; }
    if (self->_relay) [self send:data on:self->_relay encrypted:YES token:self->_session];
    else if (self->_beforeRelay.length+data.length<=49152) [self->_beforeRelay appendData:data];
    else { [self abort:self->_session]; [self unavailable:reject]; return; }
    resolve([self message:nil clear:nil error:NO]);
  });
}
- (void)sendTls:(NSString *)cleartext resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    NSData *data=bytes(cleartext);
    if (!self->_client || !self->_epoch || !data.length || data.length>3072) { [self unavailable:reject]; return; }
    [self send:data on:self->_client encrypted:NO token:self->_session];
    resolve([self message:nil clear:nil error:NO]);
  });
}
- (void)stopTls:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{ [self stop]; resolve(nil); });
}
@end
