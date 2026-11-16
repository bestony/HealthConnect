import Foundation
import HealthKit
import os

enum HealthSampleEncoder {
    static func record(for sample: HKSample, store: HKHealthStore) async -> ExportRecord {
        var record = base(sample)
        if let quantity = sample as? HKQuantitySample {
            apply(quantity, to: &record)
        } else if let category = sample as? HKCategorySample {
            record.fields["value"] = .int(category.value)
        } else if let correlation = sample as? HKCorrelation {
            var children: [ExportRecord] = []
            for object in correlation.objects {
                children.append(await self.record(for: object, store: store))
            }
            record.objects = children
        } else if let workout = sample as? HKWorkout {
            apply(workout, to: &record)
        } else if let electrocardiogram = sample as? HKElectrocardiogram {
            await apply(electrocardiogram, store: store, to: &record)
        } else if let heartbeat = sample as? HKHeartbeatSeriesSample {
            await apply(heartbeat, store: store, to: &record)
        } else if let audiogram = sample as? HKAudiogramSample {
            apply(audiogram, to: &record)
        } else if let prescription = sample as? HKVisionPrescription {
            apply(prescription, to: &record)
        } else if let mind = sample as? HKStateOfMind {
            apply(mind, to: &record)
        } else if let assessment = sample as? HKGAD7Assessment {
            record.fields["score"] = .int(assessment.score)
            record.fields["answers"] = .array(assessment.answers.map { .string(String(describing: $0)) })
        } else if let assessment = sample as? HKPHQ9Assessment {
            record.fields["score"] = .int(assessment.score)
            record.fields["answers"] = .array(assessment.answers.map { .string(String(describing: $0)) })
        } else if let assessment = sample as? HKScoredAssessment {
            record.fields["score"] = .int(assessment.score)
        } else if let dose = sample as? HKMedicationDoseEvent {
            apply(dose, to: &record)
        }
        return record
    }

    static func characteristicRecords(store: HKHealthStore) -> [ExportRecord] {
        var records: [ExportRecord] = []
        if let components = readCharacteristic("dateOfBirth", store.dateOfBirthComponents) {
            var fields: [String: ExportValue] = [:]
            if let year = components.year { fields["year"] = .int(year) }
            if let month = components.month { fields["month"] = .int(month) }
            if let day = components.day { fields["day"] = .int(day) }
            records.append(characteristic("HKCharacteristicTypeIdentifierDateOfBirth", fields: fields))
        }
        if let sex = readCharacteristic("biologicalSex", store.biologicalSex) {
            records.append(characteristic(
                "HKCharacteristicTypeIdentifierBiologicalSex",
                fields: ["value": .int(sex.biologicalSex.rawValue)]
            ))
        }
        if let blood = readCharacteristic("bloodType", store.bloodType) {
            records.append(characteristic(
                "HKCharacteristicTypeIdentifierBloodType",
                fields: ["value": .int(blood.bloodType.rawValue)]
            ))
        }
        if let skin = readCharacteristic("fitzpatrickSkinType", store.fitzpatrickSkinType) {
            records.append(characteristic(
                "HKCharacteristicTypeIdentifierFitzpatrickSkinType",
                fields: ["value": .int(skin.skinType.rawValue)]
            ))
        }
        if let wheelchair = readCharacteristic("wheelchairUse", store.wheelchairUse) {
            records.append(characteristic(
                "HKCharacteristicTypeIdentifierWheelchairUse",
                fields: ["value": .int(wheelchair.wheelchairUse.rawValue)]
            ))
        }
        if let moveMode = readCharacteristic("activityMoveMode", store.activityMoveMode) {
            records.append(characteristic(
                "HKCharacteristicTypeIdentifierActivityMoveMode",
                fields: ["value": .int(moveMode.activityMoveMode.rawValue)]
            ))
        }
        return records
    }

    static func activityRecords(_ summaries: [HKActivitySummary]) -> [ExportRecord] {
        let calendar = Calendar(identifier: .gregorian)
        return summaries.map { summary in
            let components = summary.dateComponents(for: calendar)
            var fields: [String: ExportValue] = [
                "year": .int(components.year ?? 0),
                "month": .int(components.month ?? 0),
                "day": .int(components.day ?? 0),
                "moveMode": .int(summary.activityMoveMode.rawValue),
                "paused": .bool(summary.isPaused),
                "activeEnergyBurnedKcal": quantityValue(summary.activeEnergyBurned, unit: .kilocalorie()),
                "appleMoveTimeMin": quantityValue(summary.appleMoveTime, unit: .minute()),
                "appleExerciseTimeMin": quantityValue(summary.appleExerciseTime, unit: .minute()),
                "appleStandHours": quantityValue(summary.appleStandHours, unit: .count()),
                "activeEnergyBurnedGoalKcal": quantityValue(summary.activeEnergyBurnedGoal, unit: .kilocalorie()),
                "appleMoveTimeGoalMin": quantityValue(summary.appleMoveTimeGoal, unit: .minute()),
            ]
            if let goal = summary.exerciseTimeGoal {
                fields["exerciseTimeGoalMin"] = quantityValue(goal, unit: .minute())
            }
            if let goal = summary.standHoursGoal {
                fields["standHoursGoal"] = quantityValue(goal, unit: .count())
            }
            return ExportRecord(
                kind: "activitySummary",
                typeIdentifier: HKObjectType.activitySummaryType().identifier,
                fields: fields
            )
        }
    }

