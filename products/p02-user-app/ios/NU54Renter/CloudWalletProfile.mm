#import <Foundation/Foundation.h>
#import <TargetConditionals.h>
#import <Security/Security.h>
#include <atomic>
#import <CommonCrypto/CommonDigest.h>
#import <NU54RenterSpec/NU54RenterSpec.h>
#import <React/RCTInvalidating.h>

#if NU54_P03_NATIVE_LINKED
extern "C" int nu54_p03_start(char *, void *, int, char *, char *, long long);
extern "C" void nu54_p03_stop(void);
extern "C" int nu54_p03_prepare(char *, void *, int, char *, int);
extern "C" int nu54_p03_install(char *, void *, int, char *, char *, char *);
#else
static int nu54_p03_start(char *, void *, int, char *, char *, long long) { return -1; }
static void nu54_p03_stop(void) {}
static int nu54_p03_prepare(char *, void *, int, char *, int) { return -1; }
static int nu54_p03_install(char *, void *, int, char *, char *, char *) { return -1; }
#endif

@interface CloudWalletProfile : NativeCloudWalletProfileSpecBase <NativeCloudWalletProfileSpec, NSURLSessionTaskDelegate, RCTInvalidating>
@end

static std::atomic<unsigned long long> p03Generation{0};

static dispatch_queue_t p03Queue() {
  static dispatch_queue_t queue;
  static dispatch_once_t once;
  dispatch_once(&once, ^{queue=dispatch_queue_create("nu54.p03.participant",DISPATCH_QUEUE_SERIAL);});
  return queue;
}


@implementation CloudWalletProfile
RCT_EXPORT_MODULE(CloudWalletProfile)

- (std::shared_ptr<facebook::react::TurboModule>)getTurboModule:
    (const facebook::react::ObjCTurboModule::InitParams &)params {
  return std::make_shared<facebook::react::NativeCloudWalletProfileSpecJSI>(params);
}

- (NSString *)profileText {
  NSString *encoded = [NSBundle.mainBundle objectForInfoDictionaryKey:@"NU54P03Profile"];
  if (![encoded isKindOfClass:NSString.class] || encoded.length == 0) return @"";
  NSData *bytes = [[NSData alloc] initWithBase64EncodedString:encoded options:0];
  if (!bytes || bytes.length > 4096) return nil;
  return [[NSString alloc] initWithData:bytes encoding:NSUTF8StringEncoding];
}

- (void)profile:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  NSString *raw = [self profileText];
  if (raw) resolve(raw);
  else reject(@"P03_PROFILE", @"Cloud wallet profile is unavailable", nil);
}

- (void)withParticipant:(NSString *)subject resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject
    work:(id (^)(NSURL *, NSMutableData *, NSString *))work {
  dispatch_async(p03Queue(), ^{
    NSURL *support = [NSFileManager.defaultManager URLsForDirectory:NSApplicationSupportDirectory
      inDomains:NSUserDomainMask].firstObject;
    NSData *profile = [[self profileText] dataUsingEncoding:NSUTF8StringEncoding];
    id parsed = profile ? [NSJSONSerialization JSONObjectWithData:profile options:0 error:nullptr] : nil;
    NSString *authority = [parsed isKindOfClass:NSDictionary.class] ? parsed[@"rosterAuthority"] : nil;
    if (subject.length!=36 || ![parsed isKindOfClass:NSDictionary.class]) {
      reject(@"P03_PARTICIPANT", @"Local cloud wallet participant is unavailable", nil);return;
    }
    NSData *identity=[[NSString stringWithFormat:@"%@|%@|%@",parsed[@"api"],parsed[@"identity"],subject]
      dataUsingEncoding:NSUTF8StringEncoding];
    unsigned char digest[CC_SHA256_DIGEST_LENGTH];
    CC_SHA256(identity.bytes,(CC_LONG)identity.length,digest);
    NSMutableString *scope=[NSMutableString stringWithCapacity:64];
    for (int index=0;index<CC_SHA256_DIGEST_LENGTH;index++) [scope appendFormat:@"%02x",digest[index]];
    NSURL *directory=[support URLByAppendingPathComponent:[@"cloud-p03-participant-" stringByAppendingString:scope] isDirectory:YES];
    if (![authority isKindOfClass:NSString.class] ||
        ![NSFileManager.defaultManager createDirectoryAtURL:directory withIntermediateDirectories:YES
          attributes:@{NSFilePosixPermissions:@0700,NSFileProtectionKey:NSFileProtectionCompleteUntilFirstUserAuthentication}
          error:nullptr] ||
        ![directory setResourceValue:@YES forKey:NSURLIsExcludedFromBackupKey error:nullptr]) {
      reject(@"P03_PARTICIPANT", @"Local cloud wallet participant is unavailable", nil);
      return;
    }
    NSMutableDictionary *query = [@{
      (__bridge id)kSecClass:(__bridge id)kSecClassGenericPassword,
      (__bridge id)kSecAttrService:@"nu54-p03-vault",
      (__bridge id)kSecAttrAccount:scope,
      (__bridge id)kSecAttrSynchronizable:@NO,
      (__bridge id)kSecUseDataProtectionKeychain:@YES,
    } mutableCopy];
    CFTypeRef found = nullptr;
    query[(__bridge id)kSecReturnData] = @YES;
    OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &found);
    NSMutableData *key = status==errSecSuccess ? [CFBridgingRelease(found) mutableCopy] : nil;
    if (status==errSecItemNotFound &&
        ![NSFileManager.defaultManager fileExistsAtPath:[directory URLByAppendingPathComponent:@"vault"].path] &&
        ![NSFileManager.defaultManager fileExistsAtPath:[directory URLByAppendingPathComponent:@"device-tls.sealed"].path]) {
      key = [NSMutableData dataWithLength:32];
      if (SecRandomCopyBytes(kSecRandomDefault, key.length, key.mutableBytes)==errSecSuccess) {
        [query removeObjectForKey:(__bridge id)kSecReturnData];
        query[(__bridge id)kSecValueData] = key;
        query[(__bridge id)kSecAttrAccessible] = (__bridge id)kSecAttrAccessibleWhenUnlockedThisDeviceOnly;
        status = SecItemAdd((__bridge CFDictionaryRef)query, nullptr);
      }
    }
    id result = status==errSecSuccess && key.length==32 ? work(directory,key,authority) : nil;
    if (key.length) [key resetBytesInRange:NSMakeRange(0, key.length)];
    if (result) resolve(result);
#if TARGET_OS_SIMULATOR && NU54_P03_SIMULATOR_LAB
    else reject([NSString stringWithFormat:@"P03_FIXTURE_KEYCHAIN_%d",(int)status], @"Local cloud wallet participant is unavailable", nil);
#else
    else reject(@"P03_PARTICIPANT", @"Local cloud wallet participant is unavailable", nil);
#endif
  });
}

