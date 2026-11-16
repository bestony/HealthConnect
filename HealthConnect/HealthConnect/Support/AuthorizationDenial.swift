import Foundation

/// Parses the type names from a HealthKit authorization exception.
enum AuthorizationDenial {
    static func identifiers(in reason: String) -> Set<String> {
        guard let colon = reason.range(of: ":") else {
            return []
        }
        let tail = reason[colon.upperBound...]
        let names = tail.split(separator: ",").map {
            $0.trimmingCharacters(in: .whitespacesAndNewlines)
        }
        return Set(names.filter { $0.hasPrefix("HK") })
    }
}
