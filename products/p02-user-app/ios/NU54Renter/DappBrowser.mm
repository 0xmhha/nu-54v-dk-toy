#import <React/RCTBridgeModule.h>
#import <React/RCTUtils.h>
#import <NU54RenterSpec/NU54RenterSpec.h>
#import <UIKit/UIKit.h>
#import <WebKit/WebKit.h>

static NSString *httpsOrigin(NSURL *url) {
  NSURLComponents *parts=[NSURLComponents componentsWithURL:url resolvingAgainstBaseURL:NO];
  if (![parts.scheme isEqualToString:@"https"] || !parts.host.length || parts.user || parts.password ||
      [parts.host containsString:@":"]) return nil;
  NSString *host=parts.host.lowercaseString;
  return parts.port && parts.port.integerValue!=443
    ? [NSString stringWithFormat:@"https://%@:%@",host,parts.port] : [@"https://" stringByAppendingString:host];
}

@interface DappBrowser : NativeDappBrowserSpecBase <NativeDappBrowserSpec, WKScriptMessageHandler, WKNavigationDelegate>
@end

@implementation DappBrowser {
  UIViewController *_presentation;
  WKWebView *_view;
  NSString *_scope;
  NSString *_channel;
  NSString *_document;
}
RCT_EXPORT_MODULE(DappBrowser)
- (std::shared_ptr<facebook::react::TurboModule>)getTurboModule:
    (const facebook::react::ObjCTurboModule::InitParams &)params {
  return std::make_shared<facebook::react::NativeDappBrowserSpecJSI>(params);
}

- (void)reset {
  [_view.configuration.userContentController removeScriptMessageHandlerForName:@"nu54Wallet"];
  [_view stopLoading];
  _view.navigationDelegate=nil;
  _view=nil;
  [_presentation dismissViewControllerAnimated:YES completion:nil];
  _presentation=nil;
  if (_channel.length) [self emitOnClosed:_channel];
  _channel=nil; _scope=nil; _document=nil;
}

- (void)closeTapped { [self reset]; }

- (void)open:(NSString *)url session:(NSString *)session provider:(NSString *)provider
     resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{
    NSURL *address=[NSURL URLWithString:url];
    NSString *origin=httpsOrigin(address);
    UIViewController *presenter=RCTPresentedViewController();
    NSCharacterSet *invalid=[[NSCharacterSet characterSetWithCharactersInString:@"0123456789abcdef"] invertedSet];
    if (presenter==nil || !origin.length || session.length!=64 ||
        [session rangeOfCharacterFromSet:invalid].location!=NSNotFound || !provider.length || provider.length>8192 ||
        self->_presentation) {
      reject(@"DAPP_BROWSER",@"The site could not be opened",nil);
      return;
    }
    self->_scope=origin; self->_channel=session;
    WKWebViewConfiguration *config=[WKWebViewConfiguration new];
    config.websiteDataStore=[WKWebsiteDataStore nonPersistentDataStore];
    config.preferences.javaScriptCanOpenWindowsAutomatically=NO;
    [config.userContentController addScriptMessageHandler:self name:@"nu54Wallet"];
    WKUserScript *script=[[WKUserScript alloc] initWithSource:provider injectionTime:WKUserScriptInjectionTimeAtDocumentStart forMainFrameOnly:YES];
    [config.userContentController addUserScript:script];
    UIViewController *modal=[UIViewController new];
    modal.modalPresentationStyle=UIModalPresentationFullScreen;
    modal.view.backgroundColor=UIColor.systemBackgroundColor;
    WKWebView *browser=[[WKWebView alloc] initWithFrame:CGRectZero configuration:config];
    browser.navigationDelegate=self;
    UIButton *close=[UIButton buttonWithType:UIButtonTypeSystem];
    [close setTitle:[@"Close · " stringByAppendingString:origin] forState:UIControlStateNormal];
    [close addTarget:self action:@selector(closeTapped) forControlEvents:UIControlEventTouchUpInside];
    UIStackView *stack=[[UIStackView alloc] initWithArrangedSubviews:@[close,browser]];
    stack.axis=UILayoutConstraintAxisVertical;
    stack.translatesAutoresizingMaskIntoConstraints=NO;
    [modal.view addSubview:stack];
    UILayoutGuide *safe=modal.view.safeAreaLayoutGuide;
    [NSLayoutConstraint activateConstraints:@[[stack.leadingAnchor constraintEqualToAnchor:safe.leadingAnchor],
      [stack.trailingAnchor constraintEqualToAnchor:safe.trailingAnchor],[stack.topAnchor constraintEqualToAnchor:safe.topAnchor],
      [stack.bottomAnchor constraintEqualToAnchor:safe.bottomAnchor],[close.heightAnchor constraintGreaterThanOrEqualToConstant:44]]];
    self->_view=browser; self->_presentation=modal;
    [presenter presentViewController:modal animated:YES completion:nil];
    [browser loadRequest:[NSURLRequest requestWithURL:address]];
    resolve(nil);
  });
}

