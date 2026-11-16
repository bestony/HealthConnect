import Foundation
import HealthKit

/// Read catalog for this SDK.
///
/// Clinical record types are absent. Requesting them without the Health Records entitlement can fail the whole authorization request.
/// Workout route types are absent. The export does not include location traces.
enum HealthTypeCatalog {
    static let quantityIdentifiers: [String] = [
        "HKQuantityTypeIdentifierAppleSleepingWristTemperature",
        "HKQuantityTypeIdentifierBodyFatPercentage",
        "HKQuantityTypeIdentifierBodyMass",
        "HKQuantityTypeIdentifierBodyMassIndex",
        "HKQuantityTypeIdentifierElectrodermalActivity",
        "HKQuantityTypeIdentifierHeight",
        "HKQuantityTypeIdentifierLeanBodyMass",
        "HKQuantityTypeIdentifierWaistCircumference",
        "HKQuantityTypeIdentifierActiveEnergyBurned",
        "HKQuantityTypeIdentifierAppleExerciseTime",
        "HKQuantityTypeIdentifierAppleMoveTime",
        "HKQuantityTypeIdentifierAppleStandTime",
        "HKQuantityTypeIdentifierBasalEnergyBurned",
        "HKQuantityTypeIdentifierCrossCountrySkiingSpeed",
        "HKQuantityTypeIdentifierCyclingCadence",
        "HKQuantityTypeIdentifierCyclingFunctionalThresholdPower",
        "HKQuantityTypeIdentifierCyclingPower",
        "HKQuantityTypeIdentifierCyclingSpeed",
        "HKQuantityTypeIdentifierDistanceCrossCountrySkiing",
        "HKQuantityTypeIdentifierDistanceCycling",
        "HKQuantityTypeIdentifierDistanceDownhillSnowSports",
        "HKQuantityTypeIdentifierDistancePaddleSports",
        "HKQuantityTypeIdentifierDistanceRowing",
        "HKQuantityTypeIdentifierDistanceSkatingSports",
        "HKQuantityTypeIdentifierDistanceSwimming",
        "HKQuantityTypeIdentifierDistanceWalkingRunning",
        "HKQuantityTypeIdentifierDistanceWheelchair",
        "HKQuantityTypeIdentifierEstimatedWorkoutEffortScore",
        "HKQuantityTypeIdentifierFlightsClimbed",
        "HKQuantityTypeIdentifierNikeFuel",
        "HKQuantityTypeIdentifierPaddleSportsSpeed",
        "HKQuantityTypeIdentifierPhysicalEffort",
        "HKQuantityTypeIdentifierPushCount",
        "HKQuantityTypeIdentifierRowingSpeed",
        "HKQuantityTypeIdentifierRunningPower",
        "HKQuantityTypeIdentifierRunningSpeed",
        "HKQuantityTypeIdentifierStepCount",
        "HKQuantityTypeIdentifierSwimmingStrokeCount",
        "HKQuantityTypeIdentifierUnderwaterDepth",
        "HKQuantityTypeIdentifierWorkoutEffortScore",
        "HKQuantityTypeIdentifierEnvironmentalAudioExposure",
        "HKQuantityTypeIdentifierEnvironmentalSoundReduction",
        "HKQuantityTypeIdentifierHeadphoneAudioExposure",
        "HKQuantityTypeIdentifierAtrialFibrillationBurden",
        "HKQuantityTypeIdentifierHeartRate",
        "HKQuantityTypeIdentifierHeartRateRecoveryOneMinute",
        "HKQuantityTypeIdentifierHeartRateVariabilityRMSSD",
        "HKQuantityTypeIdentifierHeartRateVariabilitySDNN",
        "HKQuantityTypeIdentifierPeripheralPerfusionIndex",
        "HKQuantityTypeIdentifierRestingHeartRate",
        "HKQuantityTypeIdentifierVO2Max",
        "HKQuantityTypeIdentifierWalkingHeartRateAverage",
        "HKQuantityTypeIdentifierAppleWalkingSteadiness",
        "HKQuantityTypeIdentifierRunningGroundContactTime",
        "HKQuantityTypeIdentifierRunningStrideLength",
        "HKQuantityTypeIdentifierRunningVerticalOscillation",
        "HKQuantityTypeIdentifierSixMinuteWalkTestDistance",
        "HKQuantityTypeIdentifierStairAscentSpeed",
        "HKQuantityTypeIdentifierStairDescentSpeed",
        "HKQuantityTypeIdentifierWalkingAsymmetryPercentage",
        "HKQuantityTypeIdentifierWalkingDoubleSupportPercentage",
        "HKQuantityTypeIdentifierWalkingSpeed",
        "HKQuantityTypeIdentifierWalkingStepLength",
        "HKQuantityTypeIdentifierDietaryBiotin",
        "HKQuantityTypeIdentifierDietaryCaffeine",
        "HKQuantityTypeIdentifierDietaryCalcium",
        "HKQuantityTypeIdentifierDietaryCarbohydrates",
        "HKQuantityTypeIdentifierDietaryChloride",
        "HKQuantityTypeIdentifierDietaryCholesterol",
        "HKQuantityTypeIdentifierDietaryChromium",
        "HKQuantityTypeIdentifierDietaryCopper",
        "HKQuantityTypeIdentifierDietaryEnergyConsumed",
        "HKQuantityTypeIdentifierDietaryFatMonounsaturated",
        "HKQuantityTypeIdentifierDietaryFatPolyunsaturated",
        "HKQuantityTypeIdentifierDietaryFatSaturated",
        "HKQuantityTypeIdentifierDietaryFatTotal",
        "HKQuantityTypeIdentifierDietaryFiber",
        "HKQuantityTypeIdentifierDietaryFolate",
        "HKQuantityTypeIdentifierDietaryIodine",
        "HKQuantityTypeIdentifierDietaryIron",
        "HKQuantityTypeIdentifierDietaryMagnesium",
        "HKQuantityTypeIdentifierDietaryManganese",
        "HKQuantityTypeIdentifierDietaryMolybdenum",
        "HKQuantityTypeIdentifierDietaryNiacin",
        "HKQuantityTypeIdentifierDietaryPantothenicAcid",
        "HKQuantityTypeIdentifierDietaryPhosphorus",
        "HKQuantityTypeIdentifierDietaryPotassium",
        "HKQuantityTypeIdentifierDietaryProtein",
        "HKQuantityTypeIdentifierDietaryRiboflavin",
        "HKQuantityTypeIdentifierDietarySelenium",
        "HKQuantityTypeIdentifierDietarySodium",
        "HKQuantityTypeIdentifierDietarySugar",
        "HKQuantityTypeIdentifierDietaryThiamin",
        "HKQuantityTypeIdentifierDietaryVitaminA",
        "HKQuantityTypeIdentifierDietaryVitaminB12",
        "HKQuantityTypeIdentifierDietaryVitaminB6",
        "HKQuantityTypeIdentifierDietaryVitaminC",
        "HKQuantityTypeIdentifierDietaryVitaminD",
        "HKQuantityTypeIdentifierDietaryVitaminE",
        "HKQuantityTypeIdentifierDietaryVitaminK",
        "HKQuantityTypeIdentifierDietaryWater",
        "HKQuantityTypeIdentifierDietaryZinc",
        "HKQuantityTypeIdentifierBloodAlcoholContent",
        "HKQuantityTypeIdentifierBloodPressureDiastolic",
        "HKQuantityTypeIdentifierBloodPressureSystolic",
        "HKQuantityTypeIdentifierInsulinDelivery",
        "HKQuantityTypeIdentifierNumberOfAlcoholicBeverages",
        "HKQuantityTypeIdentifierNumberOfTimesFallen",
        "HKQuantityTypeIdentifierTimeInDaylight",
        "HKQuantityTypeIdentifierUVExposure",
        "HKQuantityTypeIdentifierWaterTemperature",
        "HKQuantityTypeIdentifierBasalBodyTemperature",
        "HKQuantityTypeIdentifierAppleSleepingBreathingDisturbances",
        "HKQuantityTypeIdentifierForcedExpiratoryVolume1",
        "HKQuantityTypeIdentifierForcedVitalCapacity",
        "HKQuantityTypeIdentifierInhalerUsage",
        "HKQuantityTypeIdentifierOxygenSaturation",
        "HKQuantityTypeIdentifierPeakExpiratoryFlowRate",
        "HKQuantityTypeIdentifierRespiratoryRate",
        "HKQuantityTypeIdentifierBloodGlucose",
        "HKQuantityTypeIdentifierBodyTemperature",
    ]

