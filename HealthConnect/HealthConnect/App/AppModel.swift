import Foundation
import Observation
import os

enum AppRoute: Equatable {
    case setup
    case upload
}

@MainActor
@Observable
final class AppModel {
    private let settings: SettingsStoring

    let session: UploadSession
    var route: AppRoute
    var draftAddress: String
    var draftCode: String
    var setupMessage: String?
    var isSaving = false

    init(settings: SettingsStoring, session: UploadSession) {
        self.settings = settings
        self.session = session
        if let saved = settings.load() {
            draftAddress = saved.url.absoluteString
            draftCode = saved.verificationCode
            route = .upload
        } else {
            draftAddress = ""
            draftCode = ""
            route = .setup
        }
    }

    func confirm() async {
        isSaving = true
        defer { isSaving = false }
        switch ServerEndpoint.parse(address: draftAddress, verificationCode: draftCode) {
        case .failure(let error):
            setupMessage = error.message
            AppLog.setup.info("Rejected server settings")
        case .success(let endpoint):
            do {
                try settings.save(endpoint)
                setupMessage = nil
                route = .upload
                AppLog.setup.info("Saved server settings")
                await session.reload()
            } catch {
                AppLog.setup.error("Could not save server settings")
                setupMessage = "无法保存设置。"
            }
        }
    }

    var savedServerText: String {
        guard let host = settings.load()?.url.host, !host.isEmpty else {
            return "已保存"
        }
        return host
    }

    func editSettings() {
        if let saved = settings.load() {
            draftAddress = saved.url.absoluteString
            draftCode = saved.verificationCode
        }
        route = .setup
    }

    static func live() -> AppModel {
        let settings = SettingsStore(defaults: .standard, credentials: KeychainCredentialStore())
        let readableTypes = ReadableTypeFilter()
        let session = UploadSession(
            settings: settings,
            access: HealthAccessCoordinator(client: HealthKitAuthorizationClient(readableTypes: readableTypes)),
            makeExporter: { HealthKitExportSession(readableTypes: readableTypes) },
            makeUploader: { ExportUploader(transport: URLSessionExportTransport()) }
        )
        return AppModel(settings: settings, session: session)
    }
}
