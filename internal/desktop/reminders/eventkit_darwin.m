//go:build darwin && cgo

// The EventKit side of package reminders. Every answer is one JSON object; a
// failure is {"error": "..."}, so dgs reports the reason.
#import <CoreLocation/CoreLocation.h>
#import <EventKit/EventKit.h>
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

static NSString *const failureDomain = @"dgs.reminders";

static NSError *failure(NSString *message) {
    return [NSError errorWithDomain:failureDomain code:1 userInfo:@{NSLocalizedDescriptionKey: message}];
}

static char *answer(NSDictionary *object) {
    NSData *data = [NSJSONSerialization dataWithJSONObject:object options:NSJSONWritingSortedKeys error:nil];
    char *out = malloc(data.length + 1);
    memcpy(out, data.bytes, data.length);
    out[data.length] = 0;
    return out;
}

static BOOL requestAccess(EKEventStore *store, NSError **error) {
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    __block BOOL granted = NO;
    __block NSError *failed = nil;
    void (^completion)(BOOL, NSError *) = ^(BOOL ok, NSError *err) {
        granted = ok;
        failed = err;
        dispatch_semaphore_signal(done);
    };
    if (@available(macOS 14.0, *)) {
        [store requestFullAccessToRemindersWithCompletion:completion];
    } else {
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"
        [store requestAccessToEntityType:EKEntityTypeReminder completion:completion];
#pragma clang diagnostic pop
    }
    dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
    if (failed) {
        *error = failure([@"reminders access: " stringByAppendingString:failed.localizedDescription]);
        return NO;
    }
    if (!granted) {
        *error = failure(@"reminders access was denied; allow it in System Settings > Privacy & Security > Reminders");
        return NO;
    }
    return YES;
}

static NSString *string(NSDictionary *object, NSString *key) {
    id value = object[key];
    return [value isKindOfClass:[NSString class]] && [value length] > 0 ? value : nil;
}

static EKCalendar *calendarNamed(EKEventStore *store, NSString *list, NSError **error) {
    if (!list) {
        EKCalendar *calendar = [store defaultCalendarForNewReminders];
        if (!calendar) *error = failure(@"there is no default reminders list");
        return calendar;
    }
    NSPredicate *titled = [NSPredicate predicateWithFormat:@"title == %@", list];
    NSArray<EKCalendar *> *matches = [[store calendarsForEntityType:EKEntityTypeReminder] filteredArrayUsingPredicate:titled];
    if (matches.count == 0) {
        *error = failure([NSString stringWithFormat:@"no reminders list is called %@", list]);
        return nil;
    }
    if (matches.count > 1) {
        *error = failure([NSString stringWithFormat:@"more than one reminders list is called %@", list]);
        return nil;
    }
    return matches.firstObject;
}

static EKReminder *existing(EKEventStore *store, EKCalendar *calendar, NSString *mark) {
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    __block EKReminder *found = nil;
    NSPredicate *predicate = [store predicateForRemindersInCalendars:@[calendar]];
    [store fetchRemindersMatchingPredicate:predicate completion:^(NSArray<EKReminder *> *reminders) {
        for (EKReminder *reminder in reminders) {
            if ([reminder.notes ?: @"" containsString:mark]) {
                found = reminder;
                break;
            }
        }
        dispatch_semaphore_signal(done);
    }];
    dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
    return found;
}

static NSDate *parseDate(NSString *value) {
    NSISO8601DateFormatter *formatter = [NSISO8601DateFormatter new];
    NSDate *date = [formatter dateFromString:value];
    if (date) return date;
    formatter.formatOptions = NSISO8601DateFormatWithInternetDateTime | NSISO8601DateFormatWithFractionalSeconds;
    return [formatter dateFromString:value];
}

