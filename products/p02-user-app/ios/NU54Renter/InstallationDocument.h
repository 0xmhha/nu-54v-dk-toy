#import <Foundation/Foundation.h>
#import <CoreImage/CoreImage.h>
#import <ImageIO/ImageIO.h>

// The system picker owns the URL. Read a bounded copy only; never persist the grant.
static NSData *NU54ReadWalletDocument(NSURL *url, NSUInteger limit) {
  if (!url || !url.isFileURL) return nil;
  BOOL scoped = [url startAccessingSecurityScopedResource];
  NSFileHandle *file = nil;
  NSData *data = nil;
  @try {
    file = [NSFileHandle fileHandleForReadingFromURL:url error:NULL];
    data = [file readDataOfLength:limit + 1];
  } @catch (NSException *exception) {
    data = nil;
  } @finally {
    [file closeFile];
    if (scoped) [url stopAccessingSecurityScopedResource];
  }
  return data.length > 0 && data.length <= limit ? data : nil;
}


static NSString *NU54ReadInstallationDocument(NSURL *url) {
  if (![url.pathExtension.lowercaseString isEqualToString:@"json"]) return nil;
  NSData *data=NU54ReadWalletDocument(url,8192);
  return data ? [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding] : nil;
}

static NSString *NU54ReadPsbtDocument(NSURL *url) {
  NSString *ext=url.pathExtension.lowercaseString;
  if ([@[@"psbt",@"txt"] containsObject:ext]) {
    NSData *data=NU54ReadWalletDocument(url,2378);
    if (!data) return nil;
    const uint8_t magic[]={0x70,0x73,0x62,0x74,0xff};
    if (data.length>=sizeof(magic) && !memcmp(data.bytes,magic,sizeof(magic)))
      return [ext isEqualToString:@"psbt"] && data.length<=1780 ? [data base64EncodedStringWithOptions:0] : nil;
    return [[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding];
  }
  if (![@[@"png",@"jpg",@"jpeg"] containsObject:ext]) return nil;
  NSData *data=NU54ReadWalletDocument(url,2*1024*1024);
  if (!data) return nil;
  CGImageSourceRef source=CGImageSourceCreateWithData((__bridge CFDataRef)data,nil);
  if (!source) return nil;
  CFStringRef imageType=CGImageSourceGetType(source);
  CFStringRef expectedType=[ext isEqualToString:@"png"] ? CFSTR("public.png") : CFSTR("public.jpeg");
  if (!imageType || !CFEqual(imageType,expectedType)) {CFRelease(source);return nil;}
  NSDictionary *properties=CFBridgingRelease(CGImageSourceCopyPropertiesAtIndex(source,0,nil));
  NSUInteger width=[properties[(__bridge NSString *)kCGImagePropertyPixelWidth] unsignedIntegerValue];
  NSUInteger height=[properties[(__bridge NSString *)kCGImagePropertyPixelHeight] unsignedIntegerValue];
  if (!width || !height || width>4096 || height>4096 || CGImageSourceGetCount(source)!=1) {CFRelease(source);return nil;}
  CGImageRef image=CGImageSourceCreateImageAtIndex(source,0,nil);
  CFRelease(source);
  if (!image) return nil;
  CIImage *input=[CIImage imageWithCGImage:image];
  CGImageRelease(image);
  CIDetector *detector=[CIDetector detectorOfType:CIDetectorTypeQRCode context:nil options:@{CIDetectorAccuracy:CIDetectorAccuracyHigh}];
  NSArray<CIFeature *> *codes=[detector featuresInImage:input];
  if (codes.count!=1 || ![codes[0] isKindOfClass:CIQRCodeFeature.class]) return nil;
  return ((CIQRCodeFeature *)codes[0]).messageString;
}
