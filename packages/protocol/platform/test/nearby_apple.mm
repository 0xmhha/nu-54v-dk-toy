#import <Foundation/Foundation.h>
#define NU54_MOBILE_LAB 1
#include <NearbyBle.h>

static void require(BOOL condition) {
  if (!condition) { fputs("Apple Nearby fixture assertion failed\n",stderr); exit(1); }
}

static NSString *pinFor(SecCertificateRef certificate) {
  id key=CFBridgingRelease(SecCertificateCopyKey(certificate));
  return fingerprint(spki((__bridge SecKeyRef)key));
}

static NearbyEnrollment *profile(NSURL *dir, NSString *name, NSString *remote, BOOL good) {
  NSData *ca=[NSData dataWithContentsOfURL:[dir URLByAppendingPathComponent:@"ca.der"]];
  NSData *local=[NSData dataWithContentsOfURL:[dir URLByAppendingPathComponent:[name stringByAppendingString:@".der"]]];
  NSData *secret=[NSData dataWithContentsOfURL:[dir URLByAppendingPathComponent:[name stringByAppendingString:@".raw"]]];
  NSData *peer=[NSData dataWithContentsOfURL:[dir URLByAppendingPathComponent:[remote stringByAppendingString:@".der"]]];
  id certificate=CFBridgingRelease(SecCertificateCreateWithData(nullptr,(__bridge CFDataRef)local));
  id peerCertificate=CFBridgingRelease(SecCertificateCreateWithData(nullptr,(__bridge CFDataRef)peer));
  NSDictionary *params=@{(__bridge id)kSecAttrKeyType:(__bridge id)kSecAttrKeyTypeECSECPrimeRandom,
     (__bridge id)kSecAttrKeyClass:(__bridge id)kSecAttrKeyClassPrivate,(__bridge id)kSecAttrKeySizeInBits:@256};
  id key=CFBridgingRelease(SecKeyCreateWithData((__bridge CFDataRef)secret,(__bridge CFDictionaryRef)params,nullptr));
  require(certificate && key && peerCertificate);
  id identity=CFBridgingRelease(SecIdentityCreate(nullptr,(__bridge SecCertificateRef)certificate,(__bridge SecKeyRef)key));
  require(identity!=nil);
  NearbyEnrollment *enrollment=[NearbyEnrollment new];
  enrollment.identity=sec_identity_create_with_certificates((__bridge SecIdentityRef)identity,(__bridge CFArrayRef)@[]);
  enrollment.ca=CFBridgingRelease(SecCertificateCreateWithData(nullptr,(__bridge CFDataRef)ca));
  enrollment.expiry=now()+120;
  NSString *pin=good ? pinFor((__bridge SecCertificateRef)peerCertificate) : [@"" stringByPaddingToLength:64 withString:@"1" startingAtIndex:0];
  enrollment.peers=@{pin:@{@"peer":pin,@"principal":remote,@"branch":@"lab",@"expiresAt":@(enrollment.expiry)}};
  return enrollment;
}

static void relay(NSData *packet, NearbyPipe *remote, NSUInteger width) {
  for (NSUInteger i=0;i<packet.length;i+=width)
    require([remote feed:[packet subdataWithRange:NSMakeRange(i,MIN(width,packet.length-i))]]);
}

static void listenToBoard(nw_connection_t socket, NearbyPipe *client) {
  nw_connection_receive(socket,1,8192,^(dispatch_data_t content,nw_content_context_t context,bool done,nw_error_t failure){
    if (content && dispatch_data_get_size(content)) {
      const void *mapping=nullptr; size_t length=0;
      dispatch_data_t mapped=dispatch_data_create_map(content,&mapping,&length);
      relay([NSData dataWithBytes:mapping length:length],client,137); (void)mapped;
    }
    if (!done && !failure) listenToBoard(socket,client);
  });
}

