import Foundation

nonisolated struct TipOffer: Identifiable, Equatable, Sendable {
    var id: String
    var priceText: String
}

nonisolated enum TipPurchaseOutcome: Equatable, Sendable {
    case success
    case cancelled
    case pending
    case failed
}

enum TipCatalog {
    static let tiers: [TipOffer] = [
        TipOffer(id: "com.bestony.HealthConnect.tip.099", priceText: "0.99"),
        TipOffer(id: "com.bestony.HealthConnect.tip.199", priceText: "1.99"),
        TipOffer(id: "com.bestony.HealthConnect.tip.499", priceText: "4.99"),
    ]

    static func contains(_ productID: String) -> Bool {
        tiers.contains { $0.id == productID }
    }
}
