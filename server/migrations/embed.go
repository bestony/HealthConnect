// Package migrations holds embedded SQL for SQLite and DuckDB.
package migrations

import "embed"

// SQLite is the set of SQLite migration files. Names start with a zero-padded version.
//
//go:embed sqlite/*.sql
var SQLite embed.FS

// DuckDB is the set of DuckDB migration files. Names start with a zero-padded version.
//
//go:embed duckdb/*.sql
var DuckDB embed.FS
