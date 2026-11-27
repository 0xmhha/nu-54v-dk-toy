#import <CommonCrypto/CommonDigest.h>
#import <objc/runtime.h>
@import Foundation;
@import Security;
@import UserNotifications;
@import XCTest;

typedef void (^SubscriptionResolve)(id);
typedef void (^SubscriptionReject)(NSString *, NSString *, NSError *);

@protocol SubscriptionAlertsUnderTest
- (void)permission:(BOOL)ask resolve:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
- (void)schedule:(NSString *)invoice epochMilliseconds:(double)epochMilliseconds
    expiresMilliseconds:(double)expiresMilliseconds resolve:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
- (void)remove:(NSString *)invoice resolve:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
- (void)clear:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
@end

@protocol PaymentOwnerUnderTest
- (void)publicIdentity:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
- (void)loadProfile:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
- (void)compareAndSaveProfile:(NSString *)expected value:(NSString *)value
    resolve:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
@end

@protocol SubscriptionCommerceUnderTest
- (void)configure:(NSString *)slot profile:(NSString *)profile resolve:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
- (void)exchange:(NSString *)slot envelope:(NSString *)envelope resolve:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
- (void)exchangeScoped:(NSString *)slot profileDigest:(NSString *)digest envelope:(NSString *)envelope
    resolve:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
- (void)loadProfile:(NSString *)slot resolve:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
- (void)loadJournal:(NSString *)name resolve:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
- (void)saveJournal:(NSString *)name value:(NSString *)value resolve:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
- (void)publicIdentity:(NSString *)slot resolve:(SubscriptionResolve)resolve reject:(SubscriptionReject)reject;
@end

static NSString *pickupProfileDigest(NSString *raw) {
    NSData *data = [raw dataUsingEncoding:NSUTF8StringEncoding];
    unsigned char hash[CC_SHA256_DIGEST_LENGTH];
    CC_SHA256(data.bytes, (CC_LONG)data.length, hash);
    NSMutableString *out = [NSMutableString stringWithCapacity:sizeof(hash) * 2];
    for (NSUInteger index = 0; index < sizeof(hash); index++) [out appendFormat:@"%02x", hash[index]];
    return out;
}

@interface FixtureDeliveredNotification : NSObject
@property(nonatomic, strong) UNNotificationRequest *request;
@end
@implementation FixtureDeliveredNotification
@end

@interface FixtureAuthorizationSettings : NSObject
@property(nonatomic) UNAuthorizationStatus authorizationStatus;
@end
@implementation FixtureAuthorizationSettings
@end

@interface FixtureNotificationCenter : NSObject
@property(nonatomic, strong) NSMutableArray<UNNotificationRequest *> *pending;
@property(nonatomic, strong) NSMutableArray<FixtureDeliveredNotification *> *delivered;
@property(nonatomic, copy) void (^pendingReply)(NSArray<UNNotificationRequest *> *);
@property(nonatomic) NSUInteger heldPendingReads;
@property(nonatomic) NSUInteger pendingReads;
@property(nonatomic) NSUInteger deliveredReads;
@property(nonatomic) NSUInteger removals;
@property(nonatomic) UNAuthorizationStatus authorization;
@property(nonatomic) NSUInteger settingsReads;
@property(nonatomic) NSUInteger additions;
@property(nonatomic, copy) void (^heldAddition)(NSError *);
@property(nonatomic, copy) void (^onAdd)(void);
@end

@implementation FixtureNotificationCenter
- (void)getNotificationSettingsWithCompletionHandler:(void (^)(UNNotificationSettings *))complete {
    self.settingsReads++;
    FixtureAuthorizationSettings *settings = [FixtureAuthorizationSettings new];
    settings.authorizationStatus = self.authorization;
    complete((UNNotificationSettings *)settings);
}
- (void)addNotificationRequest:(UNNotificationRequest *)request withCompletionHandler:(void (^)(NSError *))callback {
    self.additions++;
    self.heldAddition = callback;
    if (self.onAdd) self.onAdd();
}
- (void)getPendingNotificationRequestsWithCompletionHandler:(void (^)(NSArray<UNNotificationRequest *> *))complete {
    self.pendingReads++;
    if (self.heldPendingReads > 0) {
        self.heldPendingReads--;
        self.pendingReply = complete;
        return;
    }
    complete(self.pending.copy);
}
- (void)getDeliveredNotificationsWithCompletionHandler:(void (^)(NSArray<UNNotification *> *))complete {
    self.deliveredReads++;
    complete((NSArray<UNNotification *> *)self.delivered.copy);
}
- (void)removePendingNotificationRequestsWithIdentifiers:(NSArray<NSString *> *)identifiers {
    self.removals++;
    NSIndexSet *removed = [self.pending indexesOfObjectsPassingTest:^BOOL(UNNotificationRequest *item, NSUInteger index, BOOL *stop) {
        return [identifiers containsObject:item.identifier];
    }];
    [self.pending removeObjectsAtIndexes:removed];
}
- (void)removeDeliveredNotificationsWithIdentifiers:(NSArray<NSString *> *)identifiers {
    self.removals++;
    NSIndexSet *removed = [self.delivered indexesOfObjectsPassingTest:^BOOL(FixtureDeliveredNotification *item, NSUInteger index, BOOL *stop) {
        return [identifiers containsObject:item.request.identifier];
    }];
    [self.delivered removeObjectsAtIndexes:removed];
}
@end

static UNNotificationRequest *notificationFixture(NSString *identifier) {
    return [UNNotificationRequest requestWithIdentifier:identifier content:[UNMutableNotificationContent new] trigger:nil];
}

static void withNotificationFixture(FixtureNotificationCenter *fixture, void (^run)(void)) {
    Class center = UNUserNotificationCenter.class;
    Method factory = class_getClassMethod(center, @selector(currentNotificationCenter));
    IMP replacement = imp_implementationWithBlock(^id(id receiver) { return fixture; });
    IMP original = method_setImplementation(factory, replacement);
    @try {
        run();
    } @finally {
        method_setImplementation(factory, original);
        imp_removeBlock(replacement);
    }
}

@interface QAListNotificationDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation QAListNotificationDelegate
- (void)userNotificationCenter:(UNUserNotificationCenter *)center willPresentNotification:(UNNotification *)notification
    withCompletionHandler:(void (^)(UNNotificationPresentationOptions))complete {
    complete(UNNotificationPresentationOptionList);
}
@end

@interface SubscriptionAlertsNative : XCTestCase
@end

@implementation SubscriptionAlertsNative