    static func medicationRecords(_ medications: [HKUserAnnotatedMedication]) -> [ExportRecord] {
        medications.map { medication in
            ExportRecord(
                kind: "medication",
                typeIdentifier: HKObjectType.userAnnotatedMedicationType().identifier,
                fields: [
                    "nickname": medication.nickname.map(ExportValue.string) ?? .string(""),
                    "isArchived": .bool(medication.isArchived),
                    "hasSchedule": .bool(medication.hasSchedule),
                    "displayText": .string(medication.medication.displayText),
                    "domain": .string(String(describing: medication.medication.identifier.domain)),
                ]
            )
        }
    }

    private static func base(_ sample: HKSample) -> ExportRecord {
        var metadata: [String: ExportValue] = [:]
        if let values = sample.metadata {
            for key in values.keys.sorted() {
                guard let raw = values[key], let encoded = metadataValue(raw) else {
                    continue
                }
                metadata[key] = encoded
            }
        }
        return ExportRecord(
            kind: "sample",
            typeIdentifier: sample.sampleType.identifier,
            uuid: sample.uuid.uuidString,
            start: ExportDate.string(from: sample.startDate),
            end: ExportDate.string(from: sample.endDate),
            source: sample.sourceRevision.source.bundleIdentifier,
            device: sample.device?.name,
            metadata: metadata
        )
    }

    private static func apply(_ sample: HKQuantitySample, to record: inout ExportRecord) {
        let unit = HealthTypeCatalog.unit(forQuantityIdentifier: sample.quantityType.identifier)
        guard sample.quantity.is(compatibleWith: unit) else {
            AppLog.health.error("Quantity unit is incompatible for \(sample.quantityType.identifier, privacy: .public)")
            return
        }
        record.fields["value"] = .number(sample.quantity.doubleValue(for: unit))
        record.fields["unit"] = .string(unit.unitString)
    }

    private static func apply(_ workout: HKWorkout, to record: inout ExportRecord) {
        record.fields["activityType"] = .int(Int(workout.workoutActivityType.rawValue))
        record.fields["duration"] = .number(workout.duration)
        var statistics: [ExportValue] = []
        for type in workout.allStatistics.keys.sorted(by: { $0.identifier < $1.identifier }) {
            guard let stats = workout.allStatistics[type] else {
                continue
            }
            let unit = HealthTypeCatalog.unit(forQuantityIdentifier: type.identifier)
            var item: [String: ExportValue] = ["type": .string(type.identifier)]
            if let sum = stats.sumQuantity(), sum.is(compatibleWith: unit) {
                item["sum"] = .number(sum.doubleValue(for: unit))
                item["unit"] = .string(unit.unitString)
            } else if let average = stats.averageQuantity(), average.is(compatibleWith: unit) {
                item["average"] = .number(average.doubleValue(for: unit))
                item["unit"] = .string(unit.unitString)
            }
            statistics.append(.object(item))
        }
        record.fields["statistics"] = .array(statistics)
    }

    private static func apply(
        _ electrocardiogram: HKElectrocardiogram,
        store: HKHealthStore,
        to record: inout ExportRecord
    ) async {
        record.fields["classification"] = .int(Int(electrocardiogram.classification.rawValue))
        record.fields["symptomsStatus"] = .int(Int(electrocardiogram.symptomsStatus.rawValue))
        record.fields["measurementCount"] = .int(electrocardiogram.numberOfVoltageMeasurements)
        if let heartRate = electrocardiogram.averageHeartRate {
            record.fields["averageHeartRate"] = quantityValue(heartRate, unit: HKUnit.count().unitDivided(by: .minute()))
        }
        var voltages: [ExportValue] = []
        let results = HKElectrocardiogramQueryDescriptor(electrocardiogram).results(for: store)
        do {
            for try await measurement in results {
                var row: [String: ExportValue] = [
                    "timeSinceStart": .number(measurement.timeSinceSampleStart),
                ]
                if let quantity = measurement.quantity(for: .appleWatchSimilarToLeadI) {
                    row["millivolts"] = quantityValue(quantity, unit: HKUnit.voltUnit(with: .milli))
                }
                voltages.append(.object(row))
            }
        } catch {
            AppLog.health.error("ECG voltage read failed for one sample")
        }
        record.fields["voltages"] = .array(voltages)
    }

