#import "HCAuthorizationProbe.h"

static void HCFinishStatus(HCAuthorizationProbeHandler handler,
                           NSInteger status,
                           NSString *reason,
                           NSError *error) {
    handler(status,
            reason,
            error != nil ? error.domain : nil,
            error != nil ? (NSInteger)error.code : 0);
}

void HCProbeReadAuthorizationStatus(HKHealthStore *store,
                                    NSSet<HKObjectType *> *readTypes,
                                    HCAuthorizationProbeHandler handler) {
    @try {
        NSSet<HKSampleType *> *shareTypes = [NSSet set];
        [store getRequestStatusForAuthorizationToShareTypes:shareTypes
                                                   readTypes:readTypes
                                                  completion:^(HKAuthorizationRequestStatus requestStatus, NSError *error) {
            HCFinishStatus(handler, (NSInteger)requestStatus, nil, error);
        }];
    } @catch (NSException *exception) {
        HCFinishStatus(handler, -1, exception.reason ?: exception.name, nil);
    }
}

void HCProbeRequestReadAuthorization(HKHealthStore *store,
                                     NSSet<HKObjectType *> *readTypes,
                                     HCAuthorizationProbeHandler handler) {
    @try {
        NSSet<HKSampleType *> *shareTypes = [NSSet set];
        [store requestAuthorizationToShareTypes:shareTypes
                                      readTypes:readTypes
                                     completion:^(BOOL success, NSError *error) {
            HCFinishStatus(handler, success ? 1 : 0, nil, error);
        }];
    } @catch (NSException *exception) {
        HCFinishStatus(handler, -1, exception.reason ?: exception.name, nil);
    }
}

void HCProbeEarliestReadDate(HKHealthStore *store,
                             NSSet<HKObjectType *> *readTypes,
                             HCEarliestDateHandler handler) {
    @try {
        [store getEarliestAuthorizedSampleDateForTypes:readTypes
                                            completion:^(NSDictionary<HKObjectType *, NSDate *> *earliestDates, NSError *error) {
            NSDate *earliest = nil;
            for (NSDate *date in earliestDates.allValues) {
                if (earliest == nil || [date compare:earliest] == NSOrderedAscending) {
                    earliest = date;
                }
            }
            handler(earliest,
                    nil,
                    error != nil ? error.domain : nil,
                    error != nil ? (NSInteger)error.code : 0);
        }];
    } @catch (NSException *exception) {
        handler(nil, exception.reason ?: exception.name, nil, 0);
    }
}

BOOL HCObserverQuerySupported(HKSampleType *type) {
    if (type == nil) {
        return NO;
    }
    @try {
        // Init validates the type and raises NSException for types that
        // cannot be observed. The query is not executed.
        HKObserverQuery *query = [[HKObserverQuery alloc] initWithSampleType:type
                                                                    predicate:nil
                                                                updateHandler:^(HKObserverQuery *unusedQuery,
                                                                                HKObserverQueryCompletionHandler completion,
                                                                                NSError *unusedError) {
            if (completion != nil) {
                completion();
            }
        }];
        return query != nil;
    } @catch (NSException *exception) {
        return NO;
    }
}
