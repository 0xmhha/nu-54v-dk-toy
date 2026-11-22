#import <Foundation/Foundation.h>
#import <math.h>
#import <time.h>
#import <Security/Security.h>
#import <TargetConditionals.h>
#import <NU54RenterSpec/NU54RenterSpec.h>
#import "../../native/market/wallet.h"

namespace {
constexpr size_t recordSize = LAB_SCOPE_LENGTH + LAB_KEY_LENGTH;

bool validOrigin(NSString *candidate) {
  if (candidate.length > 260) return false;
  NSURLComponents *url = [NSURLComponents componentsWithString:candidate];
  NSString *hostPattern = @"\\A[a-z0-9][a-z0-9.-]*[.][a-z][a-z0-9-]*\\z";
  NSNumber *port = url.port;
  if (!url.host || (port && (port.intValue < 1 || port.intValue > 65535)) ||
      [url.host rangeOfString:hostPattern options:NSRegularExpressionSearch].location == NSNotFound) return false;
  NSString *authority = port ? [NSString stringWithFormat:@"%@:%@", url.host, port] : url.host;
  return [candidate isEqualToString:[@"https://" stringByAppendingString:authority]];
}

NSString *withoutDefaultPort(NSString *origin) {
  return [origin hasSuffix:@":443"] ? [origin substringToIndex:origin.length - 4] : origin;
}

bool decode(NSString *value, uint8_t *out, size_t length) {
  return lab_decode(value.UTF8String, value.length, out, length);
}

NSString *encode(const uint8_t *value, size_t length) {
  char encoded[2 * LAB_TX_LENGTH + 3] = {'0', 'x'};
  lab_encode(value, length, encoded + 2);
  return [NSString stringWithUTF8String:encoded];
}

NSMutableDictionary *keyQuery() {
  return [@{(__bridge id)kSecClass:(__bridge id)kSecClassGenericPassword,
            (__bridge id)kSecAttrService:@"com.nu54.market-lab",
            (__bridge id)kSecAttrAccount:@"practice-8283-v1",
            (__bridge id)kSecAttrSynchronizable:@NO,
            (__bridge id)kSecUseDataProtectionKeychain:@YES} mutableCopy];
}

bool load(uint8_t record[recordSize], const uint8_t scope[LAB_SCOPE_LENGTH], bool create) {
  NSMutableDictionary *query = keyQuery();
  query[(__bridge id)kSecReturnData] = @YES;
  CFTypeRef result = nullptr;
  OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &result);
  NSData *contents = CFBridgingRelease(result);
  if (status == errSecSuccess && contents.length == recordSize) {
    memcpy(record, contents.bytes, recordSize);
    return memcmp(record, scope, LAB_SCOPE_LENGTH) == 0;
  }
  if (status != errSecItemNotFound || !create ||
      SecRandomCopyBytes(kSecRandomDefault, LAB_KEY_LENGTH, record + LAB_SCOPE_LENGTH) != errSecSuccess) return false;
  memcpy(record, scope, LAB_SCOPE_LENGTH);
  query = keyQuery();
  query[(__bridge id)kSecAttrAccessible] = (__bridge id)kSecAttrAccessibleWhenUnlockedThisDeviceOnly;
  query[(__bridge id)kSecValueData] = [NSData dataWithBytesNoCopy:record length:recordSize freeWhenDone:NO];
  return SecItemAdd((__bridge CFDictionaryRef)query, nullptr) == errSecSuccess;
}

void refused(RCTPromiseRejectBlock reject) {
  reject(@"MARKET_LAB_UNAVAILABLE", @"The isolated market wallet or connection is unavailable.", nil);
}
}

@interface MarketLab : NativeMarketLabSpecBase <NativeMarketLabSpec, NSURLSessionTaskDelegate>
@end

@implementation MarketLab {
  dispatch_queue_t _queue;
  NSURLSession *_session;
  NSString *_origin;
  NSString *_policy;
  NSString *_operatorOrigin;
  id _anchor;
}

RCT_EXPORT_MODULE(MarketLab)