- (void)startParticipant:(NSString *)subject expires:(double)expires token:(NSString *)token
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  unsigned long long generation=p03Generation.load();
  NSData *profileData=[[self profileText] dataUsingEncoding:NSUTF8StringEncoding];
  id parsed=profileData ? [NSJSONSerialization JSONObjectWithData:profileData options:0 error:nullptr] : nil;
  NSString *origin=[parsed isKindOfClass:NSDictionary.class] ? parsed[@"api"] : nil;
  if (subject.length!=36 || ![origin isKindOfClass:NSString.class]) {resolve(@NO);return;}
  [self request:[origin stringByAppendingString:@"/v1/session"] method:@"GET" body:@"" token:token resolve:^(id raw){
    NSData *responseData=[raw isKindOfClass:NSString.class] ? [raw dataUsingEncoding:NSUTF8StringEncoding] : nil;
    NSDictionary *response=responseData ? [NSJSONSerialization JSONObjectWithData:responseData options:0 error:nullptr] : nil;
    NSData *body=[response[@"body"] isKindOfClass:NSString.class] ? [response[@"body"] dataUsingEncoding:NSUTF8StringEncoding] : nil;
    NSDictionary *lease=body ? [NSJSONSerialization JSONObjectWithData:body options:0 error:nullptr] : nil;
    if ([response[@"status"] integerValue]!=200 || (lease[@"linked"] && ![lease[@"linked"] boolValue]) || ![lease[@"subject"] isEqual:subject] ||
        [lease[@"expires"] longLongValue]!=(long long)expires) {resolve(@NO);return;}
    [self withParticipant:subject resolve:resolve reject:reject work:^id(NSURL *directory, NSMutableData *key, NSString *authority) {
      if (generation!=p03Generation.load()) return @NO;
      return @(nu54_p03_start((char *)directory.fileSystemRepresentation,key.mutableBytes,32,
          (char *)subject.UTF8String,(char *)authority.UTF8String,(long long)expires)==0);
    }];
  } reject:reject];
}

- (void)stopParticipant:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  p03Generation.fetch_add(1);
  dispatch_async(p03Queue(), ^{nu54_p03_stop();resolve(@YES);});
}

- (void)invalidate {
  p03Generation.fetch_add(1);
  dispatch_async(p03Queue(), ^{nu54_p03_stop();});
}

- (void)prepareParticipant:(NSString *)subject resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  [self withParticipant:subject resolve:resolve reject:reject work:^id(NSURL *directory, NSMutableData *key, NSString *authority) {
    char csr[4096];
    int size = nu54_p03_prepare((char *)directory.fileSystemRepresentation,key.mutableBytes,32,csr,sizeof(csr));
    return size>0 ? [[NSString alloc] initWithBytes:csr length:size encoding:NSUTF8StringEncoding] : nil;
  }];
}

