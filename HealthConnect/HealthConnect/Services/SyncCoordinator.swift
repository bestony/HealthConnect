import Foundation
import HealthKit
import os

nonisolated enum ObserverQuerySupport {
    static func allows(_ type: HKSampleType) -> Bool {
        HCObserverQuerySupported(type)
    }
}

@MainActor
final class SyncCoordinator {
    private let session: UploadSession
    private let store: HKHealthStore
    private let sampleTypes: [HKSampleType]
    private var queries: [HKObserverQuery] = []
    private var started = false

    init(session: UploadSession, store: HKHealthStore = HKHealthStore(), sampleTypes: [HKSampleType]) {
        self.session = session
        self.store = store
        self.sampleTypes = sampleTypes
    }

    func start() {
        guard !started else {
            return
        }
        started = true
        guard HKHealthStore.isHealthDataAvailable() else {
            AppLog.health.info("Background delivery skipped because HealthKit is unavailable")
            return
        }
        let session = self.session
        var registered = 0
        var skipped = 0
        for type in sampleTypes {
            let typeIdentifier = type.identifier
            guard ObserverQuerySupport.allows(type) else {
                skipped += 1
                AppLog.health.info("Observer query is not supported for \(typeIdentifier, privacy: .public)")
                continue
            }
            let query = HKObserverQuery(sampleType: type, predicate: nil) { _, completion, error in
                if error != nil {
                    AppLog.health.error("Health observer failed for \(typeIdentifier, privacy: .public)")
                    completion()
                    return
                }
                Task { @MainActor in
                    session.noteHealthUpdate(completion: completion)
                }
            }
            store.execute(query)
            queries.append(query)
            registered += 1
            store.enableBackgroundDelivery(for: type, frequency: .immediate) { success, error in
                if success {
                    return
                }
                let code = (error as NSError?)?.code ?? 0
                AppLog.health.error("Background delivery was not enabled for \(typeIdentifier, privacy: .public) code \(code, privacy: .public)")
            }
        }
        AppLog.health.info("Registered \(registered, privacy: .public) health observers and skipped \(skipped, privacy: .public)")
    }
}
