# HealthConnect upload API

This document is the contract between the iOS app and a server.
The app sends one copy of Apple Health data.
The repository does not include the server that receives this data.

## Terms

- Verification code: the secret that the user types in the app.
- Export: one upload from one tap of the upload button.
- Batch: one HTTP body of newline-delimited JSON (NDJSON).
- Base URL: the server address that the user saves.

## Base URL

The app appends a relative path to the saved address.

- `https://example.com` + `v1/exports` becomes `https://example.com/v1/exports`.
- `https://example.com/base` + `v1/exports` becomes `https://example.com/base/v1/exports`.

The app accepts `https` for every host.
The app accepts `http` only for these hosts:

- `localhost`
- `::1`
- a name that ends with `.local`
- an IPv4 address in `10.0.0.0/8`, `127.0.0.0/8`, `172.16.0.0/12`, or `192.168.0.0/16`

The app rejects any other `http` address.
The app rejects a URL that contains a username or a password.

## Authentication

The app sends this header on every export request:

```http
Authorization: Bearer <verification code>
```

The verification code is not in the JSON body.
The server must return HTTP 401 when the code is wrong or missing.
The server must not write the code, the request body, or health values to logs.

## Create an export

```http
POST /v1/exports
Content-Type: application/json
```

Request body:

```json
{"startedAt":"2026-01-01T00:00:00.000Z","mode":"replace"}
```

`startedAt` is an ISO-8601 date with fractional seconds.
`mode` is optional. `replace` is the default. `replace` removes the user's stored facts and stores this export. `merge` adds rows and applies `kind` `deleted` records. Any other value is HTTP 400 `invalid_mode`.

A `deleted` record has `kind`, `typeIdentifier`, and `uuid`. The server deletes stored rows with that `uuid`. A record with an empty `uuid` is ignored.

A `merge` export upserts characteristic keys that are present. It does not remove other characteristic keys. A `replace` export replaces the characteristic set.

Success response is HTTP 201:

```json
{"exportId":"0123456789abcdef0123456789abcdef"}
```

`exportId` is 32 lowercase hexadecimal characters.
The app uses this id for the batch requests and the complete request.

## Send a batch

```http
POST /v1/exports/{exportId}/batches
Content-Type: application/x-ndjson
```

The body contains one JSON object on each line.
The line ends with a newline character (`\n`).

Success response is HTTP 204 and an empty body.

The server must store the raw body.
The server must return HTTP 404 when `exportId` is unknown.
The server must return HTTP 400 when the body is empty.
The server must accept a body up to 32 MB.
The app keeps each body at or below 8 MB.

## Complete an export

```http
POST /v1/exports/{exportId}/complete
Content-Type: application/json
```

Request body:

```json
{"sampleCount":1200,"batchCount":4}
```

- `sampleCount` is the number of records that the app sent.
- `batchCount` is the number of batch requests that the app sent.

Success response is HTTP 200.

The server must store a manifest for the export.
The manifest must contain:

- `sampleCount` from the request
- `batchCount` from the request
- `receivedBatchCount`, the number of batch bodies that the server stored
- `completedAt`, the server time in ISO-8601

The app treats only HTTP 200 as a finished upload.
The app then locks the upload button for that server address and verification code.

## Failure behavior

The app does not call `complete` when one or more HealthKit types fail to read.
The app does not lock the button after HTTP 401, a network error, or an incomplete export.
A later tap creates a new export id.
The server can keep the earlier partial export.

## Record fields

Each line is one record.
These fields are present when the app has a value:

| Field | Meaning |
| --- | --- |
| `kind` | `sample`, `characteristic`, `activitySummary`, or `medication` |
| `typeIdentifier` | HealthKit type identifier |
| `uuid` | Sample UUID |
| `start` | Start date, ISO-8601 |
| `end` | End date, ISO-8601 |
| `source` | Source bundle identifier |
| `device` | Device name |
| `metadata` | Sample metadata. Dates are strings. Binary values are Base64. |
| `value` | Quantity value as a number |
| `unit` | HealthKit unit string for `value` |
| `objects` | Child records of a correlation |

A characteristic record uses `kind` = `characteristic`.
Its `fields` are top-level JSON keys, for example `year`, `month`, `day`, or `value`.

An activity summary uses `kind` = `activitySummary`.
A medication record uses `kind` = `medication`.

ECG records can contain a `voltages` array.
Each voltage object has `timeSinceStart` and `millivolts` for Apple Watch lead I.
Heartbeat series records can contain a `beats` array.
Each beat has `timeSinceStart` and `precededByGap`.

The app does not send workout GPS routes.
The app does not send clinical health records.
The app does not send the XML body of a CDA document.

## Page size

The app reads one HealthKit type at a time.
A normal page has at most 500 samples.
An ECG page or a heartbeat-series page has at most 20 samples.
The app sends characteristics first, then samples, then activity summaries, then annotated medications.

## Optional health check

```http
GET /v1/healthz
```

This request has no authentication.
Success response is HTTP 200.
The iOS app does not call this endpoint.
