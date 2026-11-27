#import <CommonCrypto/CommonDigest.h>
#import <Security/Security.h>
#if __has_include(<NU54RenterSpec/NU54RenterSpec.h>)
#import <NU54RenterSpec/NU54RenterSpec.h>
#define CLIENT_SLOTS @[@"customer",@"travel"]
#define NU54_TRAVEL_RECORDER 1
#import <AVFoundation/AVFoundation.h>
#else
#import <NU54KioskSpec/NU54KioskSpec.h>
#define CLIENT_SLOTS @[@"terminal",@"operator",@"finance"]
#endif

static NSString * const service = @"nu54.commerce.enrollment.v2";

static NSData *spki(SecKeyRef key) {
  if (!key) return nil;
  NSDictionary *attributes = CFBridgingRelease(SecKeyCopyAttributes(key));
  if (![attributes[(__bridge id)kSecAttrKeyType] isEqual:(__bridge id)kSecAttrKeyTypeECSECPrimeRandom] ||
      [attributes[(__bridge id)kSecAttrKeySizeInBits] intValue]!=256) return nil;
  NSData *point=CFBridgingRelease(SecKeyCopyExternalRepresentation(key,nullptr));
  if (point.length!=65) return nil;
  const uint8_t prefix[]={0x30,0x59,0x30,0x13,0x06,0x07,0x2a,0x86,0x48,0xce,0x3d,0x02,0x01,
      0x06,0x08,0x2a,0x86,0x48,0xce,0x3d,0x03,0x01,0x07,0x03,0x42,0x00};
  NSMutableData *encoded=[NSMutableData dataWithBytes:prefix length:sizeof(prefix)];
  [encoded appendData:point];
  return encoded;
}

static NSString *fingerprint(NSData *data) {
  if (!data) return nil;
  uint8_t hash[CC_SHA256_DIGEST_LENGTH];
  CC_SHA256(data.bytes,(CC_LONG)data.length,hash);
  NSMutableString *out=[NSMutableString string];
  for (uint8_t byte : hash) [out appendFormat:@"%02x",byte];
  return out;
}

static CFTypeRef privateKey(NSString *slot) CF_RETURNS_RETAINED {
  if (![CLIENT_SLOTS containsObject:slot]) return nullptr;
  NSData *tag=[[service stringByAppendingFormat:@".%@",slot] dataUsingEncoding:NSUTF8StringEncoding];
  NSDictionary *query=@{(__bridge id)kSecClass:(__bridge id)kSecClassKey,
     (__bridge id)kSecAttrApplicationTag:tag,(__bridge id)kSecReturnRef:@YES};
  CFTypeRef item=nullptr;
  OSStatus rc=SecItemCopyMatching((__bridge CFDictionaryRef)query,&item);
  if (rc==errSecSuccess) return item;
  if (rc!=errSecItemNotFound) return nullptr;
  id access=CFBridgingRelease(SecAccessControlCreateWithFlags(nullptr,
      kSecAttrAccessibleWhenUnlockedThisDeviceOnly,kSecAccessControlPrivateKeyUsage,nullptr));
  if (!access) return nullptr;
  NSDictionary *attrs=@{(__bridge id)kSecAttrKeyType:(__bridge id)kSecAttrKeyTypeECSECPrimeRandom,
     (__bridge id)kSecAttrKeySizeInBits:@256,(__bridge id)kSecAttrTokenID:(__bridge id)kSecAttrTokenIDSecureEnclave,
     (__bridge id)kSecPrivateKeyAttrs:@{(__bridge id)kSecAttrIsPermanent:@YES,
        (__bridge id)kSecAttrApplicationTag:tag,(__bridge id)kSecAttrAccessControl:access}};
  return SecKeyCreateRandomKey((__bridge CFDictionaryRef)attrs,nullptr);
}

static id certificate(id raw) {
  if (![raw isKindOfClass:NSString.class] || [raw length]>10000) return nil;
  NSData *der=[[NSData alloc] initWithBase64EncodedString:raw options:0];
  return der ? CFBridgingRelease(SecCertificateCreateWithData(nullptr,(__bridge CFDataRef)der)) : nil;
}

