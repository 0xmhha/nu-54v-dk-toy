#import <Foundation/Foundation.h>
#import "../ios/NU54Renter/InstallationDocument.h"
#include <assert.h>
int main(void) { @autoreleasepool {
  NSString *directory = [NSTemporaryDirectory() stringByAppendingPathComponent:NSUUID.UUID.UUIDString];
  assert([NSFileManager.defaultManager createDirectoryAtPath:directory withIntermediateDirectories:YES attributes:nil error:NULL]);
  NSURL *json = [NSURL fileURLWithPath:[directory stringByAppendingPathComponent:@"fixture.json"]];
  assert([@"{\"v\":1}" writeToURL:json atomically:YES encoding:NSUTF8StringEncoding error:NULL]);
  assert([NU54ReadInstallationDocument(json) isEqualToString:@"{\"v\":1}"]);
  NSURL *other = [NSURL fileURLWithPath:[directory stringByAppendingPathComponent:@"fixture.txt"]];
  assert([@"{}" writeToURL:other atomically:YES encoding:NSUTF8StringEncoding error:NULL]);
  assert(NU54ReadInstallationDocument(other) == nil);
  NSMutableData *oversize = [NSMutableData dataWithLength:8193];
  assert([oversize writeToURL:json atomically:YES]); assert(NU54ReadInstallationDocument(json) == nil);
  unsigned char invalid[] = {0xff,0xfe}; NSData *bad = [NSData dataWithBytes:invalid length:sizeof(invalid)];
  assert([bad writeToURL:json atomically:YES]); assert(NU54ReadInstallationDocument(json) == nil);
  assert(NU54ReadInstallationDocument([NSURL fileURLWithPath:[directory stringByAppendingPathComponent:@"missing.json"]]) == nil);
  [NSFileManager.defaultManager removeItemAtPath:directory error:NULL];
  puts("PASS bounded installation document");
} return 0; }
