//go:build darwin && cgo

#import <AppKit/AppKit.h>
#import <QuickLookThumbnailing/QuickLookThumbnailing.h>
#include <stdlib.h>
#include <string.h>

int dgs_file_thumbnail(const char *path, unsigned char **out, long *length) {
 @autoreleasepool {
  NSString *name=[NSString stringWithUTF8String:path];
  if(!name || ![[NSFileManager defaultManager] fileExistsAtPath:name])return 1;
  QLThumbnailGenerationRequest *request=[[QLThumbnailGenerationRequest alloc]
   initWithFileAtURL:[NSURL fileURLWithPath:name] size:CGSizeMake(800,800)
   scale:1 representationTypes:QLThumbnailGenerationRequestRepresentationTypeThumbnail];
  dispatch_semaphore_t done=dispatch_semaphore_create(0);
  __block NSData *data=nil;
  [[QLThumbnailGenerator sharedGenerator] generateBestRepresentationForRequest:request
   completionHandler:^(QLThumbnailRepresentation *thumbnail,NSError *error){
    @autoreleasepool {
     if(thumbnail){
      NSBitmapImageRep *bitmap=[[NSBitmapImageRep alloc] initWithCGImage:thumbnail.CGImage];
      data=[bitmap representationUsingType:NSBitmapImageFileTypeJPEG properties:@{NSImageCompressionFactor:@0.85}];
     }
     dispatch_semaphore_signal(done);
    }
   }];
  if(dispatch_semaphore_wait(done,dispatch_time(DISPATCH_TIME_NOW,30*NSEC_PER_SEC))!=0){
   [[QLThumbnailGenerator sharedGenerator] cancelRequest:request];return 1;
  }
  if(!data)return 1;
  *length=(long)data.length;*out=malloc((size_t)*length);
  if(!*out)return 1;
  memcpy(*out,data.bytes,(size_t)*length);return 0;
 }
}