static BOOL journalName(NSString *name) {
#ifndef NU54_TRAVEL_RECORDER
  if ([@[@"commerce-nearby-inbox-v2",@"commerce-denials-v2"] containsObject:name]) return YES;
#endif
  return [@[@"commerce-inbox-v2",@"customer-order-v2",@"customer-payment-v2",@"customer-qr-v2",@"customer-stores-v1",@"terminal-payment-v2",@"terminal-mode-request-v2",@"terminal-mode-applied-v2",@"terminal-public-menu-v1",@"travel-pickup-kitchen-v1",@"subscription-alerts-v2",@"market-lab-8283-v1"] containsObject:name]
      || ((name.length<=81) && [name rangeOfString:@"^(cloud-p03-[a-f0-9]{64}|travel-local-[a-f0-9-]{36}|travel-hold-[a-f0-9]{64}|pickup-v2-[a-f0-9]{64}|print-v1-[a-f0-9]{64}|pickup-legacy-[a-f0-9]{64}|customer-(order|payment|qr|legacy)-[a-f0-9]{64})$" options:NSRegularExpressionSearch].location!=NSNotFound);
}
static NSMutableDictionary *profileQuery(NSString *slot) {
  return [@{(__bridge id)kSecClass:(__bridge id)kSecClassGenericPassword,(__bridge id)kSecAttrService:service,
    (__bridge id)kSecAttrAccount:slot,(__bridge id)kSecAttrSynchronizable:@NO} mutableCopy];
}

@interface C3Enrollment : NSObject
@property NSURL *url;
@property NSString *pin;
@property id ca;
@property NSURLCredential *identity;
@end
@implementation C3Enrollment
@end

static C3Enrollment *enrollment(NSString *raw, NSString *slot) {
  NSData *data=[raw dataUsingEncoding:NSUTF8StringEncoding];
  if (![CLIENT_SLOTS containsObject:slot] || !data.length || data.length>16384) return nil;
  id candidate=[NSJSONSerialization JSONObjectWithData:data options:0 error:nullptr];
  if (![candidate isKindOfClass:NSDictionary.class]) return nil;
  NSDictionary *profile=candidate;
  NSDictionary *scope=profile[@"scope"];
  NSString *endpoint=profile[@"endpoint"];
  NSString *pin=profile[@"pin"];
  NSArray *certificates=profile[@"certificates"];
  if (![endpoint isKindOfClass:NSString.class] || ![pin isKindOfClass:NSString.class] || pin.length!=64 ||
      [pin rangeOfCharacterFromSet:[[NSCharacterSet characterSetWithCharactersInString:@"0123456789abcdef"] invertedSet]].location!=NSNotFound ||
      ![scope isKindOfClass:NSDictionary.class] || ![scope[@"role"] isEqual:[slot isEqual:@"travel"] ? @"private-user" : slot] ||
      ![scope[@"expiresAt"] isKindOfClass:NSNumber.class] ||
      [scope[@"expiresAt"] doubleValue] <= [NSDate.date timeIntervalSince1970] ||
      ![certificates isKindOfClass:NSArray.class] || certificates.count<1 || certificates.count>4) return nil;
  NSURLComponents *url=[NSURLComponents componentsWithString:endpoint];
  if (![url.scheme isEqual:@"https"] || !url.host.length || ![url.percentEncodedPath isEqual:[slot isEqual:@"travel"] ? @"/v1/private" : @"/v2/commerce"] ||
      url.user || url.password || url.query || url.fragment || !url.URL) return nil;
  id anchor=certificate(profile[@"ca"]);
  id leaf=certificate(certificates[0]);
  NSMutableArray *chain=[NSMutableArray array];
  for (NSUInteger index=1; index<certificates.count; ++index) {
    id crt=certificate(certificates[index]);
    if (!crt) return nil;
    [chain addObject:crt];
  }
  id key=CFBridgingRelease(privateKey(slot));
  if (!anchor || !leaf || !key) return nil;
  NSDictionary *keyAttributes=CFBridgingRelease(SecKeyCopyAttributes((__bridge SecKeyRef)key));
  if (![keyAttributes[(__bridge id)kSecAttrTokenID] isEqual:(__bridge id)kSecAttrTokenIDSecureEnclave]) return nil;
  id identity=CFBridgingRelease(SecIdentityCreate(nullptr,(__bridge SecCertificateRef)leaf,(__bridge SecKeyRef)key));
  if (!identity) return nil;
  C3Enrollment *result=[C3Enrollment new];
  result.url=url.URL;
  result.pin=pin;
  result.ca=anchor;
  result.identity=[NSURLCredential credentialWithIdentity:(__bridge SecIdentityRef)identity certificates:(chain.count ? chain : nil) persistence:NSURLCredentialPersistenceNone];
  return result;
}

// The chain must end at `ca` for `host`, and the leaf must carry the pinned key. System roots are never consulted.
static BOOL anchoredServer(SecTrustRef trust, NSString *host, id ca, NSString *pin) {
  if (!trust || !host.length || !ca || pin.length!=64) return NO;
  id policy=CFBridgingRelease(SecPolicyCreateSSL(true,(__bridge CFStringRef)host));
  BOOL anchored=SecTrustSetPolicies(trust,(__bridge CFTypeRef)policy)==errSecSuccess &&
      SecTrustSetAnchorCertificates(trust,(__bridge CFArrayRef)@[ca])==errSecSuccess &&
      SecTrustSetAnchorCertificatesOnly(trust,true)==errSecSuccess && SecTrustEvaluateWithError(trust,nullptr);
  id key=anchored ? CFBridgingRelease(SecTrustCopyKey(trust)) : nil;
  return [fingerprint(spki((__bridge SecKeyRef)key)) isEqual:pin];
}

