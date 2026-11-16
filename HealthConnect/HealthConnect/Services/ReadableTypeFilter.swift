import Foundation

/// Types HealthKit refused to put in one read-authorization request.
@MainActor
final class ReadableTypeFilter {
    private var disallowed: Set<String> = []

    func setDisallowed(_ identifiers: Set<String>) {
        disallowed = identifiers
    }

    func allows(_ identifier: String) -> Bool {
        !disallowed.contains(identifier)
    }
}