    static let categoryIdentifiers: [String] = [
        "HKCategoryTypeIdentifierAppleStandHour",
        "HKCategoryTypeIdentifierEnvironmentalAudioExposureEvent",
        "HKCategoryTypeIdentifierHeadphoneAudioExposureEvent",
        "HKCategoryTypeIdentifierHighHeartRateEvent",
        "HKCategoryTypeIdentifierHypertensionEvent",
        "HKCategoryTypeIdentifierIrregularHeartRhythmEvent",
        "HKCategoryTypeIdentifierLowCardioFitnessEvent",
        "HKCategoryTypeIdentifierLowHeartRateEvent",
        "HKCategoryTypeIdentifierMindfulSession",
        "HKCategoryTypeIdentifierAppleWalkingSteadinessEvent",
        "HKCategoryTypeIdentifierHandwashingEvent",
        "HKCategoryTypeIdentifierToothbrushingEvent",
        "HKCategoryTypeIdentifierBleedingAfterMenopause",
        "HKCategoryTypeIdentifierBleedingAfterPregnancy",
        "HKCategoryTypeIdentifierBleedingDuringPregnancy",
        "HKCategoryTypeIdentifierCervicalMucusQuality",
        "HKCategoryTypeIdentifierContraceptive",
        "HKCategoryTypeIdentifierInfrequentMenstrualCycles",
        "HKCategoryTypeIdentifierIntermenstrualBleeding",
        "HKCategoryTypeIdentifierIrregularMenstrualCycles",
        "HKCategoryTypeIdentifierLactation",
        "HKCategoryTypeIdentifierMenopausalState",
        "HKCategoryTypeIdentifierMenstrualFlow",
        "HKCategoryTypeIdentifierOvulationTestResult",
        "HKCategoryTypeIdentifierPersistentIntermenstrualBleeding",
        "HKCategoryTypeIdentifierPregnancy",
        "HKCategoryTypeIdentifierPregnancyTestResult",
        "HKCategoryTypeIdentifierProgesteroneTestResult",
        "HKCategoryTypeIdentifierProlongedMenstrualPeriods",
        "HKCategoryTypeIdentifierSexualActivity",
        "HKCategoryTypeIdentifierSleepApneaEvent",
        "HKCategoryTypeIdentifierSleepAnalysis",
        "HKCategoryTypeIdentifierAbdominalCramps",
        "HKCategoryTypeIdentifierAcne",
        "HKCategoryTypeIdentifierAppetiteChanges",
        "HKCategoryTypeIdentifierBladderIncontinence",
        "HKCategoryTypeIdentifierBloating",
        "HKCategoryTypeIdentifierBreastPain",
        "HKCategoryTypeIdentifierChestTightnessOrPain",
        "HKCategoryTypeIdentifierChills",
        "HKCategoryTypeIdentifierConstipation",
        "HKCategoryTypeIdentifierCoughing",
        "HKCategoryTypeIdentifierDiarrhea",
        "HKCategoryTypeIdentifierDizziness",
        "HKCategoryTypeIdentifierDrySkin",
        "HKCategoryTypeIdentifierFainting",
        "HKCategoryTypeIdentifierFatigue",
        "HKCategoryTypeIdentifierFever",
        "HKCategoryTypeIdentifierGeneralizedBodyAche",
        "HKCategoryTypeIdentifierHairLoss",
        "HKCategoryTypeIdentifierHeadache",
        "HKCategoryTypeIdentifierHeartburn",
        "HKCategoryTypeIdentifierHotFlashes",
        "HKCategoryTypeIdentifierLossOfSmell",
        "HKCategoryTypeIdentifierLossOfTaste",
        "HKCategoryTypeIdentifierLowerBackPain",
        "HKCategoryTypeIdentifierMemoryLapse",
        "HKCategoryTypeIdentifierMoodChanges",
        "HKCategoryTypeIdentifierNausea",
        "HKCategoryTypeIdentifierNightSweats",
        "HKCategoryTypeIdentifierPelvicPain",
        "HKCategoryTypeIdentifierRapidPoundingOrFlutteringHeartbeat",
        "HKCategoryTypeIdentifierRunnyNose",
        "HKCategoryTypeIdentifierShortnessOfBreath",
        "HKCategoryTypeIdentifierSinusCongestion",
        "HKCategoryTypeIdentifierSkippedHeartbeat",
        "HKCategoryTypeIdentifierSleepChanges",
        "HKCategoryTypeIdentifierSoreThroat",
        "HKCategoryTypeIdentifierVaginalDryness",
        "HKCategoryTypeIdentifierVomiting",
        "HKCategoryTypeIdentifierWheezing",
    ]