@interface C3Exchange : NSObject <NSURLSessionDataDelegate>
@property C3Enrollment *profile;
@property NSMutableData *buffer;
@property RCTPromiseResolveBlock resolve;
@property RCTPromiseRejectBlock reject;
@property BOOL reportsOffline;
@property BOOL observedResponse;
@property BOOL observedChallenge;
@end
@implementation C3Exchange
- (void)URLSession:(NSURLSession *)session didReceiveChallenge:(NSURLAuthenticationChallenge *)challenge
 completionHandler:(void (^)(NSURLSessionAuthChallengeDisposition,NSURLCredential *))complete {
  self.observedChallenge=YES;
  NSURLProtectionSpace *peer=challenge.protectionSpace;
  if (![peer.host isEqual:self.profile.url.host] || challenge.previousFailureCount) {
    complete(NSURLSessionAuthChallengeCancelAuthenticationChallenge,nil);
  } else if ([peer.authenticationMethod isEqual:NSURLAuthenticationMethodClientCertificate]) {
    complete(NSURLSessionAuthChallengeUseCredential,self.profile.identity);
  } else if ([peer.authenticationMethod isEqual:NSURLAuthenticationMethodServerTrust]) {
    SecTrustRef trust=peer.serverTrust;
    if (!trust) { complete(NSURLSessionAuthChallengeCancelAuthenticationChallenge,nil); return; }
    if (anchoredServer(trust,peer.host,self.profile.ca,self.profile.pin)) complete(NSURLSessionAuthChallengeUseCredential,[NSURLCredential credentialForTrust:trust]);
    else complete(NSURLSessionAuthChallengeCancelAuthenticationChallenge,nil);
  } else {
    complete(NSURLSessionAuthChallengeCancelAuthenticationChallenge,nil);
  }
}
- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task
 didReceiveChallenge:(NSURLAuthenticationChallenge *)challenge
 completionHandler:(void (^)(NSURLSessionAuthChallengeDisposition,NSURLCredential *))complete {
  [self URLSession:session didReceiveChallenge:challenge completionHandler:complete];
}
- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task
 willPerformHTTPRedirection:(NSHTTPURLResponse *)response newRequest:(NSURLRequest *)request
 completionHandler:(void (^)(NSURLRequest *))complete { complete(nil); }
- (void)URLSession:(NSURLSession *)session dataTask:(NSURLSessionDataTask *)task didReceiveResponse:(NSURLResponse *)response
 completionHandler:(void (^)(NSURLSessionResponseDisposition))complete {
  self.observedResponse=YES;
  NSInteger code=[(NSHTTPURLResponse *)response statusCode];
  complete((code==200 || code==400 || code==409 || code==503) ? NSURLSessionResponseAllow : NSURLSessionResponseCancel);
}
- (void)URLSession:(NSURLSession *)session dataTask:(NSURLSessionDataTask *)task didReceiveData:(NSData *)data {
  if (self.buffer.length+data.length>32768) [task cancel]; else [self.buffer appendData:data];
}
- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task didCompleteWithError:(NSError *)error {
  NSString *body=error ? nil : [[NSString alloc] initWithData:self.buffer encoding:NSUTF8StringEncoding];
  BOOL disconnected=self.reportsOffline && !self.observedResponse && !self.observedChallenge && [error.domain isEqual:NSURLErrorDomain] &&
    (error.code==NSURLErrorNotConnectedToInternet || error.code==NSURLErrorCannotConnectToHost ||
     error.code==NSURLErrorCannotFindHost || error.code==NSURLErrorDNSLookupFailed);
  if (body.length) self.resolve(body);
  else self.reject(disconnected ? @"COMMERCE_NETWORK_OFFLINE" : @"COMMERCE_UNAVAILABLE",
                   @"The enrolled commerce connection is unavailable",nil);
  [session finishTasksAndInvalidate];
}
@end

