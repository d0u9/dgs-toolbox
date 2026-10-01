//go:build darwin && cgo
#import <Foundation/Foundation.h>
#import <CoreLocation/CoreLocation.h>
#import <Contacts/Contacts.h>
#include <stdlib.h>
#include <string.h>
@interface DGSLocation : NSObject <CLLocationManagerDelegate>
@property CLLocationManager *manager;
@property CLGeocoder *geocoder;
@property NSMutableArray *diagnostics;
@property NSDictionary *values;
@property NSString *failure;
@property BOOL lookup;
@property BOOL received;
@property BOOL done;
@end
@implementation DGSLocation
- (void)finish:(CLLocation *)location placemark:(CLPlacemark *)p {
 id null=[NSNull null];
 NSDateFormatter *formatter=[NSDateFormatter new];
 formatter.locale=[[NSLocale alloc] initWithLocaleIdentifier:@"en_US_POSIX"];
 formatter.dateFormat=@"yyyy-MM-dd HH:mm:ss Z";
 formatter.timeZone=[NSTimeZone timeZoneForSecondsFromGMT:0];
 NSString *time=[formatter stringFromDate:location.timestamp];
 formatter.timeZone=p.timeZone;
 NSString *local=p.timeZone ? [formatter stringFromDate:location.timestamp] : nil;
 NSString *address=p.postalAddress ? [CNPostalAddressFormatter stringFromPostalAddress:p.postalAddress style:CNPostalAddressFormatterStyleMailingAddress] : nil;
 self.values=@{@"latitude":[NSString stringWithFormat:@"%.6f",location.coordinate.latitude],@"longitude":[NSString stringWithFormat:@"%.6f",location.coordinate.longitude],@"altitude":[NSString stringWithFormat:@"%.2f",location.altitude],@"direction":[NSString stringWithFormat:@"%@",@(location.course)],@"speed":[NSString stringWithFormat:@"%ld",(long)location.speed],@"h_accuracy":[NSString stringWithFormat:@"%ld",(long)location.horizontalAccuracy],@"v_accuracy":[NSString stringWithFormat:@"%ld",(long)location.verticalAccuracy],@"time":time,@"address":address?:null,@"name":p.name?:null,@"isoCountryCode":p.ISOcountryCode?:null,@"country":p.country?:null,@"postalCode":p.postalCode?:null,@"administrativeArea":p.administrativeArea?:null,@"subAdministrativeArea":p.subAdministrativeArea?:null,@"locality":p.locality?:null,@"subLocality":p.subLocality?:null,@"thoroughfare":p.thoroughfare?:null,@"subThoroughfare":p.subThoroughfare?:null,@"region":p.region.identifier?:null,@"timeZone":p.timeZone.name?:null,@"time_local":local?:null};
 self.done=YES;
}
- (void)locationManagerDidChangeAuthorization:(CLLocationManager *)manager {
 [self.diagnostics addObject:[NSString stringWithFormat:@"Location authorization status: %d",manager.authorizationStatus]];
 if(manager.authorizationStatus==kCLAuthorizationStatusAuthorizedAlways){[manager startUpdatingLocation];}
 else if(manager.authorizationStatus==kCLAuthorizationStatusDenied || manager.authorizationStatus==kCLAuthorizationStatusRestricted){self.failure=@"Location access denied; allow dgs in System Settings > Privacy & Security > Location Services";self.done=YES;}
}
- (void)locationManager:(CLLocationManager *)manager didUpdateLocations:(NSArray<CLLocation *> *)locations {
 if(self.received || self.done || locations.count==0)return;
 self.received=YES;
 [manager stopUpdatingLocation];
 CLLocation *location=locations.firstObject;
 if(!self.lookup){[self finish:location placemark:nil];return;}
 [self.geocoder reverseGeocodeLocation:location completionHandler:^(NSArray<CLPlacemark *> *places,NSError *error){
 if(self.done)return;
 if(error){self.failure=[@"Reverse geocode failed: " stringByAppendingString:error.localizedDescription];self.done=YES;return;}
 [self finish:location placemark:places.firstObject];
 }];
}
- (void)locationManager:(CLLocationManager *)manager didFailWithError:(NSError *)error {
 self.failure=error.code==kCLErrorDenied ? @"Location services are disabled or location access denied; allow dgs in System Settings > Privacy & Security > Location Services" : error.localizedDescription;
 self.done=YES;
}
@end
char *dgs_location_get(int placemark,int timeout){
 @autoreleasepool {
 DGSLocation *delegate=[DGSLocation new];
 delegate.lookup=placemark!=0;
 delegate.diagnostics=[NSMutableArray new];
 delegate.manager=[CLLocationManager new];
 delegate.geocoder=[CLGeocoder new];
 delegate.manager.delegate=delegate;
 delegate.manager.desiredAccuracy=kCLLocationAccuracyBest;
 delegate.manager.distanceFilter=2.0;
 [delegate.diagnostics addObject:[NSString stringWithFormat:@"locationServicesEnabled: %@",[CLLocationManager locationServicesEnabled]?@"true":@"false"]];
 [delegate.diagnostics addObject:[NSString stringWithFormat:@"significantLocationChangeMonitoringAvailable: %@",[CLLocationManager significantLocationChangeMonitoringAvailable]?@"true":@"false"]];
 [delegate.diagnostics addObject:[NSString stringWithFormat:@"headingAvailable: %@",[CLLocationManager headingAvailable]?@"true":@"false"]];
 [delegate.diagnostics addObject:[NSString stringWithFormat:@"regionMonitoringAvailable for CLRegion: %@",[CLLocationManager isMonitoringAvailableForClass:[CLRegion class]]?@"true":@"false"]];
 if(delegate.manager.authorizationStatus==kCLAuthorizationStatusNotDetermined){
 [delegate.manager requestWhenInUseAuthorization];
 } else if(delegate.manager.authorizationStatus==kCLAuthorizationStatusAuthorizedAlways){
 [delegate.manager startUpdatingLocation];
 }
 NSDate *deadline=[NSDate dateWithTimeIntervalSinceNow:timeout];
 while(!delegate.done && deadline.timeIntervalSinceNow>0){
 [[NSRunLoop currentRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.05]];
 }
 if(!delegate.done){delegate.failure=delegate.received ? @"Reverse geocode timed out" : @"Fetching location timed out";delegate.done=YES;}
 [delegate.manager stopUpdatingLocation];
 [delegate.geocoder cancelGeocode];
 delegate.manager.delegate=nil;
 NSDictionary *result=@{@"values":delegate.values?:@{},@"diagnostics":delegate.diagnostics,@"error":delegate.failure?:@""};
 NSData *data=[NSJSONSerialization dataWithJSONObject:result options:0 error:nil];
 char *out=malloc(data.length+1);memcpy(out,data.bytes,data.length);out[data.length]=0;return out;
 }
}
