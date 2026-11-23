#include <Nu54ProtocolMobile/CommerceHttp.h>

#import <UserNotifications/UserNotifications.h>
#import <NU54RenterSpec/NU54RenterSpec.h>

static NSString *const subscriptionPrefix = @"nu54-subscription-";

// These flags are shared across module instances on the main queue.
static BOOL subscriptionClearing = NO;
static BOOL subscriptionScheduling = NO;

static BOOL subscriptionAuthorizationAllowsScheduling(UNNotificationSettings *settings) {
  return settings && (settings.authorizationStatus == UNAuthorizationStatusAuthorized ||
                      settings.authorizationStatus == UNAuthorizationStatusProvisional);
}

static void collectSubscriptionNotifications(
    UNUserNotificationCenter *center, void (^complete)(NSSet<NSString *> *)) {
  [center getPendingNotificationRequestsWithCompletionHandler:^(NSArray<UNNotificationRequest *> *pending) {
    [center getDeliveredNotificationsWithCompletionHandler:^(NSArray<UNNotification *> *delivered) {
      NSMutableSet<NSString *> *owned = [NSMutableSet set];
      for (UNNotificationRequest *request in pending) {
        if ([request.identifier hasPrefix:subscriptionPrefix]) {
          [owned addObject:request.identifier];
        }
      }
      for (UNNotification *notification in delivered) {
        if ([notification.request.identifier hasPrefix:subscriptionPrefix]) {
          [owned addObject:notification.request.identifier];
        }
      }
      dispatch_async(dispatch_get_main_queue(), ^{
        complete(pending && delivered ? owned : nil);
      });
    }];
  }];
}

@interface SubscriptionAlerts : NativeSubscriptionAlertsSpecBase <NativeSubscriptionAlertsSpec>
@end

@implementation SubscriptionAlerts
RCT_EXPORT_MODULE(SubscriptionAlerts)

- (std::shared_ptr<facebook::react::TurboModule>)getTurboModule:
    (const facebook::react::ObjCTurboModule::InitParams &)params {
  return std::make_shared<facebook::react::NativeSubscriptionAlertsSpecJSI>(params);
}

- (void)permission:(BOOL)ask resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
  if (ask) {
    [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
        completionHandler:^(BOOL granted, NSError *error) { resolve(@(granted && !error)); }];
  } else {
    [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
      resolve(@(subscriptionAuthorizationAllowsScheduling(settings)));
    }];
  }
}

- (void)schedule:(NSString *)invoice epochMilliseconds:(double)epochMilliseconds
    expiresMilliseconds:(double)expiresMilliseconds resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  double seconds = epochMilliseconds / 1000 - NSDate.date.timeIntervalSince1970;
  if (invoice.length != 36 || ![[NSUUID alloc] initWithUUIDString:invoice] || !(seconds > 0 && seconds <= 90 * 86400 && expiresMilliseconds > epochMilliseconds)) {
    resolve(@NO);
    return;
  }
  dispatch_async(dispatch_get_main_queue(), ^{
    if (subscriptionClearing || subscriptionScheduling) {
      resolve(@NO);
      return;
    }
    subscriptionScheduling = YES;
    void (^finish)(BOOL) = ^(BOOL accepted) {
      subscriptionScheduling = NO;
      resolve(@(accepted));
    };

    UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
    __block BOOL checked = NO;
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 4 * NSEC_PER_SEC), dispatch_get_main_queue(), ^{
      if (checked) return;
      checked = YES;
      finish(NO);
    });
    [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
      dispatch_async(dispatch_get_main_queue(), ^{
        if (checked) return;
        checked = YES;
        if (!subscriptionAuthorizationAllowsScheduling(settings)) {
          finish(NO);
          return;
        }
        NSTimeInterval delay = epochMilliseconds / 1000 - NSDate.date.timeIntervalSince1970;
        if (!(delay > 0 && delay <= 90 * 86400)) {
          finish(NO);
          return;
        }
        NSString *identifier = [subscriptionPrefix stringByAppendingString:invoice];
        NSUserDefaults *registry = NSUserDefaults.standardUserDefaults;
        NSDictionary *prior = [registry dictionaryForKey:identifier];
        if ([prior[@"until"] doubleValue] == expiresMilliseconds &&
            [prior[@"at"] doubleValue] <= epochMilliseconds) {
          finish(YES);
          return;
        }
        UNMutableNotificationContent *content = [UNMutableNotificationContent new];
        content.title = @"확인할 구독 회차가 있습니다";
        UNTimeIntervalNotificationTrigger *trigger = [UNTimeIntervalNotificationTrigger triggerWithTimeInterval:delay repeats:NO];
        UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:identifier content:content trigger:trigger];
        [center addNotificationRequest:request withCompletionHandler:^(NSError *error) {
          dispatch_async(dispatch_get_main_queue(), ^{
            if (!error) {
              [registry setObject:@{@"at": @(epochMilliseconds), @"until": @(expiresMilliseconds)} forKey:identifier];
            }
            finish(error == nil);
          });
        }];
      });
    }];
  });
}