- (void)testClearTrackedAndOrphanedPreviousScopeNotifications {
    id<SubscriptionAlertsUnderTest> alerts = [NSClassFromString(@"SubscriptionAlerts") new];
    XCTAssertNotNil(alerts);
    NSString *tracked = @"nu54-subscription-fixture-old-pending";
    NSString *orphan = @"nu54-subscription-fixture-old-delivered";
    NSString *unrelated = @"unrelated-notification-fixture";
    NSUserDefaults *registry = NSUserDefaults.standardUserDefaults;
    [registry setObject:@{@"fixture": @YES} forKey:tracked];
    XCTAssertNil([registry objectForKey:orphan]);
    FixtureNotificationCenter *fixture = [FixtureNotificationCenter new];
    fixture.authorization = UNAuthorizationStatusAuthorized;
    fixture.pending = [@[notificationFixture(tracked), notificationFixture(unrelated)] mutableCopy];
    FixtureDeliveredNotification *alreadyDelivered = [FixtureDeliveredNotification new];
    alreadyDelivered.request = notificationFixture(orphan);
    fixture.delivered = [@[alreadyDelivered] mutableCopy];

    withNotificationFixture(fixture, ^{
        XCTestExpectation *cleared = [self expectationWithDescription:@"previous scope is empty before resolve"];
        [alerts clear:^(id result) {
            XCTAssertNil([registry objectForKey:tracked]);
            XCTAssertEqualObjects([fixture.pending valueForKey:@"identifier"], (@[unrelated]));
            XCTAssertEqual(fixture.delivered.count, 0U);
            XCTAssertEqual(fixture.removals, 2U);
            [cleared fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"unexpected native cleanup rejection: %@", code);
            [cleared fulfill];
        }];
        [self waitForExpectations:@[cleared] timeout:10];
    });
    [registry removeObjectForKey:tracked];
}

- (void)testNotificationCleanupRecoversWhenFirstSnapshotNeverCompletes {
    id<SubscriptionAlertsUnderTest> alerts = [NSClassFromString(@"SubscriptionAlerts") new];
    XCTAssertNotNil(alerts);
    NSString *orphan = @"nu54-subscription-fixture-untracked";
    FixtureNotificationCenter *fixture = [FixtureNotificationCenter new];
    fixture.authorization = UNAuthorizationStatusAuthorized;
    fixture.pending = [@[notificationFixture(orphan)] mutableCopy];
    fixture.delivered = [NSMutableArray new];
    fixture.heldPendingReads = 1;
    withNotificationFixture(fixture, ^{
        XCTestExpectation *clean = [self expectationWithDescription:@"the ordered retry completes cleanup"];
        [alerts clear:^(id result) {
            XCTAssertEqual(fixture.pending.count, 0U);
            XCTAssertEqual(fixture.removals, 2U);
            [clean fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"the second available snapshot must recover: %@", code);
            [clean fulfill];
        }];
        [self waitForExpectations:@[clean] timeout:7];
        XCTAssertNotNil(fixture.pendingReply);
        if (fixture.pendingReply) fixture.pendingReply(@[notificationFixture(orphan)]);
        fixture.pendingReply = nil;
        XCTestExpectation *late = [self expectationWithDescription:@"old snapshot cannot remove anything twice"];
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 250 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
            XCTAssertEqual(fixture.removals, 2U);
            [late fulfill];
        });
        [self waitForExpectations:@[late] timeout:1];
    });
}

- (void)testLateNotificationCleanupRejectsAndLeavesPriorScopeRetryable {
    id<SubscriptionAlertsUnderTest> alerts = [NSClassFromString(@"SubscriptionAlerts") new];
    XCTAssertNotNil(alerts);
    NSString *tracked = @"nu54-subscription-fixture-delayed";
    NSUserDefaults *registry = NSUserDefaults.standardUserDefaults;
    [registry setObject:@{@"fixture": @YES} forKey:tracked];
    FixtureNotificationCenter *fixture = [FixtureNotificationCenter new];
    fixture.authorization = UNAuthorizationStatusAuthorized;
    fixture.pending = [@[notificationFixture(tracked)] mutableCopy];
    fixture.delivered = [NSMutableArray new];
    fixture.heldPendingReads = 2;
    withNotificationFixture(fixture, ^{
        XCTestExpectation *timedOut = [self expectationWithDescription:@"lost native callback rejects"];
        XCTestExpectation *late = [self expectationWithDescription:@"late callback must not fulfill prior attempt"];
        late.inverted = YES;
        [alerts clear:^(id result) {
            [late fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTAssertEqualObjects(code, @"CLEAR_DEADLINE");
            [timedOut fulfill];
        }];
        [self waitForExpectations:@[timedOut] timeout:10];
        XCTAssertNotNil(fixture.pendingReply);
        if (fixture.pendingReply) fixture.pendingReply(fixture.pending.copy);
        fixture.pendingReply = nil;
        [self waitForExpectations:@[late] timeout:0.25];
        XCTAssertEqual(fixture.removals, 0U);
        XCTAssertNotNil([registry objectForKey:tracked]);
    });
    [registry removeObjectForKey:tracked];
}

- (void)testFreshClearanceDoesNotRequestPermissionAndReadsBothLists {
    id<SubscriptionAlertsUnderTest> alerts = [NSClassFromString(@"SubscriptionAlerts") new];
    FixtureNotificationCenter *fixture = [FixtureNotificationCenter new];
    fixture.authorization = UNAuthorizationStatusNotDetermined;
    fixture.pending = [NSMutableArray new];
    fixture.delivered = [NSMutableArray new];
    withNotificationFixture(fixture, ^{
        XCTestExpectation *verified = [self expectationWithDescription:@"fresh install verifies both OS lists"];
        [alerts clear:^(id result) {
            XCTAssertEqual(fixture.settingsReads, 0U);
            XCTAssertEqual(fixture.pendingReads, 1U);
            XCTAssertEqual(fixture.deliveredReads, 1U);
            XCTAssertEqual(fixture.removals, 0U);
            [verified fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"fresh install verification failed: %@", code);
            [verified fulfill];
        }];
        [self waitForExpectations:@[verified] timeout:5];
    });
}

- (void)testNativeSchedulingRefusesNeverAuthorizedStatus {
    id<SubscriptionAlertsUnderTest> alerts = [NSClassFromString(@"SubscriptionAlerts") new];
    FixtureNotificationCenter *fixture = [FixtureNotificationCenter new];
    fixture.authorization = UNAuthorizationStatusNotDetermined;
    withNotificationFixture(fixture, ^{
        double now = NSDate.date.timeIntervalSince1970 * 1000;
        XCTestExpectation *denied = [self expectationWithDescription:@"native schedule without permission rejected"];
        [alerts schedule:NSUUID.UUID.UUIDString.lowercaseString epochMilliseconds:now + 30000 expiresMilliseconds:now + 60000
                  resolve:^(id result) {
            XCTAssertEqualObjects(result, @NO);
            XCTAssertEqual(fixture.settingsReads, 1U);
            XCTAssertEqual(fixture.additions, 0U);
            [denied fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"unexpected scheduling failure: %@", code);
            [denied fulfill];
        }];
        [self waitForExpectations:@[denied] timeout:5];
    });
}

- (void)testClearOrphanWithLegacySecureJournal {
    if (![NSBundle.mainBundle.bundleIdentifier hasPrefix:@"com.nu54renter.enrollqa."]) {
        XCTFail(@"Use only the isolated enrollment QA application");
        return;
    }
    id<SubscriptionCommerceUnderTest> commerce = [NSClassFromString(@"CommerceHttp") new];
    id<SubscriptionAlertsUnderTest> alerts = [NSClassFromString(@"SubscriptionAlerts") new];
    NSString *journal = @"subscription-alerts-v2";
    NSDictionary *journalQuery = @{
        (__bridge id)kSecClass:(__bridge id)kSecClassGenericPassword,
        (__bridge id)kSecAttrService:@"nu54.commerce.enrollment.v2",
        (__bridge id)kSecAttrAccount:@"journal:subscription-alerts-v2",
        (__bridge id)kSecAttrSynchronizable:@NO,
    };
    OSStatus starting = SecItemCopyMatching((__bridge CFDictionaryRef)journalQuery, NULL);
    XCTAssertTrue(starting == errSecSuccess || starting == errSecItemNotFound);
    if (starting != errSecSuccess && starting != errSecItemNotFound) return;
    __block NSString *previous;
    XCTestExpectation *loaded = [self expectationWithDescription:@"preserve existing secure scope journal"];
    [commerce loadJournal:journal resolve:^(NSString *value) {
        previous = value;
        [loaded fulfill];
    } reject:^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"could not read the original journal: %@", code);
        [loaded fulfill];
    }];
    [self waitForExpectations:@[loaded] timeout:5];
    if (!previous) return;
    void (^save)(NSString *) = ^(NSString *value) {
        XCTestExpectation *written = [self expectationWithDescription:@"write secure scope journal"];
        [commerce saveJournal:journal value:value resolve:^(id result) {
            [written fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"could not write test journal: %@", code);
            [written fulfill];
        }];
        [self waitForExpectations:@[written] timeout:5];
    };
    save(@"legacy-fixture-old-owner");
    FixtureNotificationCenter *fixture = [FixtureNotificationCenter new];
    fixture.authorization = UNAuthorizationStatusNotDetermined;
    fixture.pending = [NSMutableArray new];
    FixtureDeliveredNotification *old = [FixtureDeliveredNotification new];
    old.request = notificationFixture(@"nu54-subscription-fixture-legacy-delivered");
    fixture.delivered = [@[old] mutableCopy];
    withNotificationFixture(fixture, ^{
        XCTestExpectation *cleared = [self expectationWithDescription:@"legacy orphan must be removed"];
        [alerts clear:^(id result) {
            XCTAssertEqual(fixture.settingsReads, 0U);
            XCTAssertEqual(fixture.delivered.count, 0U);
            XCTAssertEqual(fixture.removals, 2U);
            [cleared fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"could not clear legacy notifications: %@", code);
            [cleared fulfill];
        }];
        [self waitForExpectations:@[cleared] timeout:5];
    });
    if (starting == errSecSuccess) save(previous);
    else XCTAssertEqual(SecItemDelete((__bridge CFDictionaryRef)journalQuery), errSecSuccess);
}

