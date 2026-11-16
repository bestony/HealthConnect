import Foundation
import Testing
@testable import HealthConnect

struct TipCatalogTests {
    @Test func listsThreeOneTimePrices() {
        #expect(TipCatalog.tiers.map(\.priceText) == ["0.99", "1.99", "4.99"])
        #expect(Set(TipCatalog.tiers.map(\.id)).count == 3)
        #expect(TipCatalog.contains("com.bestony.HealthConnect.tip.199"))
        #expect(!TipCatalog.contains("com.bestony.HealthConnect.tip.unknown"))
    }
}

@MainActor
struct TipStoreTests {
    @Test func purchaseSuccessThanksTheBuyer() async {
        let source = ScriptedTipSource()
        let store = TipStore(source: source)
        await store.load()
        await store.purchase(TipCatalog.tiers[0].id)
        #expect(store.message == "感谢打赏。")
        #expect(source.purchased == [TipCatalog.tiers[0].id])
        #expect(store.offers.map(\.priceText) == ["0.99", "1.99", "4.99"])
    }

    @Test func cancelLeavesTheMessageUnchanged() async {
        let source = ScriptedTipSource()
        source.outcome = .cancelled
        let store = TipStore(source: source)
        await store.load()
        await store.purchase(TipCatalog.tiers[1].id)
        #expect(store.message == nil)
        #expect(source.purchased == [TipCatalog.tiers[1].id])
    }

    @Test func aMissingTierCannotBePurchased() async {
        let source = ScriptedTipSource()
        source.offers = [TipCatalog.tiers[0]]
        let store = TipStore(source: source)
        await store.load()
        #expect(store.message == "有的打赏档位暂时无法购买。")
        await store.purchase(TipCatalog.tiers[2].id)
        #expect(store.message == "这一档暂时无法购买。")
        #expect(source.purchased.isEmpty)
        #expect(!store.canPurchase(TipCatalog.tiers[2].id))
        #expect(store.canPurchase(TipCatalog.tiers[0].id))
    }

    @Test func anEmptyCatalogCannotBePurchased() async {
        let source = ScriptedTipSource()
        source.offers = []
        let store = TipStore(source: source)
        await store.load()
        #expect(store.message == "打赏商品暂时无法加载。")
        #expect(!store.canPurchase(TipCatalog.tiers[0].id))
    }

    @Test func aFailedLoadDoesNotEnablePurchase() async {
        let source = ScriptedTipSource()
        source.loadError = TipTestError.unavailable
        let store = TipStore(source: source)
        await store.load()
        #expect(store.message == "打赏商品暂时无法加载。")
        await store.purchase(TipCatalog.tiers[0].id)
        #expect(source.purchased.isEmpty)
    }
}

private enum TipTestError: Error {
    case unavailable
}

@MainActor
private final class ScriptedTipSource: TipProductSource {
    var offers = TipCatalog.tiers
    var outcome: TipPurchaseOutcome = .success
    var loadError: Error?
    private(set) var purchased: [String] = []

    func products() async throws -> [TipOffer] {
        if let loadError {
            throw loadError
        }
        return offers
    }

    func purchase(id: String) async throws -> TipPurchaseOutcome {
        purchased.append(id)
        return outcome
    }
}
