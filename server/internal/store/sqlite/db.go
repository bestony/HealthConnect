// Package sqlite stores users, tokens, import jobs, and characteristics.
package sqlite

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"com.bestony.health-connect/migrations"

	_ "modernc.org/sqlite"
)

// ErrNotFound means the row does not exist or is not visible to the caller.
var ErrNotFound = errors.New("not found")

// DB is one SQLite database. Version 1 uses a single connection.
type DB struct {
	sql  *sql.DB
	log  *slog.Logger
	path string
}

// Open creates the parent directory, opens the database, and applies migrations.
func Open(ctx context.Context, path string, log *slog.Logger) (*DB, error) {
	if log == nil {
		log = slog.Default()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("sqlite abs: %w", err)
	}
	path = abs
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("sqlite mkdir: %w", err)
	}
	dsn := url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)",
	}
	sqlDB, err := sql.Open("sqlite", dsn.String())
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	db := &DB{sql: sqlDB, log: log, path: path}
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}
	if _, err := sqlDB.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("sqlite foreign_keys: %w", err)
	}
	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := db.chmod(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	log.Info("sqlite ready", "path", path)
	return db, nil
}

// Close closes the database.
func (db *DB) Close() error {
	return db.sql.Close()
}

func (db *DB) migrate(ctx context.Context) error {
	if _, err := db.sql.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`); err != nil {
		return fmt.Errorf("sqlite schema_migrations: %w", err)
	}
	entries, err := fs.ReadDir(migrations.SQLite, "sqlite")
	if err != nil {
		return fmt.Errorf("sqlite read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}
		var n int
		if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&n); err != nil {
			return fmt.Errorf("sqlite migration lookup: %w", err)
		}
		if n > 0 {
			continue
		}
		body, err := migrations.SQLite.ReadFile("sqlite/" + name)
		if err != nil {
			return fmt.Errorf("sqlite read %s: %w", name, err)
		}
		tx, err := db.sql.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("sqlite begin migration: %w", err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite apply %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, version, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite record %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("sqlite commit %s: %w", name, err)
		}
		db.log.Info("sqlite migration applied", "version", version, "file", name)
	}
	return nil
}

func migrationVersion(name string) (int, error) {
	prefix, _, ok := strings.Cut(name, "_")
	if !ok {
		return 0, fmt.Errorf("sqlite migration name %q: want NNN_name.sql", name)
	}
	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("sqlite migration name %q: %w", name, err)
	}
	return version, nil
}

func (db *DB) chmod() error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		path := db.path + suffix
		info, err := os.Stat(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return fmt.Errorf("sqlite stat %s: %w", path, err)
		}
		if info.Mode().Perm() == 0o600 {
			continue
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("sqlite chmod %s: %w", path, err)
		}
	}
	return nil
}

// BootstrapToken creates the first user and token when the token table is empty.
// The raw token is written to tokenPath. A later call does not rotate the token.
func (db *DB) BootstrapToken(ctx context.Context, tokenPath string) (created bool, err error) {
	var n int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_tokens`).Scan(&n); err != nil {
		return false, fmt.Errorf("sqlite count tokens: %w", err)
	}
	if n > 0 {
		db.log.Info("sqlite token already present", "bootstrap_file", tokenPath)
		return false, nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return false, fmt.Errorf("sqlite token rand: %w", err)
	}
	token := "hc_" + hex.EncodeToString(raw)
	if err := writeFileAtomic(tokenPath, []byte(token+"\n"), 0o600); err != nil {
		return false, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	userID := newID()
	tokenID := newID()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("sqlite begin bootstrap: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO users(id, display_name, created_at) VALUES (?, ?, ?)`, userID, "owner", now); err != nil {
		_ = tx.Rollback()
		return false, fmt.Errorf("sqlite insert user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO api_tokens(id, user_id, token_hash, created_at) VALUES (?, ?, ?, ?)`, tokenID, userID, hashToken(token), now); err != nil {
		_ = tx.Rollback()
		return false, fmt.Errorf("sqlite insert token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("sqlite commit bootstrap: %w", err)
	}
	db.log.Info("sqlite bootstrap token written", "path", tokenPath, "user_id", userID)
	return true, nil
}

// MintToken creates a user and a bearer token. The raw token is returned once.
func (db *DB) MintToken(ctx context.Context, displayName string) (userID, token string, err error) {
	if strings.TrimSpace(displayName) == "" {
		displayName = "owner"
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("sqlite token rand: %w", err)
	}
	token = "hc_" + hex.EncodeToString(raw)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	userID = newID()
	tokenID := newID()
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return "", "", fmt.Errorf("sqlite begin mint: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO users(id, display_name, created_at) VALUES (?, ?, ?)`, userID, displayName, now); err != nil {
		_ = tx.Rollback()
		return "", "", fmt.Errorf("sqlite insert user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO api_tokens(id, user_id, token_hash, created_at) VALUES (?, ?, ?, ?)`, tokenID, userID, hashToken(token), now); err != nil {
		_ = tx.Rollback()
		return "", "", fmt.Errorf("sqlite insert token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", "", fmt.Errorf("sqlite commit mint: %w", err)
	}
	db.log.Info("sqlite token minted", "user_id", userID)
	return userID, token, nil
}

// UserIDByToken returns the user id for a raw bearer token.
func (db *DB) UserIDByToken(ctx context.Context, token string) (string, error) {
	var userID string
	err := db.sql.QueryRowContext(ctx, `
		SELECT user_id FROM api_tokens
		WHERE token_hash = ? AND revoked_at IS NULL`, hashToken(token)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("sqlite token lookup: %w", err)
	}
	return userID, nil
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func newID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	var buf [36]byte
	hex.Encode(buf[0:8], raw[0:4])
	buf[8] = '-'
	hex.Encode(buf[9:13], raw[4:6])
	buf[13] = '-'
	hex.Encode(buf[14:18], raw[6:8])
	buf[18] = '-'
	hex.Encode(buf[19:23], raw[8:10])
	buf[23] = '-'
	hex.Encode(buf[24:36], raw[10:16])
	return string(buf[:])
}

func writeFileAtomic(path string, body []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("mkdir token dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".bootstrap-*")
	if err != nil {
		return fmt.Errorf("create token file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod token file: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write token file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close token file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename token file: %w", err)
	}
	cleanup = false
	return nil
}