    static let characteristicIdentifiers: [String] = [
        "HKCharacteristicTypeIdentifierActivityMoveMode",
        "HKCharacteristicTypeIdentifierBiologicalSex",
        "HKCharacteristicTypeIdentifierBloodType",
        "HKCharacteristicTypeIdentifierDateOfBirth",
        "HKCharacteristicTypeIdentifierFitzpatrickSkinType",
        "HKCharacteristicTypeIdentifierWheelchairUse",
    ]

    static let correlationIdentifiers: [String] = [
        "HKCorrelationTypeIdentifierBloodPressure",
        "HKCorrelationTypeIdentifierFood",
    ]

    static let documentIdentifiers: [String] = [
        "HKDocumentTypeIdentifierCDA",
    ]

    static let scoredAssessmentIdentifiers: [String] = [
        "HKScoredAssessmentTypeIdentifierGAD7",
        "HKScoredAssessmentTypeIdentifierPHQ9",
    ]

    static let specialIdentifiers: [String] = [
        "HKWorkoutTypeIdentifier",
        "HKDataTypeIdentifierHeartbeatSeries",
        "HKElectrocardiogramType",
        "HKAudiogramSampleType",
        "HKVisionPrescriptionType",
        "HKStateOfMindType",
        "HKMedicationDoseEventType",
        "HKUserAnnotatedMedicationType",
        "HKActivitySummaryTypeIdentifier",
    ]