    private static func apply(
        _ series: HKHeartbeatSeriesSample,
        store: HKHealthStore,
        to record: inout ExportRecord
    ) async {
        var beats: [ExportValue] = []
        let results = HKHeartbeatSeriesQueryDescriptor(series).results(for: store)
        do {
            for try await beat in results {
                beats.append(.object([
                    "timeSinceStart": .number(beat.timeIntervalSinceStart),
                    "precededByGap": .bool(beat.precededByGap),
                ]))
            }
        } catch {
            AppLog.health.error("Heartbeat series read failed for one sample")
        }
        record.fields["beats"] = .array(beats)
    }

    private static func apply(_ audiogram: HKAudiogramSample, to record: inout ExportRecord) {
        let points = audiogram.sensitivityPoints.map { point in
            let tests = point.tests.map { test in
                ExportValue.object([
                    "side": .int(test.side.rawValue),
                    "sensitivity": quantityValue(test.sensitivity, unit: .decibelHearingLevel()),
                ])
            }
            return ExportValue.object([
                "frequency": quantityValue(point.frequency, unit: .hertz()),
                "tests": .array(tests),
            ])
        }
        record.fields["sensitivityPoints"] = .array(points)
    }

    private static func apply(_ prescription: HKVisionPrescription, to record: inout ExportRecord) {
        record.fields["prescriptionType"] = .int(Int(prescription.prescriptionType.rawValue))
        record.fields["dateIssued"] = .string(ExportDate.string(from: prescription.dateIssued))
        if let expiration = prescription.expirationDate {
            record.fields["expirationDate"] = .string(ExportDate.string(from: expiration))
        }
    }

    private static func apply(_ mind: HKStateOfMind, to record: inout ExportRecord) {
        record.fields["stateOfMindKind"] = .string(String(describing: mind.kind))
        record.fields["valence"] = .number(mind.valence)
        record.fields["valenceClassification"] = .string(String(describing: mind.valenceClassification))
        record.fields["labels"] = .array(mind.labels.map { .string(String(describing: $0)) })
        record.fields["associations"] = .array(mind.associations.map { .string(String(describing: $0)) })
    }

    private static func apply(_ dose: HKMedicationDoseEvent, to record: inout ExportRecord) {
        record.fields["scheduleType"] = .int(dose.scheduleType.rawValue)
        record.fields["logStatus"] = .int(dose.logStatus.rawValue)
        record.fields["unit"] = .string(dose.unit.unitString)
        record.fields["domain"] = .string(String(describing: dose.medicationConceptIdentifier.domain))
        if let quantity = dose.doseQuantity {
            record.fields["doseQuantity"] = .number(quantity)
        }
        if let quantity = dose.scheduledDoseQuantity {
            record.fields["scheduledDoseQuantity"] = .number(quantity)
        }
        if let date = dose.scheduledDate {
            record.fields["scheduledDate"] = .string(ExportDate.string(from: date))
        }
    }

    private static func characteristic(_ identifier: String, fields: [String: ExportValue]) -> ExportRecord {
        ExportRecord(kind: "characteristic", typeIdentifier: identifier, fields: fields)
    }

    private static func readCharacteristic<T>(_ name: String, _ body: () throws -> T) -> T? {
        do {
            return try body()
        } catch {
            let nsError = error as NSError
            AppLog.health.debug("Characteristic \(name, privacy: .public) was not read. domain \(nsError.domain, privacy: .public) code \(nsError.code, privacy: .public)")
            return nil
        }
    }

    private static func quantityValue(_ quantity: HKQuantity, unit: HKUnit) -> ExportValue {
        guard quantity.is(compatibleWith: unit) else {
            return .string(String(describing: type(of: quantity)))
        }
        return .number(quantity.doubleValue(for: unit))
    }

    private static func metadataValue(_ value: Any) -> ExportValue? {
        if let text = value as? String {
            return .string(text)
        }
        if let date = value as? Date {
            return .string(ExportDate.string(from: date))
        }
        if let flag = value as? Bool {
            return .bool(flag)
        }
        if let data = value as? Data {
            return .string(data.base64EncodedString())
        }
        if let quantity = value as? HKQuantity {
            return .string(quantity.description)
        }
        if let number = value as? NSNumber {
            if CFGetTypeID(number) == CFBooleanGetTypeID() {
                return .bool(number.boolValue)
            }
            return .number(number.doubleValue)
        }
        if let array = value as? [Any] {
            var items: [ExportValue] = []
            for item in array {
                if let encoded = metadataValue(item) {
                    items.append(encoded)
                }
            }
            return .array(items)
        }
        if let dictionary = value as? [String: Any] {
            var object: [String: ExportValue] = [:]
            for key in dictionary.keys.sorted() {
                if let encoded = metadataValue(dictionary[key] as Any) {
                    object[key] = encoded
                }
            }
            return .object(object)
        }
        AppLog.health.debug("Skipped a metadata value of type \(String(describing: type(of: value)), privacy: .public)")
        return nil
    }
}
