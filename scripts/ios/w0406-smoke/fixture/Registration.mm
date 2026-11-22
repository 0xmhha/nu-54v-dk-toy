#import <UIKit/UIKit.h>
#import "OwnerRegistrationQA.h"

@interface RegistrationFixture : UIResponder <UIApplicationDelegate>
@end
@implementation RegistrationFixture
@end

@interface RegistrationFixtureScene : UIResponder <UIWindowSceneDelegate>
@property(nonatomic,strong) UIWindow *window;
@end

@implementation RegistrationFixtureScene
- (void)scene:(UIScene *)scene willConnectToSession:(UISceneSession *)session options:(UISceneConnectionOptions *)options {
  if (![scene isKindOfClass:UIWindowScene.class]) return;
  self.window = [[UIWindow alloc] initWithWindowScene:(UIWindowScene *)scene];
  self.window.rootViewController = [UIViewController new];
  BOOL ready = NU54OwnerRegistrationQA() && [self prepare];
  self.window.rootViewController.view.backgroundColor = ready ? UIColor.greenColor : UIColor.redColor;
  [self.window makeKeyAndVisible];
  NSLog(@"W04 keychain fixture ready=%d", ready);
}

- (BOOL)prepare {
  NSString *documents = [NSHomeDirectory() stringByAppendingPathComponent:@"Documents"];
  NSString *profilePath = [documents stringByAppendingPathComponent:@"w04-enrollment.json"];
  NSData *profile = [NSData dataWithContentsOfFile:profilePath];
  NSDictionary *query = @{
    (__bridge id)kSecClass:(__bridge id)kSecClassGenericPassword,
    (__bridge id)kSecAttrService:@"nu54.personal.profile.v2",
    (__bridge id)kSecAttrAccount:@"phone-local",
    (__bridge id)kSecAttrSynchronizable:@NO,
  };
  if ([[NSFileManager defaultManager] fileExistsAtPath:[documents stringByAppendingPathComponent:@"w06-check-profile"]]) {
    NSMutableDictionary *read = [query mutableCopy];
    read[(__bridge id)kSecReturnData] = @YES;
    CFTypeRef result = nullptr;
    OSStatus status = SecItemCopyMatching((__bridge CFDictionaryRef)read, &result);
    NSData *existing = CFBridgingRelease(result);
    if (status != errSecSuccess || !existing) return NO;
    return [existing writeToFile:[documents stringByAppendingPathComponent:@"w06-wallet-observed"] atomically:YES];
  }
  if (profile) {
    NSMutableDictionary *item = [query mutableCopy];
    item[(__bridge id)kSecAttrAccessible] = (__bridge id)kSecAttrAccessibleWhenUnlockedThisDeviceOnly;
    item[(__bridge id)kSecValueData] = profile;
    OSStatus status = SecItemAdd((__bridge CFDictionaryRef)item, nullptr);
    if (status != errSecSuccess) {
      NSLog(@"W04 profile provision failed status=%d", (int)status);
      return NO;
    }
    return [[NSData data] writeToFile:[documents stringByAppendingPathComponent:@"w04-profile-ready"] atomically:YES];
  }

  if ([[NSFileManager defaultManager] fileExistsAtPath:[documents stringByAppendingPathComponent:@"w06-check-empty"]] &&
      SecItemCopyMatching((__bridge CFDictionaryRef)query, nullptr) != errSecItemNotFound) return NO;

  // iOS may retain the dedicated QA profile after an earlier QA app uninstall.
  OSStatus removed = SecItemDelete((__bridge CFDictionaryRef)query);
  if (removed != errSecSuccess && removed != errSecItemNotFound) return NO;
  id privateKey = CFBridgingRelease(NU54RegistrationTestKey());
  if (!privateKey) return NO;
  id publicKey = CFBridgingRelease(SecKeyCopyPublicKey((__bridge SecKeyRef)privateKey));
  if (!publicKey) return NO;
  NSData *publicPoint = CFBridgingRelease(SecKeyCopyExternalRepresentation((__bridge SecKeyRef)publicKey, nullptr));
  return publicPoint.length == 65 &&
    [publicPoint writeToFile:[documents stringByAppendingPathComponent:@"w04-owner-point"] atomically:YES];
}
@end

int main(int argc, char **argv) {
  @autoreleasepool {
    return UIApplicationMain(argc, argv, nil, NSStringFromClass(RegistrationFixture.class));
  }
}