static void payment(NSURL *directory, const char *port, uint64_t expires, const char *resultPath) {
  static NearbyPipe *client;
  static nw_connection_t socket;
  NSData *quote=[NSData dataWithContentsOfURL:[directory URLByAppendingPathComponent:@"quote.bin"]];
  require(quote.length==737);
  client=[NearbyPipe new]; client.payment=YES;
  nw_parameters_t raw=nw_parameters_create_secure_tcp(NW_PARAMETERS_DISABLE_PROTOCOL,NW_PARAMETERS_DEFAULT_CONFIGURATION);
  socket=nw_connection_create(nw_endpoint_create_host("127.0.0.1",port),raw);
  nw_connection_set_queue(socket,dispatch_get_main_queue());
  static NSData *bound;
  static NSMutableData *pending,*body;
  static NSUInteger index=0,parts=0;
  pending=[NSMutableData new]; body=[NSMutableData new];
  const auto writeData=^(NSData *bytes){
    dispatch_data_t buffer=dispatch_data_create(bytes.bytes,bytes.length,dispatch_get_main_queue(),^{ (void)bytes; });
    nw_connection_send(socket,buffer,NW_CONNECTION_DEFAULT_MESSAGE_CONTEXT,true,^(nw_error_t error){ require(!error); });
  };
  client.encrypted=writeData;
  client.failed=^{ require(NO); };
  client.ready=^(NSData *epoch,NSDictionary *peer){
    require([peer[@"principal"] isEqual:@"board"]);
    bound=epoch;
    uint8_t header[25]={2};
    memset(header+1,7,16);
    for (NSUInteger at=0;at<8;at++) header[17+at]=(uint8_t)(expires>>(56-8*at));
    NSMutableData *request=[NSMutableData dataWithBytes:header length:sizeof(header)];
    [request appendData:quote];
    NSMutableData *stream=[NSMutableData new];
    NSUInteger count=(request.length+191)/192;
    for (NSUInteger part=0;part<count;part++) {
      NSData *piece=[request subdataWithRange:NSMakeRange(part*192,MIN(192,request.length-part*192))];
      uint8_t prefix[24]={0,(uint8_t)(22+piece.length)};
      memcpy(prefix+2,epoch.bytes,16);
      prefix[22]=(uint8_t)part;
      prefix[23]=(uint8_t)count;
      [stream appendBytes:prefix length:sizeof(prefix)];
      [stream appendData:piece];
    }
    require([client write:stream]);
  };
  client.clear=^(NSData *chunk){
    [pending appendData:chunk];
    while (pending.length>=2) {
      const uint8_t *wire=(const uint8_t *)pending.bytes;
      NSUInteger length=((NSUInteger)wire[0]<<8)|wire[1];
      require(length>=23 && length<=214);
      if (pending.length<length+2) return;
      require(!memcmp(wire+2,bound.bytes,16) && !wire[18] && !wire[19] && !wire[20] && !wire[21]);
      if (!index) parts=wire[23];
      require(wire[22]==index++ && wire[23]==parts);
      [body appendBytes:wire+24 length:length-22];
      [pending replaceBytesInRange:NSMakeRange(0,length+2) withBytes:nullptr length:0];
      if (index==parts) {
        const uint8_t *reply=(const uint8_t *)body.bytes;
        require(body.length>25 && reply[0]==5);
        for (NSUInteger k=1;k<17;k++) require(reply[k]==7);
        uint64_t expiry=0;
        for (NSUInteger k=17;k<25;k++) expiry=(expiry<<8)|reply[k];
        require(expiry==expires);
        NSData *payload=[body subdataWithRange:NSMakeRange(25,body.length-25)];
        require([payload writeToFile:[NSString stringWithUTF8String:resultPath] atomically:YES]);
        exit(0);
      }
    }
  };
  nw_connection_set_state_changed_handler(socket,^(nw_connection_state_t state,nw_error_t failure){
    require(state!=nw_connection_state_failed);
    if (state==nw_connection_state_ready) {
      listenToBoard(socket,client);
      [client open:profile(directory,@"payment",@"board",YES)];
    }
  });
  nw_connection_start(socket);
  dispatch_after(dispatch_time(DISPATCH_TIME_NOW,15*NSEC_PER_SEC),dispatch_get_main_queue(),^{ require(NO); });
  dispatch_main();
}