#ifdef NU54_TRAVEL_RECORDER
// One request to the customer entrance, which asks for no client certificate. This delegate holds no identity, so a
// request for one ends the connection.
@interface C3Entrance : NSObject <NSURLSessionDataDelegate>
@property NSString *host;
@property NSString *pin;
@property id ca;
@property NSInteger status;
@property NSUInteger limit;
@property NSMutableData *buffer;
@property RCTPromiseResolveBlock resolve;
@property RCTPromiseRejectBlock reject;
@end
@implementation C3Entrance
- (void)URLSession:(NSURLSession *)session didReceiveChallenge:(NSURLAuthenticationChallenge *)challenge
 completionHandler:(void (^)(NSURLSessionAuthChallengeDisposition,NSURLCredential *))complete {
  NSURLProtectionSpace *peer=challenge.protectionSpace;
  if ([peer.host isEqual:self.host] && !challenge.previousFailureCount &&
      [peer.authenticationMethod isEqual:NSURLAuthenticationMethodServerTrust] &&
      anchoredServer(peer.serverTrust,peer.host,self.ca,self.pin)) {
    complete(NSURLSessionAuthChallengeUseCredential,[NSURLCredential credentialForTrust:peer.serverTrust]);
  } else {
    complete(NSURLSessionAuthChallengeCancelAuthenticationChallenge,nil);
  }
}
- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task
 didReceiveChallenge:(NSURLAuthenticationChallenge *)challenge
 completionHandler:(void (^)(NSURLSessionAuthChallengeDisposition,NSURLCredential *))complete {
  [self URLSession:session didReceiveChallenge:challenge completionHandler:complete];
}
- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task
 willPerformHTTPRedirection:(NSHTTPURLResponse *)response newRequest:(NSURLRequest *)request
 completionHandler:(void (^)(NSURLRequest *))complete { complete(nil); }
- (void)URLSession:(NSURLSession *)session dataTask:(NSURLSessionDataTask *)task didReceiveResponse:(NSURLResponse *)response
 completionHandler:(void (^)(NSURLSessionResponseDisposition))complete {
  NSInteger code=[response isKindOfClass:NSHTTPURLResponse.class] ? [(NSHTTPURLResponse *)response statusCode] : 0;
  // What the entrance answers with. A redirect or anything else is not an answer.
  BOOL answer=code==200 || code==400 || code==404 || code==409 || code==429 || code==503;
  if (answer) self.status=code;
  complete(answer ? NSURLSessionResponseAllow : NSURLSessionResponseCancel);
}
- (void)URLSession:(NSURLSession *)session dataTask:(NSURLSessionDataTask *)task didReceiveData:(NSData *)data {
  if (self.buffer.length+data.length>self.limit) [task cancel]; else [self.buffer appendData:data];
}
- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task didCompleteWithError:(NSError *)error {
  NSString *body=(error || !self.status) ? nil : [[NSString alloc] initWithData:self.buffer encoding:NSUTF8StringEncoding];
  NSData *reply=body ? [NSJSONSerialization dataWithJSONObject:@{@"status":@(self.status),@"body":body} options:0 error:nullptr] : nil;
  if (reply) self.resolve([[NSString alloc] initWithData:reply encoding:NSUTF8StringEncoding]);
  else self.reject(@"COMMERCE_UNAVAILABLE",@"The customer entrance is unavailable",nil);
  [session finishTasksAndInvalidate];
}
@end
#endif