- (void)installParticipant:(NSString *)subject roster:(NSString *)roster
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  [self withParticipant:subject resolve:resolve reject:reject work:^id(NSURL *directory, NSMutableData *key, NSString *authority) {
    if (subject.length!=36 || roster.length>65536) return @NO;
    return @(nu54_p03_install((char *)directory.fileSystemRepresentation, key.mutableBytes,32,
      (char *)subject.UTF8String,(char *)authority.UTF8String,(char *)roster.UTF8String)==0);
  }];
}

- (void)request:(NSString *)endpoint method:(NSString *)method body:(NSString *)body token:(NSString *)token
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  NSData *profileBytes = [[self profileText] dataUsingEncoding:NSUTF8StringEncoding];
  id parsed = profileBytes ? [NSJSONSerialization JSONObjectWithData:profileBytes options:0 error:nullptr] : nil;
  NSDictionary *profile = [parsed isKindOfClass:NSDictionary.class] ? parsed : @{};
  NSURLComponents *parts = [NSURLComponents componentsWithString:endpoint];
  NSString *origin = [NSString stringWithFormat:@"https://%@%@", parts.host,
      parts.port ? [@":" stringByAppendingString:parts.port.stringValue] : @""];
  BOOL api = [origin isEqual:profile[@"api"]];
  BOOL validToken = token.length > 0 && [token rangeOfString:@"^[A-Za-z0-9_.-]+$" options:NSRegularExpressionSearch].location != NSNotFound;
  BOOL gateway = [origin isEqual:profile[@"identity"]] || [origin isEqual:profile[@"approval"]] || [origin isEqual:profile[@"recovery"]];
  BOOL onboarding = api && [parts.path isEqual:@"/v1/onboarding"] &&
    [profile[@"onboarding"] isKindOfClass:NSDictionary.class] && [method isEqual:@"GET"] &&
    !body.length && !token.length;
  BOOL apiPath = [parts.path isEqual:@"/v1/onboarding"] ? onboarding : [parts.path hasPrefix:@"/v1/"] && validToken;
  BOOL devicePath = [@[@"/p03/device/start", @"/p03/device/poll"] containsObject:parts.path ?: @""];
  NSData *payload = [body dataUsingEncoding:NSUTF8StringEncoding];
  if (!parts.URL || ![parts.scheme isEqual:@"https"] || !parts.host || parts.user || parts.password || parts.fragment || parts.query ||
      ![@[@"GET", @"POST", @"PUT", @"DELETE"] containsObject:method] || payload.length > 8192 || token.length > 8192 ||
      (api ? !apiPath : !gateway || !devicePath || ![method isEqual:@"POST"] || token.length > 0) ||
      (payload.length > 0 && ![@[@"POST", @"PUT"] containsObject:method])) {
    reject(@"P03_NETWORK", @"Cloud wallet request is unavailable", nil);
    return;
  }
  NSMutableURLRequest *request = [NSMutableURLRequest requestWithURL:parts.URL];
  request.HTTPMethod = method;
  request.HTTPBody = payload.length ? payload : nil;
  [request setValue:@"application/json" forHTTPHeaderField:@"Content-Type"];
  [request setValue:@"application/json" forHTTPHeaderField:@"Accept"];
  if (api && token.length) [request setValue:[@"Bearer " stringByAppendingString:token] forHTTPHeaderField:@"Authorization"];
  NSURLSessionConfiguration *config = NSURLSessionConfiguration.ephemeralSessionConfiguration;
  config.timeoutIntervalForRequest = 8;
  config.timeoutIntervalForResource = 8;
  config.HTTPCookieStorage = nil;
  config.URLCredentialStorage = nil;
  config.TLSMinimumSupportedProtocolVersion = tls_protocol_version_TLSv13;
  NSURLSession *session = [NSURLSession sessionWithConfiguration:config delegate:self delegateQueue:nil];
  [[session dataTaskWithRequest:request completionHandler:^(NSData *data, NSURLResponse *response, NSError *error) {
    NSHTTPURLResponse *http = [response isKindOfClass:NSHTTPURLResponse.class] ? (NSHTTPURLResponse *)response : nil;
    if (error || !http || data.length > 16384) {
      reject(@"P03_NETWORK", @"Cloud wallet service is unavailable", nil);
    } else {
      NSString *text = [[NSString alloc] initWithData:data ?: NSData.data encoding:NSUTF8StringEncoding] ?: @"";
      NSData *encoded = [NSJSONSerialization dataWithJSONObject:@{@"status": @(http.statusCode), @"body": text} options:0 error:nullptr];
      resolve([[NSString alloc] initWithData:encoded encoding:NSUTF8StringEncoding]);
    }
    [session finishTasksAndInvalidate];
  }] resume];
}