    static var declaredIdentifiers: [String] {
        quantityIdentifiers
            + categoryIdentifiers
            + characteristicIdentifiers
            + correlationIdentifiers
            + documentIdentifiers
            + scoredAssessmentIdentifiers
            + specialIdentifiers
    }

    static let readTypes: Set<HKObjectType> = makeReadTypes()

    static var sampleTypes: [HKSampleType] {
        readTypes.compactMap { $0 as? HKSampleType }.sorted { $0.identifier < $1.identifier }
    }

    static func unit(forQuantityIdentifier identifier: String) -> HKUnit {
        switch identifier {
        case "HKQuantityTypeIdentifierAppleSleepingWristTemperature":
            return HKUnit.degreeCelsius()
        case "HKQuantityTypeIdentifierBodyFatPercentage":
            return HKUnit.percent()
        case "HKQuantityTypeIdentifierBodyMass":
            return HKUnit.gramUnit(with: .kilo)
        case "HKQuantityTypeIdentifierBodyMassIndex":
            return HKUnit.count()
        case "HKQuantityTypeIdentifierElectrodermalActivity":
            return HKUnit.siemen()
        case "HKQuantityTypeIdentifierHeight":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierLeanBodyMass":
            return HKUnit.gramUnit(with: .kilo)
        case "HKQuantityTypeIdentifierWaistCircumference":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierActiveEnergyBurned":
            return HKUnit.kilocalorie()
        case "HKQuantityTypeIdentifierAppleExerciseTime":
            return HKUnit.minute()
        case "HKQuantityTypeIdentifierAppleMoveTime":
            return HKUnit.minute()
        case "HKQuantityTypeIdentifierAppleStandTime":
            return HKUnit.minute()
        case "HKQuantityTypeIdentifierBasalEnergyBurned":
            return HKUnit.kilocalorie()
        case "HKQuantityTypeIdentifierCrossCountrySkiingSpeed":
            return HKUnit.meter().unitDivided(by: HKUnit.second())
        case "HKQuantityTypeIdentifierCyclingCadence":
            return HKUnit.count().unitDivided(by: HKUnit.minute())
        case "HKQuantityTypeIdentifierCyclingFunctionalThresholdPower":
            return HKUnit.watt()
        case "HKQuantityTypeIdentifierCyclingPower":
            return HKUnit.watt()
        case "HKQuantityTypeIdentifierCyclingSpeed":
            return HKUnit.meter().unitDivided(by: HKUnit.second())
        case "HKQuantityTypeIdentifierDistanceCrossCountrySkiing":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierDistanceCycling":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierDistanceDownhillSnowSports":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierDistancePaddleSports":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierDistanceRowing":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierDistanceSkatingSports":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierDistanceSwimming":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierDistanceWalkingRunning":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierDistanceWheelchair":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierEstimatedWorkoutEffortScore":
            return HKUnit.appleEffortScore()
        case "HKQuantityTypeIdentifierFlightsClimbed":
            return HKUnit.count()
        case "HKQuantityTypeIdentifierNikeFuel":
            return HKUnit.count()
        case "HKQuantityTypeIdentifierPaddleSportsSpeed":
            return HKUnit.meter().unitDivided(by: HKUnit.second())
        case "HKQuantityTypeIdentifierPhysicalEffort":
            return HKUnit.kilocalorie().unitDivided(by: HKUnit.gramUnit(with: .kilo).unitMultiplied(by: HKUnit.hour()))
        case "HKQuantityTypeIdentifierPushCount":
            return HKUnit.count()
        case "HKQuantityTypeIdentifierRowingSpeed":
            return HKUnit.meter().unitDivided(by: HKUnit.second())
        case "HKQuantityTypeIdentifierRunningPower":
            return HKUnit.watt()
        case "HKQuantityTypeIdentifierRunningSpeed":
            return HKUnit.meter().unitDivided(by: HKUnit.second())
        case "HKQuantityTypeIdentifierStepCount":
            return HKUnit.count()
        case "HKQuantityTypeIdentifierSwimmingStrokeCount":
            return HKUnit.count()
        case "HKQuantityTypeIdentifierUnderwaterDepth":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierWorkoutEffortScore":
            return HKUnit.appleEffortScore()
        case "HKQuantityTypeIdentifierEnvironmentalAudioExposure":
            return HKUnit.decibelAWeightedSoundPressureLevel()
        case "HKQuantityTypeIdentifierEnvironmentalSoundReduction":
            return HKUnit.decibelAWeightedSoundPressureLevel()
        case "HKQuantityTypeIdentifierHeadphoneAudioExposure":
            return HKUnit.decibelAWeightedSoundPressureLevel()
        case "HKQuantityTypeIdentifierAtrialFibrillationBurden":
            return HKUnit.percent()
        case "HKQuantityTypeIdentifierHeartRate":
            return HKUnit.count().unitDivided(by: HKUnit.second())
        case "HKQuantityTypeIdentifierHeartRateRecoveryOneMinute":
            return HKUnit.count().unitDivided(by: HKUnit.minute())
        case "HKQuantityTypeIdentifierHeartRateVariabilityRMSSD":
            return HKUnit.secondUnit(with: .milli)
        case "HKQuantityTypeIdentifierHeartRateVariabilitySDNN":
            return HKUnit.secondUnit(with: .milli)
        case "HKQuantityTypeIdentifierPeripheralPerfusionIndex":
            return HKUnit.percent()
        case "HKQuantityTypeIdentifierRestingHeartRate":
            return HKUnit.count().unitDivided(by: HKUnit.minute())
        case "HKQuantityTypeIdentifierVO2Max":
            return HKUnit.literUnit(with: .milli).unitDivided(by: HKUnit.gramUnit(with: .kilo).unitMultiplied(by: HKUnit.minute()))
        case "HKQuantityTypeIdentifierWalkingHeartRateAverage":
            return HKUnit.count().unitDivided(by: HKUnit.minute())
        case "HKQuantityTypeIdentifierAppleWalkingSteadiness":
            return HKUnit.percent()
        case "HKQuantityTypeIdentifierRunningGroundContactTime":
            return HKUnit.secondUnit(with: .milli)
        case "HKQuantityTypeIdentifierRunningStrideLength":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierRunningVerticalOscillation":
            return HKUnit.meterUnit(with: .centi)
        case "HKQuantityTypeIdentifierSixMinuteWalkTestDistance":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierStairAscentSpeed":
            return HKUnit.meter().unitDivided(by: HKUnit.second())
        case "HKQuantityTypeIdentifierStairDescentSpeed":
            return HKUnit.meter().unitDivided(by: HKUnit.second())
        case "HKQuantityTypeIdentifierWalkingAsymmetryPercentage":
            return HKUnit.percent()
        case "HKQuantityTypeIdentifierWalkingDoubleSupportPercentage":
            return HKUnit.percent()
        case "HKQuantityTypeIdentifierWalkingSpeed":
            return HKUnit.meter().unitDivided(by: HKUnit.second())
        case "HKQuantityTypeIdentifierWalkingStepLength":
            return HKUnit.meter()
        case "HKQuantityTypeIdentifierDietaryBiotin":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryCaffeine":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryCalcium":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryCarbohydrates":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryChloride":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryCholesterol":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryChromium":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryCopper":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryEnergyConsumed":
            return HKUnit.kilocalorie()
        case "HKQuantityTypeIdentifierDietaryFatMonounsaturated":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryFatPolyunsaturated":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryFatSaturated":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryFatTotal":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryFiber":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryFolate":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryIodine":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryIron":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryMagnesium":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryManganese":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryMolybdenum":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryNiacin":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryPantothenicAcid":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryPhosphorus":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryPotassium":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryProtein":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryRiboflavin":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietarySelenium":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietarySodium":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietarySugar":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryThiamin":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryVitaminA":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryVitaminB12":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryVitaminB6":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryVitaminC":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryVitaminD":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryVitaminE":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryVitaminK":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierDietaryWater":
            return HKUnit.literUnit(with: .milli)
        case "HKQuantityTypeIdentifierDietaryZinc":
            return HKUnit.gram()
        case "HKQuantityTypeIdentifierBloodAlcoholContent":
            return HKUnit.percent()
        case "HKQuantityTypeIdentifierBloodPressureDiastolic":
            return HKUnit.millimeterOfMercury()
        case "HKQuantityTypeIdentifierBloodPressureSystolic":
            return HKUnit.millimeterOfMercury()
        case "HKQuantityTypeIdentifierInsulinDelivery":
            return HKUnit.internationalUnit()
        case "HKQuantityTypeIdentifierNumberOfAlcoholicBeverages":
            return HKUnit.count()
        case "HKQuantityTypeIdentifierNumberOfTimesFallen":
            return HKUnit.count()
        case "HKQuantityTypeIdentifierTimeInDaylight":
            return HKUnit.minute()
        case "HKQuantityTypeIdentifierUVExposure":
            return HKUnit.count()
        case "HKQuantityTypeIdentifierWaterTemperature":
            return HKUnit.degreeCelsius()
        case "HKQuantityTypeIdentifierBasalBodyTemperature":
            return HKUnit.degreeCelsius()
        case "HKQuantityTypeIdentifierAppleSleepingBreathingDisturbances":
            return HKUnit.count()
        case "HKQuantityTypeIdentifierForcedExpiratoryVolume1":
            return HKUnit.liter()
        case "HKQuantityTypeIdentifierForcedVitalCapacity":
            return HKUnit.liter()
        case "HKQuantityTypeIdentifierInhalerUsage":
            return HKUnit.count()
        case "HKQuantityTypeIdentifierOxygenSaturation":
            return HKUnit.percent()
        case "HKQuantityTypeIdentifierPeakExpiratoryFlowRate":
            return HKUnit.liter().unitDivided(by: HKUnit.minute())
        case "HKQuantityTypeIdentifierRespiratoryRate":
            return HKUnit.count().unitDivided(by: HKUnit.second())
        case "HKQuantityTypeIdentifierBloodGlucose":
            return HKUnit.gramUnit(with: .milli).unitDivided(by: HKUnit.literUnit(with: .deci))
        case "HKQuantityTypeIdentifierBodyTemperature":
            return HKUnit.degreeCelsius()
        default:
            return HKUnit.count()
        }
    }