- (void)clear:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(), ^{
    if (subscriptionClearing || subscriptionScheduling) {
      reject(@"CLEAR_BUSY", @"Could not verify notification cleanup", nil);
      return;
    }
    subscriptionClearing = YES;
    __block BOOL finished = NO;
    NSTimeInterval deadline = NSProcessInfo.processInfo.systemUptime + 8;
    void (^fail)(NSString *) = ^(NSString *code) {
      if (finished) return;
      finished = YES;
      subscriptionClearing = NO;
      reject(code, @"Could not verify notification cleanup", nil);
    };
    BOOL (^current)(void) = ^{
      if (NSProcessInfo.processInfo.systemUptime >= deadline) fail(@"CLEAR_DEADLINE");
      return !finished;
    };
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 8 * NSEC_PER_SEC), dispatch_get_main_queue(), ^{
      fail(@"CLEAR_DEADLINE");
    });

    UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
    void (^read)(void (^)(NSSet<NSString *> *)) = ^(void (^complete)(NSSet<NSString *> *)) {
      __block BOOL answered = NO;
      void (^accept)(NSSet<NSString *> *) = ^(NSSet<NSString *> *observed) {
        if (!current() || answered || !observed) return;
        answered = YES;
        complete(observed);
      };
      collectSubscriptionNotifications(center, accept);
      dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 2500 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
        if (current() && !answered) collectSubscriptionNotifications(center, accept);
      });
    };
    NSUserDefaults *registry = NSUserDefaults.standardUserDefaults;
    NSMutableSet<NSString *> *tracked = [NSMutableSet set];
    for (NSString *key in registry.dictionaryRepresentation) {
      if ([key hasPrefix:subscriptionPrefix]) [tracked addObject:key];
    }
    void (^confirm)(NSSet<NSString *> *) = ^(NSSet<NSString *> *remaining) {
      if (!current()) return;
      if (!remaining || remaining.count > 0) {
        fail(@"CLEAR_REMAINS");
        return;
      }
      for (NSString *key in tracked) [registry removeObjectForKey:key];
      finished = YES;
      subscriptionClearing = NO;
      resolve(nil);
    };
    // Old installs may have system reminders without registry entries. Each
    // snapshot reads pending before delivered so a delivery cannot disappear
    // between the two lists. A stalled snapshot gets one read-only retry.
    read(^(NSSet<NSString *> *observed) {
      if (!current()) return;
      if (observed.count == 0) {
        confirm(observed);
        return;
      }
      NSArray<NSString *> *identifiers = observed.allObjects;
      [center removePendingNotificationRequestsWithIdentifiers:identifiers];
      [center removeDeliveredNotificationsWithIdentifiers:identifiers];
      read(confirm);
    });
  });
}

- (void)remove:(NSString *)invoice resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  if (invoice.length == 36 && [[NSUUID alloc] initWithUUIDString:invoice]) {
    NSArray<NSString *> *ids = @[[subscriptionPrefix stringByAppendingString:invoice]];
    [NSUserDefaults.standardUserDefaults removeObjectForKey:ids.firstObject];
    [UNUserNotificationCenter.currentNotificationCenter removePendingNotificationRequestsWithIdentifiers:ids];
    [UNUserNotificationCenter.currentNotificationCenter removeDeliveredNotificationsWithIdentifiers:ids];
  }
  resolve(nil);
}

// Names no order, store or amount; the app shows those after it opens.
- (void)orderReady:(NSString *)order resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  if (order.length != 36 || ![[NSUUID alloc] initWithUUIDString:order]) { resolve(@NO); return; }
  UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
  [center getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *settings) {
    if (!subscriptionAuthorizationAllowsScheduling(settings)) { resolve(@NO); return; }
    UNMutableNotificationContent *content = [UNMutableNotificationContent new];
    content.title = @"주문한 상품이 준비되었습니다";
    UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:[@"nu54-order-" stringByAppendingString:order]
                                                                          content:content trigger:nil];
    [center addNotificationRequest:request withCompletionHandler:^(NSError *error) { resolve(@(error == nil)); }];
  }];
}
@end