- (instancetype)init {
  if ((self = [super init])) {
    _queue = dispatch_queue_create("com.nu54.market-lab", DISPATCH_QUEUE_SERIAL);
    NSString *setting = [NSBundle.mainBundle objectForInfoDictionaryKey:@"NU54MarketProfile"];
    if ([setting isKindOfClass:NSString.class]) {
#if TARGET_OS_SIMULATOR
      if ([setting isEqualToString:@"loopback"]) {
        _origin = @"http://127.0.0.1:8284";
        _policy = @"loopback";
      }
#endif
      NSArray<NSString *> *parts = [setting componentsSeparatedByString:@"|"];
      if (parts.count == 3 || parts.count == 4) {
        NSString *candidate = parts[0];
        NSData *der = parts[2].length <= 6000 ? [[NSData alloc] initWithBase64EncodedString:parts[2] options:0] : nil;
        if (der.length && validOrigin(candidate) &&
            [parts[1] rangeOfString:@"\\A0x[0-9a-f]{64}\\z" options:NSRegularExpressionSearch].location != NSNotFound) {
          _anchor = CFBridgingRelease(SecCertificateCreateWithData(kCFAllocatorDefault, (__bridge CFDataRef)der));
          if (_anchor) {
            _origin = candidate;
            _policy = parts[1];
            if (parts.count == 4 && validOrigin(parts[3]) &&
                ![withoutDefaultPort(parts[3]) isEqualToString:withoutDefaultPort(candidate)]) {
              _operatorOrigin = parts[3];
            }
          }
        }
      }
    }
    NSURLSessionConfiguration *config = NSURLSessionConfiguration.ephemeralSessionConfiguration;
    config.timeoutIntervalForRequest = 10;
    config.timeoutIntervalForResource = 12;
    config.connectionProxyDictionary = @{};
    config.TLSMinimumSupportedProtocolVersion = tls_protocol_version_TLSv12;
    config.HTTPCookieStorage = nil;
    config.URLCredentialStorage = nil;
    _session = [NSURLSession sessionWithConfiguration:config delegate:self delegateQueue:nil];
  }
  return self;
}

- (std::shared_ptr<facebook::react::TurboModule>)getTurboModule:
    (const facebook::react::ObjCTurboModule::InitParams &)params {
  return std::make_shared<facebook::react::NativeMarketLabSpecJSI>(params);
}

- (void)invalidate {
  [_session invalidateAndCancel];
}

- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task
    willPerformHTTPRedirection:(NSHTTPURLResponse *)response newRequest:(NSURLRequest *)request
    completionHandler:(void (^)(NSURLRequest *_Nullable))completionHandler {
  completionHandler(nil);
}

- (void)URLSession:(NSURLSession *)session didReceiveChallenge:(NSURLAuthenticationChallenge *)challenge
    completionHandler:(void (^)(NSURLSessionAuthChallengeDisposition, NSURLCredential *))completionHandler {
  SecTrustRef trust = challenge.protectionSpace.serverTrust;
  NSString *host = [NSURL URLWithString:_origin ?: @""].host;
  if (_anchor && trust && [challenge.protectionSpace.host isEqualToString:host] &&
      [challenge.protectionSpace.authenticationMethod isEqualToString:NSURLAuthenticationMethodServerTrust] &&
      SecTrustSetAnchorCertificates(trust, (__bridge CFArrayRef)@[_anchor]) == errSecSuccess &&
      SecTrustSetAnchorCertificatesOnly(trust, true) == errSecSuccess &&
      SecTrustEvaluateWithError(trust, nullptr)) {
    completionHandler(NSURLSessionAuthChallengeUseCredential, [NSURLCredential credentialForTrust:trust]);
  } else {
    completionHandler(NSURLSessionAuthChallengeCancelAuthenticationChallenge, nil);
  }
}

- (void)walletAddress:(NSString *)scope create:(BOOL)create
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_queue, ^{
    uint8_t context[LAB_SCOPE_LENGTH], record[recordSize], entropy[32], address[20];
    bool valid = decode(scope, context, sizeof context) && load(record, context, create) &&
        SecRandomCopyBytes(kSecRandomDefault, sizeof entropy, entropy) == errSecSuccess &&
        lab_address(record + LAB_SCOPE_LENGTH, entropy, address);
    lab_wipe(record, sizeof record);
    lab_wipe(entropy, sizeof entropy);
    if (valid) resolve(encode(address, sizeof address)); else refused(reject);
  });
}