@interface CommerceHttp : NSObject <NativeCommerceHttpSpec>
@end
@implementation CommerceHttp {
#ifdef NU54_TRAVEL_RECORDER
  AVAudioRecorder *_travelRecorder;
  NSURL *_travelURL;
#endif
  dispatch_queue_t _work;
}
RCT_EXPORT_MODULE(CommerceHttp)
- (instancetype)init {
  if ((self=[super init])) _work=dispatch_queue_create("nu54.c3",DISPATCH_QUEUE_SERIAL);
  return self;
}
- (std::shared_ptr<facebook::react::TurboModule>)getTurboModule:(const facebook::react::ObjCTurboModule::InitParams &)params {
  return std::make_shared<facebook::react::NativeCommerceHttpSpecJSI>(params);
}
- (void)unavailable:(RCTPromiseRejectBlock)reject { reject(@"COMMERCE_UNAVAILABLE",@"The enrolled commerce connection is unavailable",nil); }
- (NSString *)profile:(NSString *)slot {
  if (![CLIENT_SLOTS containsObject:slot]) return nil;
  NSMutableDictionary *query=profileQuery(slot);
  query[(__bridge id)kSecReturnData]=@YES;
  CFTypeRef saved=nullptr;
  OSStatus rc=SecItemCopyMatching((__bridge CFDictionaryRef)query,&saved);
  if (rc==errSecItemNotFound) return @"";
  NSData *data=CFBridgingRelease(saved);
  return rc==errSecSuccess ? [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] : nil;
}
- (void)randomBytes:(double)size resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  if (size<1 || size>64 || size!=(NSUInteger)size) { [self unavailable:reject]; return; }
  NSMutableData *data=[NSMutableData dataWithLength:(NSUInteger)size];
  if (SecRandomCopyBytes(kSecRandomDefault,data.length,data.mutableBytes)!=errSecSuccess) { [self unavailable:reject]; return; }
  resolve([data base64EncodedStringWithOptions:0]);
}
- (void)loadJournal:(NSString *)name resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    if (!journalName(name)) { [self unavailable:reject]; return; }
    NSMutableDictionary *query=profileQuery([@"journal:" stringByAppendingString:name]);
    query[(__bridge id)kSecReturnData]=@YES;
    CFTypeRef saved=nullptr;
    OSStatus rc=SecItemCopyMatching((__bridge CFDictionaryRef)query,&saved);
    if (rc==errSecItemNotFound) { resolve(@""); return; }
    NSData *data=CFBridgingRelease(saved);
    NSString *value=rc==errSecSuccess ? [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] : nil;
    if (value) resolve(value); else [self unavailable:reject];
  });
}
- (void)saveJournal:(NSString *)name value:(NSString *)value resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    NSData *data=[value dataUsingEncoding:NSUTF8StringEncoding];
    if (!journalName(name) || !data || data.length>400000) { [self unavailable:reject]; return; }
    NSMutableDictionary *query=profileQuery([@"journal:" stringByAppendingString:name]);
    NSDictionary *update=@{(__bridge id)kSecValueData:data};
    OSStatus rc=SecItemUpdate((__bridge CFDictionaryRef)query,(__bridge CFDictionaryRef)update);
    if (rc==errSecItemNotFound) {
      [query addEntriesFromDictionary:update];
      query[(__bridge id)kSecAttrAccessible]=(__bridge id)kSecAttrAccessibleWhenUnlockedThisDeviceOnly;
      rc=SecItemAdd((__bridge CFDictionaryRef)query,nullptr);
    }
    if (rc==errSecSuccess) resolve(nil); else [self unavailable:reject];
  });
}
- (void)publicIdentity:(NSString *)slot resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    id key=CFBridgingRelease(privateKey(slot));
    id pub=key ? CFBridgingRelease(SecKeyCopyPublicKey((__bridge SecKeyRef)key)) : nil;
    NSData *encoded=spki((__bridge SecKeyRef)pub);
    if (encoded) resolve([encoded base64EncodedStringWithOptions:0]); else [self unavailable:reject];
  });
}
- (void)configure:(NSString *)slot profile:(NSString *)raw resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    if (!enrollment(raw,slot)) { [self unavailable:reject]; return; }
    NSMutableDictionary *query=profileQuery(slot);
    NSDictionary *update=@{(__bridge id)kSecValueData:[raw dataUsingEncoding:NSUTF8StringEncoding]};
    OSStatus rc=SecItemUpdate((__bridge CFDictionaryRef)query,(__bridge CFDictionaryRef)update);
    if (rc==errSecItemNotFound) {
      [query addEntriesFromDictionary:update];
      query[(__bridge id)kSecAttrAccessible]=(__bridge id)kSecAttrAccessibleWhenUnlockedThisDeviceOnly;
      rc=SecItemAdd((__bridge CFDictionaryRef)query,nullptr);
    }
    if (rc==errSecSuccess) resolve(nil); else [self unavailable:reject];
  });
}
- (void)loadProfile:(NSString *)slot resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{ NSString *raw=[self profile:slot]; if (raw) resolve(raw); else [self unavailable:reject]; });
}
- (void)exchange:(NSString *)slot envelope:(NSString *)envelope resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  if ([slot isEqual:@"travel"]) { [self unavailable:reject]; return; }
  [self exchangeAt:slot method:@"POST" path:@"" envelope:envelope limit:8192 profileDigest:nil resolve:resolve reject:reject];
}

- (void)exchangeScoped:(NSString *)slot profileDigest:(NSString *)digest envelope:(NSString *)envelope
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  if ([slot isEqual:@"travel"] || digest.length!=64) { [self unavailable:reject]; return; }
  [self exchangeAt:slot method:@"POST" path:@"" envelope:envelope limit:8192 profileDigest:digest resolve:resolve reject:reject];
}

#ifndef NU54_TRAVEL_RECORDER
- (void)exchangeRenterV1Origin:(NSString *)operation profileDigest:(NSString *)digest body:(NSString *)body
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  if ((! [operation isEqual:@"reserve"] && ! [operation isEqual:@"confirm"]) || digest.length != 64) {
    [self unavailable:reject]; return;
  }
  [self exchangeAt:@"terminal" method:@"POST" path:[@"/v2/private/renter-v1-orders/" stringByAppendingString:operation]
    envelope:body limit:2048 profileDigest:digest resolve:resolve reject:reject];
}