    private static func makeReadTypes() -> Set<HKObjectType> {
        var types = Set<HKObjectType>()
        for name in quantityIdentifiers {
            let identifier = HKQuantityTypeIdentifier(rawValue: name)
            if let type = HKObjectType.quantityType(forIdentifier: identifier) {
                types.insert(type)
            }
        }
        for name in categoryIdentifiers {
            let identifier = HKCategoryTypeIdentifier(rawValue: name)
            if let type = HKObjectType.categoryType(forIdentifier: identifier) {
                types.insert(type)
            }
        }
        for name in characteristicIdentifiers {
            let identifier = HKCharacteristicTypeIdentifier(rawValue: name)
            if let type = HKObjectType.characteristicType(forIdentifier: identifier) {
                types.insert(type)
            }
        }
        for name in correlationIdentifiers {
            let identifier = HKCorrelationTypeIdentifier(rawValue: name)
            if let type = HKObjectType.correlationType(forIdentifier: identifier) {
                types.insert(type)
            }
        }
        for name in documentIdentifiers {
            let identifier = HKDocumentTypeIdentifier(rawValue: name)
            if let type = HKObjectType.documentType(forIdentifier: identifier) {
                types.insert(type)
            }
        }
        for name in scoredAssessmentIdentifiers {
            types.insert(HKScoredAssessmentType(HKScoredAssessmentTypeIdentifier(rawValue: name)))
        }
        types.insert(HKObjectType.workoutType())
        if let heartbeat = HKObjectType.seriesType(forIdentifier: HKDataTypeIdentifierHeartbeatSeries) {
            types.insert(heartbeat)
        }
        types.insert(HKObjectType.electrocardiogramType())
        types.insert(HKObjectType.audiogramSampleType())
        types.insert(HKObjectType.visionPrescriptionType())
        types.insert(HKObjectType.stateOfMindType())
        types.insert(HKObjectType.medicationDoseEventType())
        types.insert(HKObjectType.userAnnotatedMedicationType())
        types.insert(HKObjectType.activitySummaryType())
        return types
    }
}