static NSDictionary *create(NSDictionary *request, NSError **error) {
    NSString *title = string(request, @"title");
    if (![title stringByTrimmingCharactersInSet:NSCharacterSet.whitespaceAndNewlineCharacterSet].length) {
        *error = failure(@"a reminder needs a title");
        return nil;
    }
    EKEventStore *store = [EKEventStore new];
    if (!requestAccess(store, error)) return nil;
    EKCalendar *target = calendarNamed(store, string(request, @"list"), error);
    if (!target) return nil;

    NSString *mark = string(request, @"mark");
    if (mark) {
        EKReminder *found = existing(store, target, mark);
        if (found) {
            return @{@"id": found.calendarItemIdentifier,
                     @"skipped": [NSString stringWithFormat:@"this capture is already a reminder in %@, as %@", target.title, mark]};
        }
    }

    EKReminder *reminder = [EKReminder reminderWithEventStore:store];
    reminder.calendar = target;
    reminder.title = title;
    NSString *notes = string(request, @"notes") ?: @"";
    if (mark) notes = notes.length ? [NSString stringWithFormat:@"%@\n\n%@", notes, mark] : mark;
    reminder.notes = notes.length ? notes : nil;

    NSString *due = string(request, @"due");
    if (due) {
        NSDate *date = parseDate(due);
        if (!date) {
            *error = failure([NSString stringWithFormat:@"due %@ is not an RFC 3339 timestamp", due]);
            return nil;
        }
        NSCalendar *calendar = NSCalendar.currentCalendar;
        NSCalendarUnit units = NSCalendarUnitYear | NSCalendarUnitMonth | NSCalendarUnitDay |
            NSCalendarUnitHour | NSCalendarUnitMinute | NSCalendarUnitSecond | NSCalendarUnitTimeZone;
        NSDateComponents *components = [calendar components:units fromDate:date];
        components.calendar = calendar;
        reminder.dueDateComponents = components;
        [reminder addAlarm:[EKAlarm alarmWithAbsoluteDate:date]];
    }

    NSDictionary *location = request[@"location"];
    if ([location isKindOfClass:[NSDictionary class]]) {
        EKStructuredLocation *structured = [EKStructuredLocation locationWithTitle:string(location, @"title") ?: @""];
        structured.geoLocation = [[CLLocation alloc] initWithLatitude:[location[@"latitude"] doubleValue]
                                                            longitude:[location[@"longitude"] doubleValue]];
        if (location[@"radius"]) structured.radius = [location[@"radius"] doubleValue];
        EKAlarm *alarm = [EKAlarm new];
        alarm.structuredLocation = structured;
        NSString *proximity = string(location, @"proximity");
        if ([proximity isEqualToString:@"arrive"]) {
            alarm.proximity = EKAlarmProximityEnter;
        } else if ([proximity isEqualToString:@"leave"]) {
            alarm.proximity = EKAlarmProximityLeave;
        } else {
            *error = failure([NSString stringWithFormat:@"proximity %@ is neither arrive nor leave", proximity]);
            return nil;
        }
        [reminder addAlarm:alarm];
    }

    NSError *saveError = nil;
    if (![store saveReminder:reminder commit:YES error:&saveError]) {
        *error = failure([@"save the reminder: " stringByAppendingString:saveError.localizedDescription]);
        return nil;
    }
    return @{@"id": reminder.calendarItemIdentifier, @"list": target.title};
}

static NSDictionary *lists(NSError **error) {
    EKEventStore *store = [EKEventStore new];
    if (!requestAccess(store, error)) return nil;
    NSMutableArray *titles = [NSMutableArray array];
    for (EKCalendar *calendar in [store calendarsForEntityType:EKEntityTypeReminder]) {
        [titles addObject:calendar.title];
    }
    return @{@"lists": titles, @"default": store.defaultCalendarForNewReminders.title ?: @""};
}

char *dgs_reminders_call(const char *op, const char *input, int length) {
    @autoreleasepool {
        NSError *error = nil;
        NSDictionary *result = nil;
        if (strcmp(op, "create") == 0) {
            NSData *data = [NSData dataWithBytes:input length:length];
            id request = [NSJSONSerialization JSONObjectWithData:data options:0 error:&error];
            if (![request isKindOfClass:[NSDictionary class]]) {
                return answer(@{@"error": @"the request is not a JSON object"});
            }
            result = create(request, &error);
        } else if (strcmp(op, "lists") == 0) {
            result = lists(&error);
        } else {
            error = failure([NSString stringWithFormat:@"unknown operation %s", op]);
        }
        if (!result) return answer(@{@"error": error.localizedDescription ?: @"unknown failure"});
        return answer(result);
    }
}