- (void)exchangePrint:(NSString *)operation profileDigest:(NSString *)digest reference:(NSString *)reference body:(NSString *)body
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    NSString *raw=[self profile:@"operator"];
    NSDictionary *profile=raw ? [NSJSONSerialization JSONObjectWithData:[raw dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nullptr] : nil;
    NSString *branch=profile[@"scope"][@"branchRef"];
    NSString *pattern=@"^[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}$";
    BOOL queue=[operation isEqual:@"queue"];
    BOOL decision=[operation isEqual:@"decision"];
    if (!branch || digest.length!=64 ||
        [branch rangeOfString:pattern options:NSRegularExpressionSearch].location==NSNotFound ||
        (!(queue && !reference.length) &&
         [reference rangeOfString:pattern options:NSRegularExpressionSearch].location==NSNotFound)) {
      [self unavailable:reject]; return;
    }
    NSString *path;
    if (queue || [operation isEqual:@"attempts"]) {
      path=[@"/v2/fulfillment/print-attempts?branchId=" stringByAppendingString:branch];
      if (reference.length) path=[path stringByAppendingFormat:@"&%@=%@",queue ? @"saleRef" : @"receiptRef",reference];
    } else if (decision) {
      id input=[NSJSONSerialization JSONObjectWithData:[body dataUsingEncoding:NSUTF8StringEncoding] options:0 error:nullptr];
      if (![input isKindOfClass:NSDictionary.class] || ![input[@"branchId"] isEqual:branch] || ![input[@"attemptId"] isEqual:reference]) {
        [self unavailable:reject]; return;
      }
      path=@"/v2/fulfillment/print-decisions";
    } else {
      [self unavailable:reject]; return;
    }
    [self exchangeAt:@"operator" method:decision ? @"POST" : @"GET" path:path envelope:body
      limit:4096 profileDigest:digest resolve:resolve reject:reject];
  });
}
#endif

#ifdef NU54_TRAVEL_RECORDER
- (void)cleanTravelCapture {
  [_travelRecorder stop];
  _travelRecorder=nil;
  if (_travelURL) [NSFileManager.defaultManager removeItemAtURL:_travelURL error:nullptr];
  _travelURL=nil;
  [AVAudioSession.sharedInstance setActive:NO error:nullptr];
}
- (void)startTravelRecording:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  void (^permission)(BOOL)=^(BOOL allowed) {
    dispatch_async(self->_work,^{
      if (!allowed || self->_travelRecorder) { [self unavailable:reject]; return; }
      AVAudioSession *audio=AVAudioSession.sharedInstance;
      if (![audio setCategory:AVAudioSessionCategoryRecord error:nullptr] || ![audio setActive:YES error:nullptr]) {
        [self unavailable:reject]; return;
      }
      NSURL *directory=[NSURL fileURLWithPath:NSTemporaryDirectory() isDirectory:YES];
      for (NSURL *previous in [NSFileManager.defaultManager contentsOfDirectoryAtURL:directory
          includingPropertiesForKeys:nil options:0 error:nullptr]) {
        if ([previous.lastPathComponent hasPrefix:@"nu54-travel-"] && [previous.pathExtension isEqual:@"m4a"])
          [NSFileManager.defaultManager removeItemAtURL:previous error:nullptr];
      }
      NSString *name=[NSString stringWithFormat:@"nu54-travel-%@.m4a",NSUUID.UUID.UUIDString];
      self->_travelURL=[directory URLByAppendingPathComponent:name];
      self->_travelRecorder=[[AVAudioRecorder alloc] initWithURL:self->_travelURL settings:@{
        AVFormatIDKey:@(kAudioFormatMPEG4AAC),AVSampleRateKey:@16000,AVNumberOfChannelsKey:@1,AVEncoderBitRateKey:@32000
      } error:nullptr];
      if (!self->_travelRecorder || ![self->_travelRecorder recordForDuration:20]) {
        [self cleanTravelCapture]; [self unavailable:reject]; return;
      }
      [NSFileManager.defaultManager setAttributes:@{NSFileProtectionKey:NSFileProtectionComplete}
          ofItemAtPath:self->_travelURL.path error:nullptr];
      resolve(nil);
    });
  };
  if (@available(iOS 17.0,*)) [AVAudioApplication requestRecordPermissionWithCompletionHandler:permission];
  else [AVAudioSession.sharedInstance requestRecordPermission:permission];
}
- (void)finishTravelRecording:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    if (!self->_travelRecorder) { [self unavailable:reject]; return; }
    [self->_travelRecorder stop];
    NSData *sample=[NSData dataWithContentsOfURL:self->_travelURL options:NSDataReadingMappedIfSafe error:nullptr];
    NSString *result=(sample.length>0 && sample.length<=95000) ? [sample base64EncodedStringWithOptions:0] : nil;
    [self cleanTravelCapture];
    if (result) resolve(result); else [self unavailable:reject];
  });
}
- (void)discardTravelRecording:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{ [self cleanTravelCapture]; resolve(nil); });
}
#endif