- (void)URLSession:(NSURLSession *)session task:(NSURLSessionTask *)task
    willPerformHTTPRedirection:(NSHTTPURLResponse *)response newRequest:(NSURLRequest *)request
    completionHandler:(void (^)(NSURLRequest *))completionHandler {
  completionHandler(nil);
}
@end

#if TARGET_OS_SIMULATOR && NU54_P03_SIMULATOR_LAB && NU54_P03_NATIVE_LINKED
@interface CloudWalletProfile (SimulatorLab)
- (void)runSimulatorLab:(NSString *)mode subject:(NSString *)subject roster:(NSString *)roster;
@end

@implementation CloudWalletProfile (SimulatorLab)

- (void)runSimulatorLab:(NSString *)mode subject:(NSString *)subject roster:(NSString *)roster {
  if (subject.length!=36 ||
      [subject rangeOfString:@"^[a-f0-9-]{36}$" options:NSRegularExpressionSearch].location==NSNotFound) return;
  NSURL *home=[NSFileManager.defaultManager URLsForDirectory:NSDocumentDirectory inDomains:NSUserDomainMask].firstObject;
  NSURL *(^path)(NSString *)=^NSURL *(NSString *suffix){
    return [home URLByAppendingPathComponent:[NSString stringWithFormat:@"p03-%@.%@",subject,suffix]];
  };
  RCTPromiseRejectBlock failure=^(NSString *code,NSString *message,NSError *error){
    [((code && [code hasPrefix:@"P03_FIXTURE_"]) ? [@"failed " stringByAppendingString:code] : @"failed") writeToURL:path(@"status") atomically:YES encoding:NSUTF8StringEncoding error:nullptr];
  };
  if ([mode isEqual:@"prepare"]) {
    [self prepareParticipant:subject resolve:^(id csr){
      [csr writeToURL:path(@"csr") atomically:YES encoding:NSUTF8StringEncoding error:nullptr];
    } reject:failure];
    return;
  }
  if (![mode isEqual:@"serve"]) return;
  void (^launch)(void)=^{
    [self withParticipant:subject resolve:^(id ready){
      if (![ready isEqual:@YES]) {failure(nil,nil,nil);return;}
      [@"ready" writeToURL:path(@"status") atomically:YES encoding:NSUTF8StringEncoding error:nullptr];
      dispatch_async(dispatch_get_global_queue(QOS_CLASS_UTILITY,0), ^{
        for (NSUInteger attempt=0;attempt<2700;attempt++) {
          if ([NSFileManager.defaultManager fileExistsAtPath:path(@"stop").path]) {
            [NSFileManager.defaultManager removeItemAtURL:path(@"stop") error:nullptr];
            [self stopParticipant:^(id result){
              [@"stopped" writeToURL:path(@"status") atomically:YES encoding:NSUTF8StringEncoding error:nullptr];
            } reject:failure];
            return;
          }
          [NSThread sleepForTimeInterval:0.2];
        }
      });
    } reject:failure work:^id(NSURL *directory,NSMutableData *key,NSString *authority){
      long long expires=(long long)NSDate.date.timeIntervalSince1970+300;
      return @(nu54_p03_start((char *)directory.fileSystemRepresentation,key.mutableBytes,32,
        (char *)subject.UTF8String,(char *)authority.UTF8String,expires)==0);
    }];
  };
  if (roster.length>0) {
    [self installParticipant:subject roster:roster resolve:^(id accepted){
      if ([accepted isEqual:@YES]) launch(); else failure(nil,nil,nil);
    } reject:failure];
  } else launch();
}
@end

__attribute__((constructor)) static void nu54P03SimulatorLab(void) {
  dispatch_after(dispatch_time(DISPATCH_TIME_NOW,1000*NSEC_PER_MSEC),dispatch_get_main_queue(), ^{
    NSDictionary<NSString *,NSString *> *env=NSProcessInfo.processInfo.environment;
    NSString *mode=env[@"NU54_P03_SIMULATOR_MODE"];
    NSString *bundle=NSBundle.mainBundle.bundleIdentifier;
    if (!mode || (![bundle isEqual:@"org.nu54.p03fixture"] && ![bundle isEqual:@"org.nu54.p03marketfixture"])) return;
    NSString *subject=env[@"NU54_P03_SIMULATOR_SUBJECT"];
    NSString *encoded=env[@"NU54_P03_SIMULATOR_ROSTER"];
    NSData *data=encoded ? [[NSData alloc] initWithBase64EncodedString:encoded options:0] : nil;
    NSString *roster=data ? [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] : nil;
    [[CloudWalletProfile new] runSimulatorLab:mode subject:subject roster:roster];
  });
}
#endif