- (void)testEnrollmentClearanceWaitsForInFlightNativeScheduling {
    id<SubscriptionAlertsUnderTest> alerts = [NSClassFromString(@"SubscriptionAlerts") new];
    FixtureNotificationCenter *fixture = [FixtureNotificationCenter new];
    fixture.authorization = UNAuthorizationStatusAuthorized;
    fixture.pending = [NSMutableArray new];
    fixture.delivered = [NSMutableArray new];
    withNotificationFixture(fixture, ^{
        XCTestExpectation *add = [self expectationWithDescription:@"native addition begins"];
        fixture.onAdd = ^{ [add fulfill]; };
        double now = NSDate.date.timeIntervalSince1970 * 1000;
        XCTestExpectation *finished = [self expectationWithDescription:@"native addition settles"];
        [alerts schedule:NSUUID.UUID.UUIDString.lowercaseString epochMilliseconds:now + 30000 expiresMilliseconds:now + 60000
                  resolve:^(id result) {
            XCTAssertEqualObjects(result, @NO);
            [finished fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"unexpected scheduler promise error: %@", code);
            [finished fulfill];
        }];
        [self waitForExpectations:@[add] timeout:5];
        XCTestExpectation *held = [self expectationWithDescription:@"enrollment blocked while notification is being scheduled"];
        [alerts clear:^(id result) {
            XCTFail(@"cleared a scope with an unresolved OS addition");
            [held fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTAssertEqualObjects(code, @"CLEAR_BUSY");
            [held fulfill];
        }];
        [self waitForExpectations:@[held] timeout:5];
        fixture.heldAddition([NSError errorWithDomain:@"test-notification" code:1 userInfo:nil]);
        fixture.heldAddition = nil;
        [self waitForExpectations:@[finished] timeout:5];

        fixture.authorization = UNAuthorizationStatusNotDetermined;
        FixtureDeliveredNotification *old = [FixtureDeliveredNotification new];
        old.request = notificationFixture(@"nu54-subscription-fixture-before-registry");
        [fixture.delivered addObject:old];
        XCTestExpectation *done = [self expectationWithDescription:@"spanning a native schedule still needs both OS lists"];
        [alerts clear:^(id result) {
            XCTAssertEqual(fixture.settingsReads, 1U);
            XCTAssertEqual(fixture.delivered.count, 0U);
            [done fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"unexpected reconciliation error: %@", code);
            [done fulfill];
        }];
        [self waitForExpectations:@[done] timeout:5];
    });
}

// Opt-in on a disposable install with a previously enrolled fixture customer.
- (void)testRealPriorCustomerAlertsClearWithoutChangingProfile {
    XCTAssertTrue([NSBundle.mainBundle.bundleIdentifier hasPrefix:@"com.nu54renter.enrollqa."]);
    if (![NSBundle.mainBundle.bundleIdentifier hasPrefix:@"com.nu54renter.enrollqa."]) return;
    self.continueAfterFailure = NO;
    id<SubscriptionCommerceUnderTest> commerce = [NSClassFromString(@"CommerceHttp") new];
    id<SubscriptionAlertsUnderTest> alerts = [NSClassFromString(@"SubscriptionAlerts") new];
    __block NSString *prior;
    XCTestExpectation *profile = [self expectationWithDescription:@"read previously enrolled customer"];
    [commerce loadProfile:@"customer" resolve:^(NSString *value) {
        prior = value;
        [profile fulfill];
    } reject:^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"prior customer read failed: %@", code);
        [profile fulfill];
    }];
    [self waitForExpectations:@[profile] timeout:5];
    XCTAssertGreaterThan(prior.length, 0U);
    NSString *orderRef = NSProcessInfo.processInfo.environment[@"NU54_SUBSCRIPTION_PRIOR_ORDER"];
    XCTAssertNotNil([[NSUUID alloc] initWithUUIDString:orderRef ?: @""]);
    NSData *priorData = [prior dataUsingEncoding:NSUTF8StringEncoding];
    NSDictionary *priorProfile = [NSJSONSerialization JSONObjectWithData:priorData options:0 error:NULL];
    NSString *branch = priorProfile[@"scope"][@"branchRef"];
    XCTAssertNotNil([[NSUUID alloc] initWithUUIDString:branch ?: @""]);
    void (^checkOrder)(void) = ^{
        NSDictionary *body = @{@"command": @{@"version": @2, @"contract": @"C3", @"operation": @"order.status",
            @"requestId": NSUUID.UUID.UUIDString.lowercaseString, @"domainRef": branch, @"objectRef": orderRef,
            @"expiresAt": @((long long)NSDate.date.timeIntervalSince1970 + 20), @"body": @{}}};
        NSData *data = [NSJSONSerialization dataWithJSONObject:body options:0 error:NULL];
        NSString *wire = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
        XCTestExpectation *status = [self expectationWithDescription:@"retrieve customer's original order over mTLS"];
        [commerce exchange:@"customer" envelope:wire resolve:^(NSString *response) {
            NSDictionary *reply = [NSJSONSerialization JSONObjectWithData:[response dataUsingEncoding:NSUTF8StringEncoding]
                                                                     options:0 error:NULL];
            XCTAssertEqualObjects(reply[@"status"], @"ok");
            XCTAssertEqualObjects(reply[@"order"][@"id"], orderRef);
            [status fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"original order lookup failed: %@", code);
            [status fulfill];
        }];
        [self waitForExpectations:@[status] timeout:10];
    };
    checkOrder();

    UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
    XCTestExpectation *permission = [self expectationWithDescription:@"request provisional test notification permission"];
    [center requestAuthorizationWithOptions:UNAuthorizationOptionAlert | UNAuthorizationOptionProvisional
                          completionHandler:^(BOOL granted, NSError *error) {
        XCTAssertNil(error);
        XCTAssertTrue(granted);
        [permission fulfill];
    }];
    [self waitForExpectations:@[permission] timeout:10];

    NSString *pendingRef = NSUUID.UUID.UUIDString.lowercaseString;
    NSString *pendingID = [@"nu54-subscription-" stringByAppendingString:pendingRef];
    NSString *deliveredID = [@"nu54-subscription-" stringByAppendingString:NSUUID.UUID.UUIDString.lowercaseString];
    NSString *unrelatedID = [@"unrelated-fixture-" stringByAppendingString:NSUUID.UUID.UUIDString.lowercaseString];
    id<UNUserNotificationCenterDelegate> originalDelegate = center.delegate;
    QAListNotificationDelegate *listDelegate = [QAListNotificationDelegate new];
    center.delegate = listDelegate;
    @try {
        XCTestExpectation *scheduled = [self expectationWithDescription:@"schedule prior customer's pending reminder"];
        double now = NSDate.date.timeIntervalSince1970 * 1000;
        [alerts schedule:pendingRef epochMilliseconds:now + 120000 expiresMilliseconds:now + 180000
                  resolve:^(id result) {
            XCTAssertEqualObjects(result, @YES);
            [scheduled fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"native scheduling rejected: %@", code);
            [scheduled fulfill];
        }];
        [self waitForExpectations:@[scheduled] timeout:10];

        UNMutableNotificationContent *content = [UNMutableNotificationContent new];
        content.title = @"Disposable local fixture";
        for (NSString *identifier in @[deliveredID, unrelatedID]) {
            XCTestExpectation *queued = [self expectationWithDescription:@"queue disposable notification"];
            UNNotificationTrigger *trigger = [identifier isEqualToString:unrelatedID]
                ? [UNTimeIntervalNotificationTrigger triggerWithTimeInterval:120 repeats:NO] : nil;
            [center addNotificationRequest:[UNNotificationRequest requestWithIdentifier:identifier content:content trigger:trigger]
                     withCompletionHandler:^(NSError *error) {
                XCTAssertNil(error);
                [queued fulfill];
            }];
            [self waitForExpectations:@[queued] timeout:10];
        }
        BOOL delivered = NO;
        for (NSInteger attempt = 0; attempt < 10 && !delivered; attempt++) {
            XCTestExpectation *listed = [self expectationWithDescription:@"wait for orphaned reminder in notification list"];
            __block BOOL found = NO;
            [center getDeliveredNotificationsWithCompletionHandler:^(NSArray<UNNotification *> *rows) {
                for (UNNotification *row in rows) found |= [row.request.identifier isEqualToString:deliveredID];
                dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 250 * NSEC_PER_MSEC), dispatch_get_main_queue(), ^{
                    [listed fulfill];
                });
            }];
            [self waitForExpectations:@[listed] timeout:5];
            delivered = found;
        }
        XCTAssertTrue(delivered);
        void (^checkPending)(BOOL) = ^(BOOL expected) {
            XCTestExpectation *listed = [self expectationWithDescription:@"check owned and unrelated pending reminders"];
            [center getPendingNotificationRequestsWithCompletionHandler:^(NSArray<UNNotificationRequest *> *rows) {
                NSArray *ids = [rows valueForKey:@"identifier"];
                XCTAssertEqual([ids containsObject:pendingID], expected);
                XCTAssertTrue([ids containsObject:unrelatedID]);
                [listed fulfill];
            }];
            [self waitForExpectations:@[listed] timeout:5];
        };
        checkPending(YES);
        XCTestExpectation *cleared = [self expectationWithDescription:@"production module verifies previous reminders removed"];
        [alerts clear:^(id result) {
            [cleared fulfill];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"native cleanup rejected: %@", code);
            [cleared fulfill];
        }];
        [self waitForExpectations:@[cleared] timeout:10];
        checkPending(NO);
        XCTestExpectation *verified = [self expectationWithDescription:@"verify previous delivered alert and profile"];
        [center getDeliveredNotificationsWithCompletionHandler:^(NSArray<UNNotification *> *rows) {
            for (UNNotification *row in rows) XCTAssertNotEqualObjects(row.request.identifier, deliveredID);
            [commerce loadProfile:@"customer" resolve:^(NSString *saved) {
                XCTAssertEqualObjects(saved, prior);
                [verified fulfill];
            } reject:^(NSString *code, NSString *message, NSError *error) {
                XCTFail(@"stored customer read failed: %@", code);
                [verified fulfill];
            }];
        }];
        [self waitForExpectations:@[verified] timeout:5];
        checkOrder();
    } @finally {
        center.delegate = originalDelegate;
        [center removePendingNotificationRequestsWithIdentifiers:@[pendingID, unrelatedID]];
        [center removeDeliveredNotificationsWithIdentifiers:@[deliveredID]];
    }
}