int main(int argc, const char **argv) {
  @autoreleasepool {
    require(argc==2 || argc==5);
    NSURL *directory=[NSURL fileURLWithPath:[NSString stringWithUTF8String:argv[1]]];
    if (argc==5) payment(directory,argv[2],strtoull(argv[3],nullptr,10),argv[4]);
    static NearbyPipe *clients[7], *servers[7];
    static NSData *clientEpochs[2], *serverEpochs[2];
    static NSMutableData *received[2], *expected[2];
    static BOOL valid[2], rejected[7], forgeNext;
    static NSUInteger acknowledged;
    for (NSUInteger i=0;i<7;i++) {
      NSString *visitor=i==1?@"customer2":@"customer";
      clients[i]=[NearbyPipe new]; servers[i]=[NearbyPipe new]; servers[i].server=YES;
      clients[i].encrypted=^(NSData *value){
        if (i==6 && forgeNext) {
          forgeNext=NO;
          NSMutableData *altered=[value mutableCopy];
          ((uint8_t *)altered.mutableBytes)[altered.length-1]^=1;
          relay(altered,servers[i],20);
        } else relay(value,servers[i],20);
      };
      servers[i].encrypted=^(NSData *value){ relay(value,clients[i],244); };
      clients[i].failed=^{ if (i>=2) { rejected[i]=YES; [clients[i] stop]; [servers[i] stop]; } else require(NO); };
      servers[i].failed=^{ if (i>=2) { rejected[i]=YES; [clients[i] stop]; [servers[i] stop]; } else require(NO); };
      if (i<2) {
        received[i]=[NSMutableData new];
        expected[i]=[NSMutableData new];
        for (NSUInteger sample=0;sample<600;sample++) {
          uint8_t frame[10]={0,8,(uint8_t)i,(uint8_t)(sample>>8),(uint8_t)sample,17,18,19,20,21};
          [expected[i] appendBytes:frame length:sizeof(frame)];
        }
        clients[i].ready=^(NSData *epoch,NSDictionary *peer){
          require([peer[@"principal"] isEqual:@"terminal"]);
          clientEpochs[i]=epoch;
          // One bounded write at a time tests streaming through independent connection queues.
          for (NSUInteger off=0;off<expected[i].length;off+=1000)
            require([clients[i] write:[expected[i] subdataWithRange:NSMakeRange(off,MIN(1000,expected[i].length-off))]]);
        };
        servers[i].ready=^(NSData *epoch,NSDictionary *peer){
          require([peer[@"principal"] isEqual:visitor]);
          serverEpochs[i]=epoch;
        };
        servers[i].clear=^(NSData *data){
          [received[i] appendData:data];
          require(received[i].length<=expected[i].length);
          if (received[i].length==expected[i].length) {
            require([received[i] isEqualToData:expected[i]] && [clientEpochs[i] isEqualToData:serverEpochs[i]]);
            valid[i]=YES;
            acknowledged++;
          }
        };
      }
      NSString *serverName=i==4?@"customer2":@"terminal";
      NearbyEnrollment *client=profile(directory,visitor,serverName,i!=2);
      if (i==3) {
        NSString *pin=client.peers.allKeys.firstObject;
        NSMutableDictionary *expired=[client.peers[pin] mutableCopy];
        expired[@"expiresAt"]=@(0);
        client.peers=@{pin:expired};
      }
      if (i==5) clients[i].payment=YES;
      if (i==6) clients[i].ready=^(NSData *epoch,NSDictionary *peer){
        forgeNext=YES;
        require([clients[i] write:[NSData dataWithBytes:"\0\1x" length:3]]);
      };
      [servers[i] open:profile(directory,serverName,visitor,YES)];
      [clients[i] open:client];
    }
    dispatch_source_t timer=dispatch_source_create(DISPATCH_SOURCE_TYPE_TIMER,0,0,dispatch_get_main_queue());
    dispatch_source_set_timer(timer,dispatch_time(DISPATCH_TIME_NOW,100*NSEC_PER_MSEC),100*NSEC_PER_MSEC,0);
    dispatch_source_set_event_handler(timer,^{
      if (acknowledged==2 && valid[0] && valid[1] && rejected[2] && rejected[3] && rejected[4] && rejected[5] && rejected[6]) {
        require(![clientEpochs[0] isEqualToData:clientEpochs[1]]);
        puts("Apple Network/Security TLS 1.3: two independent epochs/visitors, 600 frames each, wrong pin, expired enrollment, wrong role/ALPN and corrupted record rejected");
        exit(0);
      }
    });
    dispatch_resume(timer);
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW,15*NSEC_PER_SEC),dispatch_get_main_queue(),^{ require(NO); });
    dispatch_main();
  }
}