#ifdef NU54_TRAVEL_RECORDER
- (void)exchangeRenterReturn:(NSString *)digest assignment:(NSString *)assignment account:(NSString *)account
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  NSString *uuid=@"^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$";
  if ([digest rangeOfString:@"^[a-f0-9]{64}$" options:NSRegularExpressionSearch].location==NSNotFound ||
      [assignment rangeOfString:uuid options:NSRegularExpressionSearch].location==NSNotFound ||
      (account.length && [account rangeOfString:@"^0x[a-f0-9]{40}$" options:NSRegularExpressionSearch].location==NSNotFound)) {
    [self unavailable:reject]; return;
  }
  NSString *path=[@"/v2/renter/returns/" stringByAppendingString:assignment];
  BOOL accept=account.length>0;
  path=[path stringByAppendingString:accept ? @"/accept" : @"?credential=1&slot=1"];
  NSString *wire=accept ? [NSString stringWithFormat:@"{\"account\":\"%@\"}",account] : @"";
  [self exchangeAt:@"customer" method:accept ? @"POST" : @"GET" path:path envelope:wire
      limit:1024 profileDigest:digest resolve:resolve reject:reject];
}
#endif

#ifdef NU54_TRAVEL_RECORDER
// The server is accepted only by the trust anchor of this build: `anchor` is `{"origin","ca","pin"}` from the app's
// deployment configuration. No enrollment profile is read, and there is no other trust to fall back on. `session` is
// the customer session token of /v2/customer/commerce and empty elsewhere; the store list, a store page, a menu page
// and a menu photo part are read with GET and an empty body.
- (void)customerRequest:(NSString *)anchor path:(NSString *)path session:(NSString *)session body:(NSString *)body
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    NSCharacterSet *notHex=[[NSCharacterSet characterSetWithCharactersInString:@"0123456789abcdef"] invertedSet];
    BOOL read=[path isEqual:@"/v2/customer/stores"] ||
        [path rangeOfString:@"^/v2/customer/stores/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}(/menu/(0|[1-9][0-9]{0,4})|/photos/[0-9a-f]{64}/(0|[1-9][0-9]{0,5}))?$" options:NSRegularExpressionSearch].location!=NSNotFound;
    BOOL commerce=[path isEqual:@"/v2/customer/commerce"];
    NSData *payload=[body dataUsingEncoding:NSUTF8StringEncoding];
    NSData *given=[anchor dataUsingEncoding:NSUTF8StringEncoding];
    id candidate=(given.length && given.length<=8192) ? [NSJSONSerialization JSONObjectWithData:given options:0 error:nullptr] : nil;
    NSDictionary *trust=[candidate isKindOfClass:NSDictionary.class] ? candidate : nil;
    NSString *origin=trust[@"origin"];
    NSURLComponents *url=[origin isKindOfClass:NSString.class] ? [NSURLComponents componentsWithString:origin] : nil;
    BOOL sized=read ? payload.length==0 : (payload.length>=2 && payload.length<=(commerce ? 8192 : 1024));
    BOOL held=commerce ? (session.length==64 && [session rangeOfCharacterFromSet:notHex].location==NSNotFound) : session.length==0;
    if (!(read || commerce || [@[@"/v2/customer/challenge",@"/v2/customer/proof"] containsObject:path]) || !sized || !held ||
        ![url.scheme isEqual:@"https"] || !url.host.length || url.user || url.password || url.query || url.fragment ||
        url.percentEncodedPath.length || (url.port && (url.port.integerValue<1 || url.port.integerValue>65535))) {
      [self unavailable:reject]; return;
    }
    NSString *pin=trust[@"pin"];
    id anchorCertificate=[trust[@"ca"] isKindOfClass:NSString.class] ? certificate(trust[@"ca"]) : nil;
    if (!anchorCertificate || ![pin isKindOfClass:NSString.class] || pin.length!=64 ||
        [pin rangeOfCharacterFromSet:notHex].location!=NSNotFound) {
      [self unavailable:reject]; return;
    }
    url.percentEncodedPath=path;
    if (!url.URL) { [self unavailable:reject]; return; }
    NSURLSessionConfiguration *config=NSURLSessionConfiguration.ephemeralSessionConfiguration;
    config.TLSMinimumSupportedProtocolVersion=tls_protocol_version_TLSv13;
    config.TLSMaximumSupportedProtocolVersion=tls_protocol_version_TLSv13;
    config.timeoutIntervalForResource=8;
    config.timeoutIntervalForRequest=8;
    config.HTTPShouldSetCookies=NO;
    config.HTTPCookieStorage=nil;
    config.URLCredentialStorage=nil;
    config.URLCache=nil;
    config.requestCachePolicy=NSURLRequestReloadIgnoringLocalCacheData;
    C3Entrance *delegate=[C3Entrance new];
    delegate.host=url.host; delegate.pin=pin; delegate.ca=anchorCertificate;
    delegate.limit=(read || commerce) ? 32768 : 8192;
    delegate.resolve=resolve; delegate.reject=reject; delegate.buffer=[NSMutableData data];
    NSURLSession *connection=[NSURLSession sessionWithConfiguration:config delegate:delegate delegateQueue:nil];
    NSMutableURLRequest *request=[NSMutableURLRequest requestWithURL:url.URL];
    request.HTTPMethod=read ? @"GET" : @"POST";
    if (!read) request.HTTPBody=payload;
    [request setValue:@"application/json" forHTTPHeaderField:@"Content-Type"];
    [request setValue:@"no-store" forHTTPHeaderField:@"Cache-Control"];
    if (commerce) [request setValue:[@"Bearer " stringByAppendingString:session] forHTTPHeaderField:@"Authorization"];
    [[connection dataTaskWithRequest:request] resume];
  });
}
#endif