- (void)testLocalReminderUsesMinimalPendingRequestAndCanBeRemoved {
    Class module = NSClassFromString(@"SubscriptionAlerts");
    XCTAssertNotNil(module, @"Test must run inside the app that links the shipping module");
    if (!module) return;
    id<SubscriptionAlertsUnderTest> alerts = [module new];
    NSString *reference = NSUUID.UUID.UUIDString.lowercaseString;
    NSString *identifier = [@"nu54-subscription-" stringByAppendingString:reference];
    UNUserNotificationCenter *center = UNUserNotificationCenter.currentNotificationCenter;
    XCTestExpectation *permission = [self expectationWithDescription:@"notification permission"];
    [alerts permission:NO resolve:^(id result) {
        XCTAssertEqualObjects(result, @YES, @"Enable notifications for the assigned simulator test app first");
        [permission fulfill];
    } reject:^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"System permission read rejected: %@", error);
        [permission fulfill];
    }];
    [self waitForExpectations:@[permission] timeout:5];

    double now = NSDate.date.timeIntervalSince1970 * 1000;
    XCTestExpectation *scheduled = [self expectationWithDescription:@"schedule"];
    [alerts schedule:reference epochMilliseconds:now + 30000 expiresMilliseconds:now + 60000 resolve:^(id result) {
        XCTAssertEqualObjects(result, @YES);
        [scheduled fulfill];
    } reject:^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Native scheduling rejected: %@", error);
        [scheduled fulfill];
    }];
    [self waitForExpectations:@[scheduled] timeout:5];

    XCTestExpectation *duplicate = [self expectationWithDescription:@"duplicate schedule"];
    [alerts schedule:reference epochMilliseconds:now + 31000 expiresMilliseconds:now + 60000 resolve:^(id result) {
        XCTAssertEqualObjects(result, @YES);
        [duplicate fulfill];
    } reject:^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Duplicate native scheduling rejected: %@", error);
        [duplicate fulfill];
    }];
    [self waitForExpectations:@[duplicate] timeout:5];

    XCTestExpectation *pending = [self expectationWithDescription:@"pending native request"];
    [center getPendingNotificationRequestsWithCompletionHandler:^(NSArray<UNNotificationRequest *> *requests) {
        NSArray *matching = [requests filteredArrayUsingPredicate:[NSPredicate predicateWithFormat:@"identifier == %@", identifier]];
        XCTAssertEqual(matching.count, 1U);
        UNNotificationRequest *request = matching.firstObject;
        XCTAssertEqualObjects(request.content.title, @"확인할 구독 회차가 있습니다");
        XCTAssertEqual(request.content.body.length, 0U);
        XCTAssertEqual(request.content.userInfo.count, 0U);
        XCTAssertTrue([request.trigger isKindOfClass:UNTimeIntervalNotificationTrigger.class]);
        [pending fulfill];
    }];
    [self waitForExpectations:@[pending] timeout:5];

    XCTestExpectation *removed = [self expectationWithDescription:@"removed native request"];
    [alerts remove:reference resolve:^(id result) {
        [center getPendingNotificationRequestsWithCompletionHandler:^(NSArray<UNNotificationRequest *> *requests) {
            NSArray *matching = [requests filteredArrayUsingPredicate:[NSPredicate predicateWithFormat:@"identifier == %@", identifier]];
            XCTAssertEqual(matching.count, 0U);
            [removed fulfill];
        }];
    } reject:^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Native removal rejected: %@", error);
        [removed fulfill];
    }];
    [self waitForExpectations:@[removed] timeout:5];
}

