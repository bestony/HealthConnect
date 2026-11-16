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
    private static let structuralKeys: Set<String> = [
        "kind", "typeIdentifier", "uuid", "start", "end", "source", "device", "metadata", "objects",
    ]

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
            guard let value = fields[key] else {
                continue
            }
            let name = Self.structuralKeys.contains(key) ? "field_\(key)" : key
            try container.encode(value, forKey: DynamicCodingKey(name))
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

enum ExportBatchError: Error, Equatable {
    case recordTooLarge
}

enum ExportBatchSizer {
    static let maxBytes = 8_000_000

    static func groups(records: [ExportRecord], maxBytes: Int = ExportBatchSizer.maxBytes) throws -> [Data] {
        let pieces = try records.flatMap { try split($0, maxBytes: maxBytes) }
        var groups: [Data] = []
        var current: [ExportRecord] = []
        var currentBytes = 0
        for piece in pieces {
            let encoded = try ExportPageEncoder.encode([piece])
            if encoded.count > maxBytes {
                throw ExportBatchError.recordTooLarge
            }
            if !current.isEmpty && currentBytes + encoded.count > maxBytes {
                groups.append(try ExportPageEncoder.encode(current))
                current = []
                currentBytes = 0
            }
            current.append(piece)
            currentBytes += encoded.count
        }
        if !current.isEmpty {
            groups.append(try ExportPageEncoder.encode(current))
        }
        return groups
    }

    private static func split(_ record: ExportRecord, maxBytes: Int) throws -> [ExportRecord] {
        let encoded = try ExportPageEncoder.encode([record])
        if encoded.count <= maxBytes {
            return [record]
        }
        if let key = largestArrayKey(record), case .array(let items) = record.fields[key] {
            do {
                return try splitCollection(record, count: items.count, maxBytes: maxBytes) { count in
                    var piece = record
                    piece.fields[key] = .array(Array(items.prefix(count)))
                    return piece
                } advance: { count in
                    var rest = record
                    rest.fields[key] = .array(Array(items.dropFirst(count)))
                    return rest
                }
            } catch ExportBatchError.recordTooLarge {
            }
        }
        if let objects = record.objects, objects.count > 1 {
            return try splitCollection(record, count: objects.count, maxBytes: maxBytes) { count in
                var piece = record
                piece.objects = Array(objects.prefix(count))
                return piece
            } advance: { count in
                var rest = record
                rest.objects = Array(objects.dropFirst(count))
                return rest
            }
        }
        throw ExportBatchError.recordTooLarge
    }

    private static func splitCollection(
        _ record: ExportRecord,
        count: Int,
        maxBytes: Int,
        piece: (Int) -> ExportRecord,
        advance: (Int) -> ExportRecord
    ) throws -> [ExportRecord] {
        var taken = count
        var chosen: ExportRecord?
        while taken >= 1 {
            let candidate = piece(taken)
            let size = try ExportPageEncoder.encode([candidate]).count
            if size <= maxBytes {
                chosen = candidate
                break
            }
            if taken == 1 {
                throw ExportBatchError.recordTooLarge
            }
            taken = max(taken / 2, 1)
        }
        guard let chosen else {
            throw ExportBatchError.recordTooLarge
        }
        var records = try split(chosen, maxBytes: maxBytes)
        if taken < count {
            records.append(contentsOf: try split(advance(taken), maxBytes: maxBytes))
        }
        return records
    }

    private static func largestArrayKey(_ record: ExportRecord) -> String? {
        var best: (key: String, count: Int)?
        for key in record.fields.keys.sorted() {
            guard case .array(let items) = record.fields[key], items.count > 1 else {
                continue
            }
            if best == nil || items.count > best!.count {
                best = (key, items.count)
            }
        }
        return best?.key
    }
}
