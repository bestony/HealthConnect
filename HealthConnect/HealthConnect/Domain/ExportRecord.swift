import Foundation

nonisolated enum ExportValue: Equatable, Sendable {
    case string(String)
    case number(Double)
    case int(Int)
    case bool(Bool)
    case array([ExportValue])
    case object([String: ExportValue])
}

extension ExportValue: Encodable {
    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .string(let value):
            try container.encode(value)
        case .number(let value):
            try container.encode(value)
        case .int(let value):
            try container.encode(value)
        case .bool(let value):
            try container.encode(value)
        case .array(let value):
            try container.encode(value)
        case .object(let value):
            try container.encode(value)
        }
    }
}

nonisolated struct ExportRecord: Equatable, Sendable {
    var kind: String
    var typeIdentifier: String
    var uuid: String?
    var start: String?
    var end: String?
    var source: String?
    var device: String?
    var fields: [String: ExportValue] = [:]
    var metadata: [String: ExportValue] = [:]
    var objects: [ExportRecord]?
}

extension ExportRecord: Encodable {
    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: DynamicCodingKey.self)
        try container.encode(kind, forKey: DynamicCodingKey("kind"))
        try container.encode(typeIdentifier, forKey: DynamicCodingKey("typeIdentifier"))
        try container.encodeIfPresent(uuid, forKey: DynamicCodingKey("uuid"))
        try container.encodeIfPresent(start, forKey: DynamicCodingKey("start"))
        try container.encodeIfPresent(end, forKey: DynamicCodingKey("end"))
        try container.encodeIfPresent(source, forKey: DynamicCodingKey("source"))
        try container.encodeIfPresent(device, forKey: DynamicCodingKey("device"))
        for key in fields.keys.sorted() {
            if let value = fields[key] {
                try container.encode(value, forKey: DynamicCodingKey(key))
            }
        }
        if !metadata.isEmpty {
            try container.encode(metadata, forKey: DynamicCodingKey("metadata"))
        }
        if let objects, !objects.isEmpty {
            try container.encode(objects, forKey: DynamicCodingKey("objects"))
        }
    }
}

nonisolated struct ExportPage: Equatable, Sendable {
    var typeIdentifier: String
    var records: [ExportRecord]
}

nonisolated struct DynamicCodingKey: CodingKey, Sendable {
    var stringValue: String
    var intValue: Int? { nil }

    init(_ string: String) {
        stringValue = string
    }

    init?(stringValue: String) {
        self.stringValue = stringValue
    }

    init?(intValue: Int) {
        return nil
    }
}

enum ExportDate {
    static func string(from date: Date) -> String {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.string(from: date)
    }
}

enum ExportPageEncoder {
    static func encode(_ records: [ExportRecord]) throws -> Data {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
        var body = Data()
        for record in records {
            var line = try encoder.encode(record)
            line.append(0x0A)
            body.append(line)
        }
        return body
    }
}

enum ExportBatchSizer {
    static let maxBytes = 8_000_000

    static func groups(records: [ExportRecord]) throws -> [Data] {
        var groups: [Data] = []
        var current: [ExportRecord] = []
        var currentBytes = 0
        for record in records {
            let encoded = try ExportPageEncoder.encode([record])
            if !current.isEmpty && currentBytes + encoded.count > maxBytes {
                groups.append(try ExportPageEncoder.encode(current))
                current = []
                currentBytes = 0
            }
            current.append(record)
            currentBytes += encoded.count
        }
        if !current.isEmpty {
            groups.append(try ExportPageEncoder.encode(current))
        }
        return groups
    }
}