- (void)testQueueRealReminderForTap {
    id<SubscriptionAlertsUnderTest> alerts = [NSClassFromString(@"SubscriptionAlerts") new];
    XCTAssertNotNil(alerts);
    NSString *reference = NSProcessInfo.processInfo.environment[@"NU54_SUBSCRIPTION_DIRECT_INVOICE"] ?: @"ab3cd900-3fb6-4821-ac19-48cb448f6633";
    XCTestExpectation *queued = [self expectationWithDescription:@"queue tap fixture"];
    SubscriptionReject reject = ^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Native request rejected: %@", error);
        [queued fulfill];
    };
    [alerts clear:^(id result) {
        double now = NSDate.date.timeIntervalSince1970 * 1000;
        [alerts schedule:reference epochMilliseconds:now + 12000 expiresMilliseconds:now + 180000 resolve:^(id scheduled) {
            XCTAssertEqualObjects(scheduled, @YES);
            [queued fulfill];
        } reject:reject];
    } reject:reject];
    [self waitForExpectations:@[queued] timeout:5];
}

- (void)testCommerceEnrollmentPublicKey {
    id<SubscriptionCommerceUnderTest> commerce = [NSClassFromString(@"CommerceHttp") new];
    XCTAssertNotNil(commerce);
    XCTestExpectation *ready = [self expectationWithDescription:@"native commerce public key"];
    NSString *slot = NSProcessInfo.processInfo.environment[@"NU54_SUBSCRIPTION_COMMERCE_SLOT"] ?: @"customer";
    [commerce publicIdentity:slot resolve:^(id result) {
        XCTAssertEqual([[NSData alloc] initWithBase64EncodedString:result options:0].length, 91U);
        NSLog(@"NU54_SUBSCRIPTION_C3_SPKI=%@", result);
        [ready fulfill];
    } reject:^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Native commerce key unavailable: %@ %@", code, message);
        [ready fulfill];
    }];
    [self waitForExpectations:@[ready] timeout:5];
}

- (void)testNativeKioskStaffEnrollment {
    NSDictionary *info = NSProcessInfo.processInfo.environment;
    NSString *slot = info[@"NU54_SUBSCRIPTION_COMMERCE_SLOT"];
    NSData *bytes = [[NSData alloc] initWithBase64EncodedString:info[@"NU54_SUBSCRIPTION_UI_FIXTURE"] options:0];
    NSDictionary *fixture = [NSJSONSerialization JSONObjectWithData:bytes options:0 error:NULL];
    NSDictionary *profile = fixture[@"profile"];
    NSData *profileData = [NSJSONSerialization dataWithJSONObject:profile options:0 error:NULL];
    NSString *raw = [[NSString alloc] initWithData:profileData encoding:NSUTF8StringEncoding];
    id<SubscriptionCommerceUnderTest> commerce = [NSClassFromString(@"CommerceHttp") new];
    XCTestExpectation *read = [self expectationWithDescription:@"mTLS read of current terminal mode"];
    SubscriptionReject reject = ^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Mode enrollment or request rejected: %@ %@", code, message);
        [read fulfill];
    };
    [commerce configure:slot profile:raw resolve:^(id result) {
        NSDictionary *scope = profile[@"scope"];
        NSArray *permissions = [fixture[@"staffPermissions"] isKindOfClass:NSArray.class] ? fixture[@"staffPermissions"] : nil;
        NSDictionary *command = @{@"command": @{@"version": @2, @"contract": @"C3",
          @"operation": permissions ? @"staff.access" : @"terminal.state", @"requestId": NSUUID.UUID.UUIDString.lowercaseString,
          @"domainRef": scope[@"branchRef"], @"objectRef": permissions ? scope[@"subjectRef"] : fixture[@"terminal"],
          @"expiresAt": @((long long)NSDate.date.timeIntervalSince1970 + 20), @"body": @{}}};
        NSData *data = [NSJSONSerialization dataWithJSONObject:command options:0 error:NULL];
        NSString *wire = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
        [commerce exchange:slot envelope:wire resolve:^(NSString *response) {
            NSDictionary *reply = [NSJSONSerialization JSONObjectWithData:
                [response dataUsingEncoding:NSUTF8StringEncoding] options:0 error:NULL];
            XCTAssertEqualObjects(reply[@"status"], @"ok");
            if (permissions) {
                XCTAssertEqualObjects(reply[@"staff"][@"permissions"], permissions);
                XCTAssertEqualObjects(reply[@"staff"][@"branchRef"], scope[@"branchRef"]);
                XCTAssertEqualObjects(reply[@"staff"][@"actorRef"], scope[@"subjectRef"]);
            } else {
                XCTAssertEqualObjects(reply[@"terminal"][@"mode"], @"menu");
            }
            [read fulfill];
        } reject:reject];
    } reject:reject];
    [self waitForExpectations:@[read] timeout:12];
}