- (void)deleteWallet:(NSString *)scope resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(_queue, ^{
    uint8_t context[LAB_SCOPE_LENGTH], record[recordSize];
    bool valid = decode(scope, context, sizeof context) && load(record, context, false);
    lab_wipe(record, sizeof record);
    if (valid && SecItemDelete((__bridge CFDictionaryRef)keyQuery()) == errSecSuccess) resolve(@YES);
    else refused(reject);
  });
}

- (void)sign:(NSString *)scope target:(NSString *)target nonce:(double)nonce gasPrice:(double)gasPrice
    data:(NSString *)data quote:(NSString *)quote
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  if (!(nonce >= 0 && nonce <= 9007199254740991.0 && nonce == floor(nonce)) ||
      !(gasPrice > 0 && gasPrice <= 100000000000.0 && gasPrice == floor(gasPrice))) {
    refused(reject);
    return;
  }
  dispatch_async(_queue, ^{
    uint8_t context[LAB_SCOPE_LENGTH], receiver[20], record[recordSize], entropy[32];
    uint8_t calldata[LAB_QUOTE_LENGTH], offer[LAB_QUOTE_LENGTH], raw[LAB_TX_LENGTH];
    size_t dataLength = data.length / 2, rawLength = 0;
    time_t now = time(nullptr);
    bool valid = now > 0 && (dataLength == 68 || dataLength == LAB_BOUND_APPROVE_LENGTH || dataLength == LAB_QUOTE_LENGTH) &&
        decode(scope, context, sizeof context) && decode(target, receiver, sizeof receiver) &&
        decode(data, calldata, dataLength) && decode(quote, offer, sizeof offer) &&
        load(record, context, false) && SecRandomCopyBytes(kSecRandomDefault, sizeof entropy, entropy) == errSecSuccess &&
        lab_sign(record + LAB_SCOPE_LENGTH, entropy, context, receiver, (uint64_t)nonce, (uint64_t)gasPrice, (uint64_t)now,
            calldata, dataLength, offer, raw, &rawLength);
    lab_wipe(record, sizeof record);
    lab_wipe(entropy, sizeof entropy);
    if (valid) resolve(encode(raw, rawLength)); else refused(reject);
  });
}

- (void)deploymentPolicy:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  resolve(_policy ?: @"");
}

- (void)operatorOrigin:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  resolve(_operatorOrigin ?: @"");
}

- (void)exchange:(NSString *)path json:(NSString *)json
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  NSString *pattern = @"^/v1/(profile|wallet/0x[0-9a-fA-F]{40}|positions/[0-9]{1,78}(/history)?|"
      @"executions/0x[0-9a-fA-F]{64}/0x[0-9a-fA-F]{64}|approvals/0x[0-9a-fA-F]{64}/0x[0-9a-fA-F]{40}|quotes|transactions)\\z";
  BOOL writes = [path isEqualToString:@"/v1/quotes"] || [path isEqualToString:@"/v1/transactions"];
  if (!_origin || path.length > 160 || [path rangeOfString:pattern options:NSRegularExpressionSearch].location == NSNotFound ||
      writes != (json.length > 0) || json.length > 8192) {
    refused(reject);
    return;
  }
  NSURL *url = [NSURL URLWithString:[_origin stringByAppendingString:path]];
  NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:url];
  if (writes) {
    request.HTTPMethod = @"POST";
    request.HTTPBody = [json dataUsingEncoding:NSUTF8StringEncoding];
    [request setValue:@"application/json" forHTTPHeaderField:@"Content-Type"];
  }
  [[_session dataTaskWithRequest:request completionHandler:^(NSData *body, NSURLResponse *response, NSError *error) {
    NSHTTPURLResponse *http = (NSHTTPURLResponse *)response;
    NSString *content = [http valueForHTTPHeaderField:@"Content-Type"];
    NSString *text = !error && http.statusCode >= 200 && http.statusCode < 300 && body.length <= 32768 &&
        [content hasPrefix:@"application/json"] ? [[NSString alloc] initWithData:body encoding:NSUTF8StringEncoding] : nil;
    if (text) resolve(text); else refused(reject);
  }] resume];
}
@end
