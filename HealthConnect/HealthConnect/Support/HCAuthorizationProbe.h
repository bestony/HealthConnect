@import Foundation;
@import HealthKit;

NS_ASSUME_NONNULL_BEGIN

typedef void (^HCAuthorizationProbeHandler)(NSInteger status,
                                            NSString * _Nullable exceptionReason,
                                            NSString * _Nullable errorDomain,
                                            NSInteger errorCode);

typedef void (^HCEarliestDateHandler)(NSDate * _Nullable earliest,
                                      NSString * _Nullable exceptionReason,
                                      NSString * _Nullable errorDomain,
                                      NSInteger errorCode);

/// HealthKit throws NSException for a read type it refuses to authorize.
/// These calls catch that exception and return it as exceptionReason.
void HCProbeReadAuthorizationStatus(HKHealthStore *store,
                                    NSSet<HKObjectType *> *readTypes,
                                    HCAuthorizationProbeHandler handler);

void HCProbeRequestReadAuthorization(HKHealthStore *store,
                                     NSSet<HKObjectType *> *readTypes,
                                     HCAuthorizationProbeHandler handler);

void HCProbeEarliestReadDate(HKHealthStore *store,
                             NSSet<HKObjectType *> *readTypes,
                             HCEarliestDateHandler handler);

NS_ASSUME_NONNULL_END