- (void)testNativeKioskRejectsARequestBoundToTheReplacedProfile {
    NSString *encoded = NSProcessInfo.processInfo.environment[@"NU54_SUBSCRIPTION_UI_FIXTURE"];
    NSData *fixtureData = [[NSData alloc] initWithBase64EncodedString:encoded options:0];
    NSDictionary *fixture = [NSJSONSerialization JSONObjectWithData:fixtureData options:0 error:NULL];
    NSDictionary *profile = fixture[@"profile"];
    NSDictionary *scope = profile[@"scope"];
    NSData *profileData = [NSJSONSerialization dataWithJSONObject:profile options:0 error:NULL];
    NSString *raw = [[NSString alloc] initWithData:profileData encoding:NSUTF8StringEncoding];
    NSString *slot = @"operator";
    id<SubscriptionCommerceUnderTest> commerce = [NSClassFromString(@"CommerceHttp") new];
    XCTestExpectation *finished = [self expectationWithDescription:@"replacement enrollment fences prior queued requests"];
    SubscriptionReject failure = ^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Current enrollment rejected: %@", code);
        [finished fulfill];
    };
    [commerce loadProfile:slot resolve:^(NSString *previous) {
        XCTAssertGreaterThan(previous.length, 0U);
        NSString *oldDigest = pickupProfileDigest(previous);
        [commerce configure:slot profile:raw resolve:^(id result) {
            NSDictionary *command = @{@"command": @{@"version": @2, @"contract": @"C3", @"operation": @"staff.access",
                @"requestId": NSUUID.UUID.UUIDString.lowercaseString, @"domainRef": scope[@"branchRef"],
                @"objectRef": scope[@"subjectRef"], @"expiresAt": @((long long)NSDate.date.timeIntervalSince1970 + 20),
                @"body": @{}}};
            NSData *data = [NSJSONSerialization dataWithJSONObject:command options:0 error:NULL];
            NSString *wire = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
            [commerce exchangeScoped:slot profileDigest:oldDigest envelope:wire resolve:^(id response) {
                XCTFail(@"A stale request used the replacement clerk's credential");
                [finished fulfill];
            } reject:^(NSString *code, NSString *message, NSError *error) {
                XCTAssertEqualObjects(code, @"COMMERCE_UNAVAILABLE");
                [commerce exchangeScoped:slot profileDigest:pickupProfileDigest(raw) envelope:wire resolve:^(NSString *response) {
                    NSDictionary *reply = [NSJSONSerialization JSONObjectWithData:
                        [response dataUsingEncoding:NSUTF8StringEncoding] options:0 error:NULL];
                    XCTAssertEqualObjects(reply[@"status"], @"ok");
                    XCTAssertEqualObjects(reply[@"staff"][@"actorRef"], scope[@"subjectRef"]);
                    [finished fulfill];
                } reject:failure];
            }];
        } reject:failure];
    } reject:failure];
    [self waitForExpectations:@[finished] timeout:12];
}

- (void)testPaymentFixtureOwnerIdentity {
    XCTAssertEqualObjects(NSBundle.mainBundle.bundleIdentifier, @"com.nu54renter.w04qa");
    NSString *scope = NSProcessInfo.processInfo.environment[@"NU54_TRAVEL_OWNER_SCOPE"];
    if (scope) {
        XCTAssertNotNil([[NSUUID alloc] initWithUUIDString:scope]);
        NSString *documents = [NSHomeDirectory() stringByAppendingPathComponent:@"Documents"];
        XCTAssertTrue([scope writeToFile:[documents stringByAppendingPathComponent:@"w04-key-scope"]
            atomically:YES encoding:NSUTF8StringEncoding error:NULL]);
    }
    id<PaymentOwnerUnderTest> owner = [NSClassFromString(@"OwnerWallet") new];
    XCTestExpectation *ready = [self expectationWithDescription:@"simulator owner SPKI"];
    [owner publicIdentity:^(id result) {
        XCTAssertEqual([[NSData alloc] initWithBase64EncodedString:result options:0].length, 91U);
        NSLog(@"NU54_PAYMENT_OWNER_SPKI=%@", result);
        [ready fulfill];
    } reject:^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Owner identity unavailable: %@ %@", code, message);
        [ready fulfill];
    }];
    [self waitForExpectations:@[ready] timeout:5];
}

- (void)testSeedTravelBoardWallet {
    XCTAssertEqualObjects(NSBundle.mainBundle.bundleIdentifier, @"com.nu54renter.w04qa");
    NSString *encoded = NSProcessInfo.processInfo.environment[@"NU54_SUBSCRIPTION_UI_FIXTURE"];
    NSData *bytes = [[NSData alloc] initWithBase64EncodedString:encoded options:0];
    NSString *wallet = [[NSString alloc] initWithData:bytes encoding:NSUTF8StringEncoding];
    XCTAssertNotNil(wallet);
    if (!wallet) return;
    id<PaymentOwnerUnderTest> owner = [NSClassFromString(@"OwnerWallet") new];
    XCTestExpectation *stored = [self expectationWithDescription:@"simulator travel wallet"];
    SubscriptionReject reject = ^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Payment wallet seed rejected: %@", code);
        [stored fulfill];
    };
    [owner loadProfile:^(NSString *existing) {
        [owner compareAndSaveProfile:existing value:wallet resolve:^(NSNumber *matched) {
            XCTAssertTrue(matched.boolValue);
            [stored fulfill];
        } reject:reject];
    } reject:reject];
    [self waitForExpectations:@[stored] timeout:5];
}

- (void)testClearTravelBoardWallet {
    XCTAssertEqualObjects(NSBundle.mainBundle.bundleIdentifier, @"com.nu54renter.w04qa");
    NSString *documents = [NSHomeDirectory() stringByAppendingPathComponent:@"Documents"];
    NSString *scopePath = [documents stringByAppendingPathComponent:@"w04-key-scope"];
    NSString *scopeText = NSProcessInfo.processInfo.environment[@"NU54_TRAVEL_OWNER_SCOPE"] ?:
        [NSString stringWithContentsOfFile:scopePath encoding:NSUTF8StringEncoding error:NULL];
    NSUUID *scope = [[NSUUID alloc] initWithUUIDString:
        [scopeText stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet]];
    if (scope) {
        NSData *tag = [[@"nu54.owner.registration.w04qa.p256." stringByAppendingString:scope.UUIDString]
            dataUsingEncoding:NSUTF8StringEncoding];
        OSStatus removed = SecItemDelete((__bridge CFDictionaryRef)@{
            (__bridge id)kSecClass:(__bridge id)kSecClassKey,
            (__bridge id)kSecAttrApplicationTag:tag,
        });
        XCTAssertTrue(removed == errSecSuccess || removed == errSecItemNotFound);
    }
    OSStatus removed = SecItemDelete((__bridge CFDictionaryRef)@{
        (__bridge id)kSecClass:(__bridge id)kSecClassGenericPassword,
        (__bridge id)kSecAttrService:@"nu54.personal.profile.v2",
        (__bridge id)kSecAttrAccount:@"phone-local",
        (__bridge id)kSecAttrSynchronizable:@NO,
    });
    XCTAssertTrue(removed == errSecSuccess || removed == errSecItemNotFound);

    // Clear only the dedicated simulator bundle's commerce customer enrollment
    // and its journals. The simulator can retain those items after an uninstall.
    NSString *commerce = @"nu54.commerce.enrollment.v2";
    NSDictionary *query = @{
        (__bridge id)kSecClass:(__bridge id)kSecClassGenericPassword,
        (__bridge id)kSecAttrService:commerce,
        (__bridge id)kSecAttrSynchronizable:@NO,
    };
    NSMutableDictionary *search = [query mutableCopy];
    search[(__bridge id)kSecReturnAttributes] = @YES;
    search[(__bridge id)kSecMatchLimit] = (__bridge id)kSecMatchLimitAll;
    CFTypeRef found = NULL;
    OSStatus listed = SecItemCopyMatching((__bridge CFDictionaryRef)search, &found);
    XCTAssertTrue(listed == errSecSuccess || listed == errSecItemNotFound);
    for (NSDictionary *item in CFBridgingRelease(found)) {
        NSString *account = item[(__bridge id)kSecAttrAccount];
        if ([account isEqualToString:@"customer"] || [account hasPrefix:@"journal:customer-"]) {
            NSMutableDictionary *cleanup = [query mutableCopy];
            cleanup[(__bridge id)kSecAttrAccount] = account;
            XCTAssertEqual(SecItemDelete((__bridge CFDictionaryRef)cleanup), errSecSuccess);
        }
    }
    NSData *customerTag = [[commerce stringByAppendingString:@".customer"] dataUsingEncoding:NSUTF8StringEncoding];
    removed = SecItemDelete((__bridge CFDictionaryRef)@{
        (__bridge id)kSecClass:(__bridge id)kSecClassKey,
        (__bridge id)kSecAttrApplicationTag:customerTag,
    });
    XCTAssertTrue(removed == errSecSuccess || removed == errSecItemNotFound);
}

