package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	StatusAccepted  = "accepted"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	ModeReplace     = "replace"
	ModeMerge       = "merge"
)

var (
	ErrImportActive        = errors.New("import active")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrNotRetriable        = errors.New("not retriable")
)

// Import is one upload job.
type Import struct {
	ID             string
	UserID         string
	IdempotencyKey string
	ContentHash    []byte
	Mode           string
	Status         string
	Phase          string
	Filename       string
	ByteSize       int64
	RowsRead       int64
	RowsWritten    int64
	Error          string
	ExportDate     string
	CreatedAt      string
	StartedAt      string
	FinishedAt     string
}

// CreateImport inserts a job, or returns the existing job when the idempotency key and bytes match.
func (db *DB) CreateImport(ctx context.Context, in Import) (Import, bool, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return Import{}, false, fmt.Errorf("sqlite begin import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if in.IdempotencyKey != "" {
		existing, err := findIdempotent(ctx, tx, in.UserID, in.IdempotencyKey)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return Import{}, false, err
		}
		if err == nil {
			if !bytes.Equal(existing.ContentHash, in.ContentHash) {
				return Import{}, false, ErrIdempotencyConflict
			}
			return existing, true, nil
		}
	}
	var active int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM imports
		WHERE user_id = ? AND status IN (?, ?)`, in.UserID, StatusAccepted, StatusRunning).Scan(&active); err != nil {
		return Import{}, false, fmt.Errorf("sqlite active import: %w", err)
	}
	if active > 0 {
		return Import{}, false, ErrImportActive
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	in.Status = StatusAccepted
	in.CreatedAt = now
	_, err = tx.ExecContext(ctx, `
		INSERT INTO imports(
			id, user_id, idempotency_key, content_hash, mode, status, phase,
			filename, byte_size, rows_read, rows_written, created_at
		) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, '', ?, ?, 0, 0, ?)`,
		in.ID, in.UserID, in.IdempotencyKey, in.ContentHash, in.Mode, in.Status,
		in.Filename, in.ByteSize, now)
	if err != nil {
		return Import{}, false, fmt.Errorf("sqlite insert import: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Import{}, false, fmt.Errorf("sqlite commit import: %w", err)
	}
	db.log.Info("import accepted", "import_id", in.ID, "user_id", in.UserID, "mode", in.Mode, "byte_size", in.ByteSize)
	return in, false, nil
}

func findIdempotent(ctx context.Context, tx *sql.Tx, userID, key string) (Import, error) {
	row := tx.QueryRowContext(ctx, importSelect+` WHERE user_id = ? AND idempotency_key = ?`, userID, key)
	imp, err := scanImport(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Import{}, ErrNotFound
	}
	return imp, err
}

// GetImport returns one job owned by the user.
func (db *DB) GetImport(ctx context.Context, userID, id string) (Import, error) {
	row := db.sql.QueryRowContext(ctx, importSelect+` WHERE user_id = ? AND id = ?`, userID, id)
	imp, err := scanImport(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Import{}, ErrNotFound
	}
	if err != nil {
		return Import{}, fmt.Errorf("sqlite get import: %w", err)
	}
	return imp, nil
}

// GetImportByID returns a job for the worker.
func (db *DB) GetImportByID(ctx context.Context, id string) (Import, error) {
	row := db.sql.QueryRowContext(ctx, importSelect+` WHERE id = ?`, id)
	imp, err := scanImport(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Import{}, ErrNotFound
	}
	if err != nil {
		return Import{}, fmt.Errorf("sqlite get import by id: %w", err)
	}
	return imp, nil
}

// ListImports returns the newest jobs for the user.
func (db *DB) ListImports(ctx context.Context, userID string) ([]Import, error) {
	rows, err := db.sql.QueryContext(ctx, importSelect+` WHERE user_id = ? ORDER BY created_at DESC, rowid DESC LIMIT 50`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlite list imports: %w", err)
	}
	defer rows.Close()
	var out []Import
	for rows.Next() {
		imp, err := scanImport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, imp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite list imports: %w", err)
	}
	return out, nil
}

// MarkRunning moves a job to running.
func (db *DB) MarkRunning(ctx context.Context, id, phase string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.sql.ExecContext(ctx, `
		UPDATE imports
		SET status = ?, phase = ?, started_at = COALESCE(started_at, ?)
		WHERE id = ?`, StatusRunning, phase, now, id)
	if err != nil {
		return fmt.Errorf("sqlite mark running: %w", err)
	}
	db.log.Info("import running", "import_id", id, "phase", phase)
	return nil
}

// UpdateProgress stores the parse phase and the number of records read.
func (db *DB) UpdateProgress(ctx context.Context, id, phase string, rowsRead int64) error {
	_, err := db.sql.ExecContext(ctx, `UPDATE imports SET phase = ?, rows_read = ? WHERE id = ?`, phase, rowsRead, id)
	if err != nil {
		return fmt.Errorf("sqlite progress: %w", err)
	}
	db.log.Debug("import progress", "import_id", id, "phase", phase, "rows_read", rowsRead)
	return nil
}

// SetExportDate stores the export timestamp as UTC RFC3339.
func (db *DB) SetExportDate(ctx context.Context, id string, at time.Time) error {
	_, err := db.sql.ExecContext(ctx, `UPDATE imports SET export_date = ? WHERE id = ?`, at.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("sqlite export date: %w", err)
	}
	return nil
}

// MarkSucceeded closes a job and records the written fact count.
func (db *DB) MarkSucceeded(ctx context.Context, id string, rowsWritten int64) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.sql.ExecContext(ctx, `
		UPDATE imports
		SET status = ?, phase = '', rows_written = ?, error = NULL, finished_at = ?
		WHERE id = ?`, StatusSucceeded, rowsWritten, now, id)
	if err != nil {
		return fmt.Errorf("sqlite mark succeeded: %w", err)
	}
	db.log.Info("import succeeded", "import_id", id, "rows_written", rowsWritten)
	return nil
}

// MarkFailed closes a job with a stable error code.
func (db *DB) MarkFailed(ctx context.Context, id, code string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if len(code) > 200 {
		code = code[:200]
	}
	_, err := db.sql.ExecContext(ctx, `
		UPDATE imports SET status = ?, phase = '', error = ?, finished_at = ? WHERE id = ?`,
		StatusFailed, code, now, id)
	if err != nil {
		return fmt.Errorf("sqlite mark failed: %w", err)
	}
	db.log.Error("import failed", "import_id", id, "error_code", code)
	return nil
}

// InterruptActive marks queued and running jobs as interrupted.
func (db *DB) InterruptActive(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := db.sql.ExecContext(ctx, `
		UPDATE imports
		SET status = ?, phase = '', error = 'interrupted', finished_at = ?
		WHERE status IN (?, ?)`, StatusFailed, now, StatusAccepted, StatusRunning)
	if err != nil {
		return fmt.Errorf("sqlite interrupt: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		db.log.Info("imports interrupted", "count", n)
	}
	return nil
}

// Requeue moves a failed job back to accepted when the user has no active job.
func (db *DB) Requeue(ctx context.Context, userID, id string) (Import, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return Import{}, fmt.Errorf("sqlite begin requeue: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var active int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM imports
		WHERE user_id = ? AND status IN (?, ?) AND id <> ?`, userID, StatusAccepted, StatusRunning, id).Scan(&active); err != nil {
		return Import{}, fmt.Errorf("sqlite active import: %w", err)
	}
	if active > 0 {
		return Import{}, ErrImportActive
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE imports
		SET status = ?, phase = '', error = NULL, rows_read = 0, rows_written = 0,
			started_at = NULL, finished_at = NULL
		WHERE id = ? AND user_id = ? AND status = ?`, StatusAccepted, id, userID, StatusFailed)
	if err != nil {
		return Import{}, fmt.Errorf("sqlite requeue: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Import{}, ErrNotRetriable
	}
	if err := tx.Commit(); err != nil {
		return Import{}, fmt.Errorf("sqlite commit requeue: %w", err)
	}
	return db.GetImport(ctx, userID, id)
}

// ReplaceCharacteristics replaces the caller's <Me> attributes.
func (db *DB) ReplaceCharacteristics(ctx context.Context, userID, importID string, attrs map[string]string) error {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite begin characteristics: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM characteristics WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("sqlite delete characteristics: %w", err)
	}
	for key, value := range attrs {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO characteristics(user_id, attr_key, attr_value, import_id)
			VALUES (?, ?, ?, ?)`, userID, key, value, importID); err != nil {
			return fmt.Errorf("sqlite insert characteristic: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite commit characteristics: %w", err)
	}
	db.log.Info("characteristics replaced", "user_id", userID, "import_id", importID, "count", len(attrs))
	return nil
}

const importSelect = `
	SELECT id, user_id, COALESCE(idempotency_key, ''), content_hash, mode, status, phase,
		filename, byte_size, rows_read, rows_written, COALESCE(error, ''), COALESCE(export_date, ''),
		created_at, COALESCE(started_at, ''), COALESCE(finished_at, '')
	FROM imports`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanImport(row rowScanner) (Import, error) {
	var imp Import
	var hash []byte
	err := row.Scan(
		&imp.ID, &imp.UserID, &imp.IdempotencyKey, &hash, &imp.Mode, &imp.Status, &imp.Phase,
		&imp.Filename, &imp.ByteSize, &imp.RowsRead, &imp.RowsWritten, &imp.Error, &imp.ExportDate,
		&imp.CreatedAt, &imp.StartedAt, &imp.FinishedAt,
	)
	if err != nil {
		return Import{}, err
	}
	imp.ContentHash = hash
	return imp, nil
}
