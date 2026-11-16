import Foundation
import Observation
import os
import StoreKit

protocol TipProductSource: AnyObject {
    func products() async throws -> [TipOffer]
    func purchase(id: String) async throws -> TipPurchaseOutcome
}

@MainActor
@Observable
final class TipStore {
    private let source: TipProductSource
    private var purchasable: Set<String> = []

    private(set) var offers: [TipOffer] = TipCatalog.tiers
    private(set) var message: String?
    private(set) var isLoading = false
    private(set) var isPurchasing = false
    private(set) var didLoad = false

    init(source: TipProductSource, observeTransactions: Bool = false) {
        self.source = source
        if observeTransactions {
            Task { await TipStore.finishTipUpdates() }
        }
    }

    func load() async {
        guard !isLoading else {
            return
        }
        isLoading = true
        defer { isLoading = false }
        do {
            let loaded = try await source.products()
            let known = loaded.filter { TipCatalog.contains($0.id) }
            purchasable = Set(known.map(\.id))
            offers = TipCatalog.tiers.map { tier in
                known.first { $0.id == tier.id } ?? tier
            }
            didLoad = true
            if known.isEmpty {
                message = "打赏商品暂时无法加载。"
            } else if purchasable.count < TipCatalog.tiers.count {
                message = "有的打赏档位暂时无法购买。"
            }
            AppLog.tips.info("Loaded \(self.purchasable.count, privacy: .public) tip products")
        } catch {
            didLoad = true
            AppLog.tips.error("Tip products failed to load")
            message = "打赏商品暂时无法加载。"
        }
    }

    func purchase(_ id: String) async {
        guard !isPurchasing else {
            return
        }
        guard purchasable.contains(id) else {
            message = "这一档暂时无法购买。"
            AppLog.tips.error("Tip product is unavailable")
            return
        }
        isPurchasing = true
        defer { isPurchasing = false }
        do {
            switch try await source.purchase(id: id) {
            case .success:
                message = "感谢打赏。"
                AppLog.tips.info("Tip purchase finished")
            case .cancelled:
                AppLog.tips.info("Tip purchase canceled")
            case .pending:
                message = "购买正在等待确认。"
                AppLog.tips.info("Tip purchase is pending")
            case .failed:
                message = "打赏没有完成。可以重试。"
                AppLog.tips.error("Tip purchase did not finish")
            }
        } catch {
            message = "打赏没有完成。可以重试。"
            AppLog.tips.error("Tip purchase failed")
        }
    }

    func canPurchase(_ id: String) -> Bool {
        didLoad && !isPurchasing && purchasable.contains(id)
    }

    private static func finishTipUpdates() async {
        for await update in Transaction.updates {
            if Task.isCancelled {
                return
            }
            guard let transaction = TipTransaction.verified(update) else {
                AppLog.tips.error("Tip transaction failed verification")
                continue
            }
            guard TipCatalog.contains(transaction.productID) else {
                continue
            }
            await transaction.finish()
            AppLog.tips.info("Finished a tip transaction")
        }
    }
}

enum TipTransaction {
    static func verified<T>(_ result: VerificationResult<T>) -> T? {
        switch result {
        case .verified(let value):
            return value
        case .unverified:
            return nil
        }
    }
}

final class StoreKitTipSource: TipProductSource {
    private var productsByID: [String: Product] = [:]

    func products() async throws -> [TipOffer] {
        let ids = Set(TipCatalog.tiers.map(\.id))
        let products = try await Product.products(for: ids)
        productsByID = Dictionary(uniqueKeysWithValues: products.map { ($0.id, $0) })
        return products.map { TipOffer(id: $0.id, priceText: $0.displayPrice) }
    }

    func purchase(id: String) async throws -> TipPurchaseOutcome {
        guard let product = productsByID[id] else {
            return .failed
        }
        switch try await product.purchase() {
        case .success(let verification):
            guard let transaction = TipTransaction.verified(verification) else {
                return .failed
            }
            await transaction.finish()
            return .success
        case .userCancelled:
            return .cancelled
        case .pending:
            return .pending
        @unknown default:
            return .failed
        }
    }
}