- (void)testEnrollNativePaymentReviewFixture {
    NSString *bundle = NSBundle.mainBundle.bundleIdentifier;
    XCTAssertTrue(([@[@"com.nu54renter.cataloglab", @"com.nu54renter.w04qa", @"com.nu54renter.cafepickupqa"] containsObject:bundle]));
    NSString *encoded = NSProcessInfo.processInfo.environment[@"NU54_SUBSCRIPTION_UI_FIXTURE"];
    NSData *data = [[NSData alloc] initWithBase64EncodedString:encoded options:0];
    NSDictionary *fixture = [NSJSONSerialization JSONObjectWithData:data options:0 error:NULL];
    XCTAssertNotNil(fixture[@"profile"]);
    if (!fixture[@"profile"]) return;
    NSString *(^json)(NSString *) = ^NSString *(NSString *key) {
        return [[NSString alloc] initWithData:[NSJSONSerialization dataWithJSONObject:fixture[key] options:0 error:NULL]
            encoding:NSUTF8StringEncoding];
    };
    id<SubscriptionCommerceUnderTest> native = [NSClassFromString(@"CommerceHttp") new];
    XCTestExpectation *stored = [self expectationWithDescription:@"native customer registration and existing quote"];
    SubscriptionReject reject = ^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Payment fixture enrollment rejected: %@ %@", code, message);
        [stored fulfill];
    };
    [native configure:@"customer" profile:json(@"profile") resolve:^(id result) {
        [native saveJournal:@"customer-payment-v2" value:fixture[@"pending"] ? json(@"pending") : @"null" resolve:^(id saved) {
            if (![bundle isEqualToString:@"com.nu54renter.w04qa"] || !fixture[@"wallet"]) {
                [stored fulfill];
                return;
            }
            id<PaymentOwnerUnderTest> owner = [NSClassFromString(@"OwnerWallet") new];
            [owner loadProfile:^(NSString *existing) {
                [owner compareAndSaveProfile:existing value:json(@"wallet") resolve:^(NSNumber *matched) {
                    XCTAssertTrue(matched.boolValue);
                    [stored fulfill];
                } reject:reject];
            } reject:reject];
        } reject:reject];
    } reject:reject];
    [self waitForExpectations:@[stored] timeout:8];
}

- (void)testClearKioskOutcomeEnrollment {
    if (![NSBundle.mainBundle.bundleIdentifier isEqualToString:@"com.nu54kiosk.outcomeqa"]) {
        XCTFail(@"K08 acceptance cleanup requires the dedicated test app");
        return;
    }
    NSString *service = @"nu54.commerce.enrollment.v2";
    OSStatus removed = SecItemDelete((__bridge CFDictionaryRef)@{
        (__bridge id)kSecClass:(__bridge id)kSecClassGenericPassword,
        (__bridge id)kSecAttrService:service,
        (__bridge id)kSecAttrSynchronizable:@NO,
    });
    XCTAssertTrue(removed == errSecSuccess || removed == errSecItemNotFound);
    removed = SecItemDelete((__bridge CFDictionaryRef)@{
        (__bridge id)kSecClass:(__bridge id)kSecClassKey,
        (__bridge id)kSecAttrApplicationTag:[[service stringByAppendingString:@".terminal"] dataUsingEncoding:NSUTF8StringEncoding],
    });
    XCTAssertTrue(removed == errSecSuccess || removed == errSecItemNotFound);
}

- (void)testNativeCatalogReadsPublishedDeclarations {
    NSDictionary *info = NSProcessInfo.processInfo.environment;
    NSString *slot = info[@"NU54_SUBSCRIPTION_COMMERCE_SLOT"];
    NSString *bundle = NSBundle.mainBundle.bundleIdentifier;
    XCTAssertTrue(([@[@"com.nu54kiosk.cataloglab", @"com.nu54renter.cataloglab"] containsObject:bundle]));
    NSString *encoded = info[@"NU54_SUBSCRIPTION_UI_FIXTURE"];
    NSData *bytes = encoded ? [[NSData alloc] initWithBase64EncodedString:encoded options:0] : nil;
    NSDictionary *fixture = bytes ? [NSJSONSerialization JSONObjectWithData:bytes options:0 error:NULL] : nil;
    NSDictionary *profile = fixture[@"profile"];
    NSString *wantedSKU = fixture[@"sku"];
    XCTAssertNotNil(profile);
    XCTAssertNotNil(wantedSKU);
    if (!profile || !wantedSKU) return;
    NSData *profileData = [NSJSONSerialization dataWithJSONObject:profile options:0 error:NULL];
    NSString *raw = [[NSString alloc] initWithData:profileData encoding:NSUTF8StringEncoding];
    id<SubscriptionCommerceUnderTest> commerce = [NSClassFromString(@"CommerceHttp") new];
    XCTestExpectation *read = [self expectationWithDescription:@"mTLS read of published catalog"];
    SubscriptionReject reject = ^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Catalog enrollment or request rejected: %@ %@", code, message);
        [read fulfill];
    };
    [commerce configure:slot profile:raw resolve:^(id result) {
        NSDictionary *scope = profile[@"scope"];
        NSDictionary *command = @{@"command": @{@"version": @2, @"contract": @"C3",
          @"operation": @"catalog.read", @"requestId": NSUUID.UUID.UUIDString.lowercaseString,
          @"domainRef": scope[@"branchRef"], @"objectRef": scope[@"branchRef"],
          @"expiresAt": @((long long)NSDate.date.timeIntervalSince1970 + 20), @"body": @{@"page": @"0"}}};
        NSData *data = [NSJSONSerialization dataWithJSONObject:command options:0 error:NULL];
        NSString *wire = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
        [commerce exchange:slot envelope:wire resolve:^(NSString *response) {
            NSDictionary *reply = [NSJSONSerialization JSONObjectWithData:
                [response dataUsingEncoding:NSUTF8StringEncoding] options:0 error:NULL];
            XCTAssertEqualObjects(reply[@"status"], @"ok");
            NSPredicate *wanted = [NSPredicate predicateWithFormat:@"sku == %@ AND optionKey == %@", wantedSKU, fixture[@"option"]];
            NSArray *matches = [reply[@"catalog"] filteredArrayUsingPredicate:wanted];
            XCTAssertEqual(matches.count, 1U);
            NSDictionary *row = matches.firstObject;
            XCTAssertEqualObjects(row[@"unitPrice"], fixture[@"price"]);
            XCTAssertEqualObjects(row[@"metadata"], fixture[@"metadata"]);
            [read fulfill];
        } reject:reject];
    } reject:reject];
    [self waitForExpectations:@[read] timeout:12];
}