- (void)exchangeTravel:(NSString *)method path:(NSString *)path envelope:(NSString *)envelope
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  NSRange match=[path rangeOfString:@"^/(consents|records(/[a-f0-9-]{36})?|visits|reviews|transcriptions|erasure(/[a-f0-9-]{36})?|itineraries/(preview|apply|generate|[a-f0-9-]{36}))$"
       options:NSRegularExpressionSearch];
  if (match.location==NSNotFound || ![@[@"GET",@"POST"] containsObject:method]) {
    [self unavailable:reject]; return;
  }
  [self exchangeAt:@"travel" method:method path:path envelope:envelope limit:1400000 profileDigest:nil resolve:resolve reject:reject];
}

- (void)exchangeAt:(NSString *)slot method:(NSString *)method path:(NSString *)path envelope:(NSString *)envelope limit:(NSUInteger)limit profileDigest:(NSString *)digest
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_work,^{
    NSData *body=[envelope dataUsingEncoding:NSUTF8StringEncoding];
    NSString *raw=[self profile:slot];
    if (digest && (!raw || ![digest isEqual:fingerprint([raw dataUsingEncoding:NSUTF8StringEncoding])])) {
      [self unavailable:reject]; return;
    }
    C3Enrollment *profile=enrollment(raw,slot);
    BOOL post=[method isEqual:@"POST"];
    if (!profile || (post ? (body.length<2 || body.length>limit) : body.length!=0)) { [self unavailable:reject]; return; }
    NSURLSessionConfiguration *config=NSURLSessionConfiguration.ephemeralSessionConfiguration;
    config.TLSMinimumSupportedProtocolVersion=tls_protocol_version_TLSv13;
    config.TLSMaximumSupportedProtocolVersion=tls_protocol_version_TLSv13;
    config.timeoutIntervalForResource=[path isEqual:@"/itineraries/generate"] ? 70 : 8;
    config.timeoutIntervalForRequest=config.timeoutIntervalForResource;
    config.HTTPShouldSetCookies=NO;
    config.URLCredentialStorage=nil;
    C3Exchange *delegate=[C3Exchange new];
    delegate.reportsOffline=digest && [slot isEqual:@"terminal"];
    if (digest && [slot isEqual:@"customer"] && post && !path.length) {
      id wire=[NSJSONSerialization JSONObjectWithData:body options:0 error:nullptr];
      id command=[wire isKindOfClass:NSDictionary.class] ? wire[@"command"] : nil;
      delegate.reportsOffline=[command isKindOfClass:NSDictionary.class] &&
          [command[@"contract"] isEqual:@"C3"] && [command[@"operation"] isEqual:@"catalog.read"];
    }
    delegate.profile=profile; delegate.resolve=resolve; delegate.reject=reject; delegate.buffer=[NSMutableData data];
    NSURLSession *session=[NSURLSession sessionWithConfiguration:config delegate:delegate delegateQueue:nil];
    NSURL *url=([path hasPrefix:@"/v2/fulfillment/"] || [path hasPrefix:@"/v2/renter/returns/"] || [path hasPrefix:@"/v2/private/renter-v1-orders/"])
      ? [[NSURL URLWithString:path relativeToURL:profile.url] absoluteURL]
      : [NSURL URLWithString:[profile.url.absoluteString stringByAppendingString:path]];
    NSMutableURLRequest *request=[NSMutableURLRequest requestWithURL:url];
    request.HTTPMethod=method;
    if (post) request.HTTPBody=body;
    [request setValue:@"application/json" forHTTPHeaderField:@"Content-Type"];
    [request setValue:@"no-store" forHTTPHeaderField:@"Cache-Control"];
    [[session dataTaskWithRequest:request] resume];
  });
}
@end
