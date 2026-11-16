import os

enum AppLog {
    static let setup = Logger(subsystem: "com.bestony.HealthConnect", category: "setup")
    static let health = Logger(subsystem: "com.bestony.HealthConnect", category: "health")
    static let upload = Logger(subsystem: "com.bestony.HealthConnect", category: "upload")
    static let tips = Logger(subsystem: "com.bestony.HealthConnect", category: "tips")
}
