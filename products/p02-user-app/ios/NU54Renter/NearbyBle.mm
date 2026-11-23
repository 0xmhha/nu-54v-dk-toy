#include <Nu54ProtocolMobile/NearbyBle.h>

#import <AVFoundation/AVFoundation.h>
#import <CoreImage/CoreImage.h>
#import <React/RCTUtils.h>

@interface ShopQr : NSObject <NativeShopQrSpec, UIImagePickerControllerDelegate, UINavigationControllerDelegate>
@end

@implementation ShopQr {
  RCTPromiseResolveBlock _resolve;
  RCTPromiseRejectBlock _reject;
}
RCT_EXPORT_MODULE(ShopQr)

- (std::shared_ptr<facebook::react::TurboModule>)getTurboModule:
    (const facebook::react::ObjCTurboModule::InitParams &)params {
  return std::make_shared<facebook::react::NativeShopQrSpecJSI>(params);
}

- (void)scan:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(), ^{
    UIViewController *host = RCTPresentedViewController();
    if (self->_resolve || !host || ![UIImagePickerController isSourceTypeAvailable:UIImagePickerControllerSourceTypeCamera] ||
        [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo] == AVAuthorizationStatusDenied ||
        [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeVideo] == AVAuthorizationStatusRestricted) {
      reject(@"SHOP_CAMERA", @"Camera unavailable", nil);
      return;
    }
    self->_resolve = [resolve copy];
    self->_reject = [reject copy];
    UIImagePickerController *picker = [UIImagePickerController new];
    picker.sourceType = UIImagePickerControllerSourceTypeCamera;
    picker.mediaTypes = @[@"public.image"];
    picker.delegate = self;
    picker.modalPresentationStyle = UIModalPresentationFullScreen;
    [host presentViewController:picker animated:YES completion:nil];
  });
}

- (void)finish:(UIImagePickerController *)picker value:(NSString *)value {
  RCTPromiseResolveBlock resolve = _resolve;
  RCTPromiseRejectBlock reject = _reject;
  _resolve = nil;
  _reject = nil;
  [picker dismissViewControllerAnimated:YES completion:^{
    if (value) { if (resolve) resolve(value); }
    else if (reject) reject(@"SHOP_CAMERA", @"Could not read a single QR from the camera image", nil);
  }];
}

- (void)imagePickerControllerDidCancel:(UIImagePickerController *)picker {
  [self finish:picker value:@""];
}

- (void)imagePickerController:(UIImagePickerController *)picker
        didFinishPickingMediaWithInfo:(NSDictionary<UIImagePickerControllerInfoKey,id> *)info {
  UIImage *photo = info[UIImagePickerControllerOriginalImage];
  NSString *value = nil;
  if (photo.CGImage) {
    CIImage *image = [CIImage imageWithCGImage:photo.CGImage];
    CIDetector *detector = [CIDetector detectorOfType:CIDetectorTypeQRCode context:nil
        options:@{CIDetectorAccuracy: CIDetectorAccuracyHigh}];
    NSArray<CIFeature *> *codes = [detector featuresInImage:image];
    if (codes.count == 1 && [codes[0] isKindOfClass:CIQRCodeFeature.class]) {
      value = ((CIQRCodeFeature *)codes[0]).messageString;
      // 208 is the operator's answer to a reset request: prefix 15, nonce 64, colon, signature 128.
      if (value.length > ([value hasPrefix:@"wc:"] ? 1200 : [value hasPrefix:@"NU54-RESET1-OK:"] ? 208 : 96)) value = nil;
    }
  }
  [self finish:picker value:value];
}
@end
