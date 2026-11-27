// Exercise the same bounded native document reader used by the iOS picker.
#import "../ios/NU54Renter/InstallationDocument.h"

int main(int argc, const char *argv[]) {
  @autoreleasepool {
    if (argc!=4) return 2;
    NSURL *vector=[NSURL fileURLWithPath:[NSString stringWithUTF8String:argv[1]]];
    NSURL *scratch=[NSURL fileURLWithPath:[NSString stringWithUTF8String:argv[2]] isDirectory:YES];
    NSDictionary *sample=[NSJSONSerialization JSONObjectWithData:[NSData dataWithContentsOfURL:vector] options:0 error:nil];
    NSString *expected=sample[@"unsignedPSBT"];
    NSData *raw=[[NSData alloc] initWithBase64EncodedString:expected options:0];
    if (!raw || raw.length>1780) return 3;

    NSURL *binary=[scratch URLByAppendingPathComponent:@"test.psbt"];
    NSURL *text=[scratch URLByAppendingPathComponent:@"test.txt"];
    NSURL *qr=[NSURL fileURLWithPath:[NSString stringWithUTF8String:argv[3]]];
    if (![raw writeToURL:binary atomically:YES] ||
        ![[expected dataUsingEncoding:NSUTF8StringEncoding] writeToURL:text atomically:YES]) return 4;
    for (NSURL *item in @[binary,text,qr]) {
      if (![NU54ReadPsbtDocument(item) isEqualToString:expected] || NU54ReadInstallationDocument(item)) return 6;
    }
    NSMutableData *tooLarge=[raw mutableCopy];
    tooLarge.length=1781;
    if (![tooLarge writeToURL:binary atomically:YES] || NU54ReadPsbtDocument(binary)) return 7;
    NSMutableData *oversizedText=[NSMutableData dataWithLength:2379];
    if (![oversizedText writeToURL:text atomically:YES] || NU54ReadPsbtDocument(text)) return 8;
    NSURL *wrongType=[scratch URLByAppendingPathComponent:@"fake.png"];
    if (![raw writeToURL:wrongType atomically:YES] || NU54ReadPsbtDocument(wrongType)) return 9;
    puts("PASS native PSBT binary, Base64, QR, bounded input and installation file separation");
    return 0;
  }
}
