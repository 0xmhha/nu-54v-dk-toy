// Simulator-only registration transport. Production uses CoreBluetooth and Secure Enclave.
#import <TargetConditionals.h>

#if defined(NU54_OWNER_REGISTRATION_QA)
#if !TARGET_OS_SIMULATOR
#error Owner registration QA requires the iOS simulator
#endif
#import <Foundation/Foundation.h>
#import <Security/Security.h>

static inline BOOL NU54OwnerRegistrationQA(void) {
  return [NSBundle.mainBundle.bundleIdentifier isEqualToString:@"com.nu54renter.w04qa"];
}

static inline SecKeyRef NU54RegistrationTestKey(void) CF_RETURNS_RETAINED {
  if (!NU54OwnerRegistrationQA()) return nullptr;
  NSString *scopeFile = [NSHomeDirectory() stringByAppendingPathComponent:@"Documents/w04-key-scope"];
  NSString *rawScope = [NSString stringWithContentsOfFile:scopeFile encoding:NSUTF8StringEncoding error:nil];
  NSUUID *scope = [[NSUUID alloc] initWithUUIDString:[rawScope stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet]];
  if (!scope) return nullptr;
  NSData *tag = [[@"nu54.owner.registration.w04qa.p256." stringByAppendingString:scope.UUIDString]
    dataUsingEncoding:NSUTF8StringEncoding];
  NSDictionary *key = @{
    (__bridge id)kSecAttrApplicationTag:tag,
    (__bridge id)kSecAttrKeyType:(__bridge id)kSecAttrKeyTypeECSECPrimeRandom,
    (__bridge id)kSecAttrKeyClass:(__bridge id)kSecAttrKeyClassPrivate
  };
  NSMutableDictionary *query = [key mutableCopy];
  query[(__bridge id)kSecClass] = (__bridge id)kSecClassKey;
  query[(__bridge id)kSecReturnRef] = @YES;
  CFTypeRef existing = nullptr;
  OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)query, &existing);
  if (status == errSecSuccess) return (SecKeyRef)existing;
  if (status != errSecItemNotFound) return nullptr;
  return SecKeyCreateRandomKey((__bridge CFDictionaryRef)@{
    (__bridge id)kSecAttrKeyType:(__bridge id)kSecAttrKeyTypeECSECPrimeRandom,
    (__bridge id)kSecAttrKeySizeInBits:@256,
    (__bridge id)kSecPrivateKeyAttrs:@{
      (__bridge id)kSecAttrIsPermanent:@YES,
      (__bridge id)kSecAttrApplicationTag:tag,
      (__bridge id)kSecAttrAccessible:(__bridge id)kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
    },
  }, nullptr);
}
#endif