- (void)testEnrollNativeCustomerAndQueueActualInvoice {
    NSString *fixtureData = NSProcessInfo.processInfo.environment[@"NU54_SUBSCRIPTION_UI_FIXTURE"];
    NSData *bytes = fixtureData ? [[NSData alloc] initWithBase64EncodedString:fixtureData options:0] : nil;
    NSDictionary *fixture = bytes ? [NSJSONSerialization JSONObjectWithData:bytes options:0 error:NULL] : nil;
    NSString *device = NSProcessInfo.processInfo.environment[@"NU54_SUBSCRIPTION_DEVICE"] ?: @"ios";
    BOOL manual = [NSProcessInfo.processInfo.environment[@"NU54_SUBSCRIPTION_MANUAL"] isEqual:@"1"];
    NSDictionary *profile = fixture[@"profiles"][device];
    NSString *invoice = fixture[@"invoice"][@"id"];
    XCTAssertNotNil(profile);
    XCTAssertNotNil(invoice);
    if (!profile || !invoice) return;
    NSData *profileData = [NSJSONSerialization dataWithJSONObject:profile options:0 error:NULL];
    NSString *raw = [[NSString alloc] initWithData:profileData encoding:NSUTF8StringEncoding];
    id<SubscriptionCommerceUnderTest> commerce = [NSClassFromString(@"CommerceHttp") new];
    id<SubscriptionAlertsUnderTest> alerts = [NSClassFromString(@"SubscriptionAlerts") new];
    XCTestExpectation *enrolled = [self expectationWithDescription:@"enrolled native customer and queued actual invoice"];
    SubscriptionReject reject = ^(NSString *code, NSString *message, NSError *error) {
        XCTFail(@"Native enrollment or notification rejected: %@ %@", code, message);
        [enrolled fulfill];
    };
    [commerce loadProfile:@"customer" resolve:^(id prior) {
        NSUserDefaults *testState = NSUserDefaults.standardUserDefaults;
        NSDictionary *last = [testState dictionaryForKey:@"nu54.test.subscription.enrollment"];
        NSString *knownPrior = NSProcessInfo.processInfo.environment[@"NU54_SUBSCRIPTION_PREVIOUS_PROFILE"];
        if (!last && knownPrior) {
            NSData *encodedPrior = [[NSData alloc] initWithBase64EncodedString:knownPrior options:0];
            last = encodedPrior ? [NSJSONSerialization JSONObjectWithData:encodedPrior options:0 error:NULL] : nil;
        }
        NSDictionary *previous = [prior length] ? [NSJSONSerialization JSONObjectWithData:
            [prior dataUsingEncoding:NSUTF8StringEncoding] options:0 error:NULL] : nil;
        if ([prior length] > 0 && ![previous isEqual:profile] && ![previous isEqual:last]) {
            XCTFail(@"Use an isolated simulator without an unrelated commerce enrollment");
            [enrolled fulfill];
            return;
        }
        [commerce configure:@"customer" profile:raw resolve:^(id result) {
            if (!manual) [testState setObject:profile forKey:@"nu54.test.subscription.enrollment"];
            NSDictionary *scope = profile[@"scope"];
            NSDictionary *body = @{@"command": @{@"version": @2, @"contract": @"C3",
                @"operation": @"invoice.list", @"requestId": NSUUID.UUID.UUIDString.lowercaseString,
                @"domainRef": scope[@"branchRef"], @"objectRef": scope[@"subjectRef"],
                @"expiresAt": @((long long)NSDate.date.timeIntervalSince1970 + 20), @"body": @{}}};
            NSData *command = [NSJSONSerialization dataWithJSONObject:body options:0 error:NULL];
            NSString *wire = [[NSString alloc] initWithData:command encoding:NSUTF8StringEncoding];
            [commerce exchange:@"customer" envelope:wire resolve:^(NSString *response) {
                NSDictionary *reply = [NSJSONSerialization JSONObjectWithData:
                    [response dataUsingEncoding:NSUTF8StringEncoding] options:0 error:NULL];
                XCTAssertEqualObjects(reply[@"status"], @"ok");
                NSArray *rows = reply[@"invoices"];
                XCTAssertEqualObjects([rows.firstObject objectForKey:@"id"], invoice);
                if (manual) {
                    [enrolled fulfill];
                    return;
                }
                [alerts clear:^(id cleared) {
                    double now = NSDate.date.timeIntervalSince1970 * 1000;
                    [alerts schedule:invoice epochMilliseconds:now + 12000 expiresMilliseconds:now + 180000
                              resolve:^(id scheduled) {
                        XCTAssertEqualObjects(scheduled, @YES);
                        [enrolled fulfill];
                    } reject:reject];
                } reject:reject];
            } reject:reject];
        } reject:reject];
    } reject:reject];
    [self waitForExpectations:@[enrolled] timeout:12];
}

- (void)testNativeLeafAndIntermediateEnrollment {
    NSString *encoded = NSProcessInfo.processInfo.environment[@"NU54_SUBSCRIPTION_UI_FIXTURE"];
    NSData *fixture = encoded ? [[NSData alloc] initWithBase64EncodedString:encoded options:0] : nil;
    NSDictionary *input = fixture ? [NSJSONSerialization JSONObjectWithData:fixture options:0 error:NULL] : nil;
    NSString *slot = input[@"slot"];
    NSArray *profiles = input[@"profiles"];
    XCTAssertTrue([slot isEqual:@"customer"] || [slot isEqual:@"terminal"]);
    XCTAssertEqual(profiles.count, 2U);
    if (profiles.count != 2) return;
    id<SubscriptionCommerceUnderTest> commerce = [NSClassFromString(@"CommerceHttp") new];
    for (NSUInteger index = 0; index < profiles.count; index++) {
        NSDictionary *profile = profiles[index];
        XCTAssertEqual([profile[@"certificates"] count], index + 1);
        NSData *data = [NSJSONSerialization dataWithJSONObject:profile options:0 error:NULL];
        NSString *raw = [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
        XCTestExpectation *configured = [self expectationWithDescription:@"shipping client credential configuration"];
        [commerce configure:slot profile:raw resolve:^(id result) {
            [commerce loadProfile:slot resolve:^(id saved) {
                XCTAssertEqualObjects(saved, raw);
                [configured fulfill];
            } reject:^(NSString *code, NSString *message, NSError *error) {
                XCTFail(@"Stored profile rejected: %@", code);
                [configured fulfill];
            }];
        } reject:^(NSString *code, NSString *message, NSError *error) {
            XCTFail(@"Certificate chain of %lu rejected: %@", (unsigned long)index + 1, code);
            [configured fulfill];
        }];
        [self waitForExpectations:@[configured] timeout:5];
    }
}
@end