- (void)webView:(WKWebView *)webView decidePolicyForNavigationAction:(WKNavigationAction *)action
 decisionHandler:(void (^)(WKNavigationActionPolicy))decision {
  BOOL allowed=webView==_view && action.targetFrame!=nil && [httpsOrigin(action.request.URL) isEqualToString:_scope];
  if (webView==_view && action.targetFrame.isMainFrame && (!allowed || _document)) {
    decision(WKNavigationActionPolicyCancel);
    [self reset];
    return;
  }
  decision(allowed ? WKNavigationActionPolicyAllow : WKNavigationActionPolicyCancel);
}

- (void)webView:(WKWebView *)webView didCommitNavigation:(WKNavigation *)navigation {
  _document=[NSUUID UUID].UUIDString;
}

- (void)userContentController:(WKUserContentController *)controller didReceiveScriptMessage:(WKScriptMessage *)message {
  if (message.webView!=_view || !message.frameInfo.isMainFrame || !_document ||
      ![httpsOrigin(message.frameInfo.request.URL) isEqualToString:_scope] ||
      ![httpsOrigin(_view.URL) isEqualToString:_scope] || ![message.body isKindOfClass:NSString.class] ||
      [message.body length]<2 || [message.body length]>4096) return;
  NSData *packet=[NSJSONSerialization dataWithJSONObject:@{@"session":_channel,@"origin":_scope,@"document":_document,@"request":message.body}
                                                options:0 error:nullptr];
  if (packet) [self emitOnRequest:[[NSString alloc] initWithData:packet encoding:NSUTF8StringEncoding]];
}

- (void)respond:(NSString *)session document:(NSString *)document reply:(NSString *)reply
        resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{
    if (![self->_channel isEqualToString:session] || ![self->_document isEqualToString:document] ||
        ![httpsOrigin(self->_view.URL) isEqualToString:self->_scope] || reply.length<2 || reply.length>4096) {
      reject(@"DAPP_CHANNEL",@"The wallet channel is closed",nil); return;
    }
    [self->_view callAsyncJavaScript:@"window.nu54WalletReply(packet);" arguments:@{@"packet":reply}
                           inFrame:nil inContentWorld:WKContentWorld.pageWorld completionHandler:^(id result,NSError *error){
      if (error) reject(@"DAPP_CHANNEL",@"The wallet channel is closed",nil);
      else resolve(nil);
    }];
  });
}

- (void)confirm:(NSString *)session document:(NSString *)document title:(NSString *)title message:(NSString *)message
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{
    if (![self->_channel isEqualToString:session] || ![self->_document isEqualToString:document] ||
        !self->_presentation || title.length>120 || message.length>500) { resolve(@NO); return; }
    UIAlertController *review=[UIAlertController alertControllerWithTitle:title message:message
                                                                    preferredStyle:UIAlertControllerStyleAlert];
    [review addAction:[UIAlertAction actionWithTitle:@"Cancel" style:UIAlertActionStyleCancel handler:^(UIAlertAction *action){resolve(@NO);}]];
    [review addAction:[UIAlertAction actionWithTitle:@"Ask board" style:UIAlertActionStyleDefault handler:^(UIAlertAction *action){
      resolve(@([self->_channel isEqualToString:session] && [self->_document isEqualToString:document]));
    }]];
    [self->_presentation presentViewController:review animated:YES completion:nil];
  });
}

- (void)confirmAllowance:(NSString *)session document:(NSString *)document title:(NSString *)title message:(NSString *)message
    resolve:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{
    BOOL (^current)(void)=^BOOL {
      return [self->_channel isEqualToString:session] && [self->_document isEqualToString:document] &&
             [httpsOrigin(self->_view.URL) isEqualToString:self->_scope];
    };
    BOOL login = [title hasPrefix:@"사이트 로그인 · "];
    if (!current() || !self->_presentation || title.length>120 || message.length>(login ? 1000 : 500)) { resolve(@"reject"); return; }
    UIAlertController *review=[UIAlertController alertControllerWithTitle:title message:message
                                                                    preferredStyle:UIAlertControllerStyleAlert];
    [review addAction:[UIAlertAction actionWithTitle:@"거절" style:UIAlertActionStyleCancel handler:^(UIAlertAction *action){resolve(@"reject");}]];
    [review addAction:[UIAlertAction actionWithTitle:(login ? @"세션 보기" : @"연결 보기") style:UIAlertActionStyleDefault handler:^(UIAlertAction *action){
      resolve(current() ? @"connections" : @"reject");
    }]];
    [review addAction:[UIAlertAction actionWithTitle:@"보드 요청" style:UIAlertActionStyleDefault handler:^(UIAlertAction *action){
      resolve(current() ? @"approve" : @"reject");
    }]];
    [self->_presentation presentViewController:review animated:YES completion:nil];
  });
}

- (void)close:(RCTPromiseResolveBlock)resolve reject:(RCTPromiseRejectBlock)reject {
  dispatch_async(dispatch_get_main_queue(),^{ [self reset]; resolve(nil); });
}

- (void)invalidate {
  dispatch_async(dispatch_get_main_queue(),^{ [self reset]; });
}
@end
