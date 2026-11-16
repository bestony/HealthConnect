package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	ExportOpen      = "open"
	ExportCompleted = "completed"
)

// ErrExportClosed means the export no longer accepts batches.
var ErrExportClosed = errors.New("export closed")

// Export is one iOS upload session.
type Export struct {
	ID                 string
	UserID             string
	StartedAt          string
	Status             string
	SampleCount        int64
	BatchCount         int64
	ReceivedBatchCount int64
	CompletedAt        string
	CreatedAt          string
	Mode               string
}

// CreateExport inserts an open export.
func (db *DB) CreateExport(ctx context.Context, id, userID, startedAt, mode string) (Export, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.sql.ExecContext(ctx, `
		INSERT INTO exports(id, user_id, started_at, status, received_batch_count, created_at, mode)
		VALUES (?, ?, ?, ?, 0, ?, ?)`, id, userID, startedAt, ExportOpen, now, mode)
	if err != nil {
		return Export{}, fmt.Errorf("sqlite insert export: %w", err)
	}
	db.log.Info("export created", "export_id", id, "user_id", userID, "mode", mode)
	return db.GetExport(ctx, userID, id)
}

// GetExport returns an export owned by the user.
func (db *DB) GetExport(ctx context.Context, userID, id string) (Export, error) {
	row := db.sql.QueryRowContext(ctx, `
		SELECT id, user_id, started_at, status,
		       COALESCE(sample_count, 0), COALESCE(batch_count, 0), received_batch_count,
		       COALESCE(completed_at, ''), created_at, COALESCE(mode, 'replace')
		FROM exports WHERE user_id = ? AND id = ?`, userID, id)
	var exp Export
	err := row.Scan(&exp.ID, &exp.UserID, &exp.StartedAt, &exp.Status, &exp.SampleCount, &exp.BatchCount, &exp.ReceivedBatchCount, &exp.CompletedAt, &exp.CreatedAt, &exp.Mode)
	if errors.Is(err, sql.ErrNoRows) {
		return Export{}, ErrNotFound
	}
	if err != nil {
		return Export{}, fmt.Errorf("sqlite get export: %w", err)
	}
	return exp, nil
}

// ReserveBatch increments the stored batch count and returns the new file index.
func (db *DB) ReserveBatch(ctx context.Context, userID, id string) (int64, error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("sqlite begin batch: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var status string
	var count int64
	err = tx.QueryRowContext(ctx, `
		SELECT status, received_batch_count FROM exports WHERE user_id = ? AND id = ?`, userID, id).Scan(&status, &count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("sqlite read export: %w", err)
	}
	if status != ExportOpen {
		return 0, ErrExportClosed
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE exports SET received_batch_count = received_batch_count + 1 WHERE id = ?`, id); err != nil {
		return 0, fmt.Errorf("sqlite reserve batch: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("sqlite commit batch: %w", err)
	}
	return count, nil
}

// ReleaseBatch undoes a reserved index when the body could not be stored.
func (db *DB) ReleaseBatch(ctx context.Context, id string, index int64) error {
	_, err := db.sql.ExecContext(ctx, `
		UPDATE exports
		SET received_batch_count = received_batch_count - 1
		WHERE id = ? AND status = ? AND received_batch_count = ?`, id, ExportOpen, index+1)
	if err != nil {
		return fmt.Errorf("sqlite release batch: %w", err)
	}
	return nil
}

// Characteristic returns one stored characteristic value.
func (db *DB) Characteristic(ctx context.Context, userID, key string) (string, error) {
	var value string
	err := db.sql.QueryRowContext(ctx, `
		SELECT attr_value FROM characteristics WHERE user_id = ? AND attr_key = ?`, userID, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("sqlite characteristic: %w", err)
	}
	return value, nil
}

// CompleteExport stores the manifest. A second call returns the stored row.
func (db *DB) CompleteExport(ctx context.Context, userID, id string, sampleCount, batchCount int64, completedAt string) (Export, error) {
	res, err := db.sql.ExecContext(ctx, `
		UPDATE exports
		SET status = ?, sample_count = ?, batch_count = ?, completed_at = ?
		WHERE user_id = ? AND id = ? AND status = ?`,
		ExportCompleted, sampleCount, batchCount, completedAt, userID, id, ExportOpen)
	if err != nil {
		return Export{}, fmt.Errorf("sqlite complete export: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		exp, err := db.GetExport(ctx, userID, id)
		if err != nil {
			return Export{}, err
		}
		if exp.Status == ExportCompleted {
			return exp, nil
		}
		return Export{}, ErrExportClosed
	}
	db.log.Info("export completed", "export_id", id, "user_id", userID, "sample_count", sampleCount, "batch_count", batchCount)
	return db.GetExport(ctx, userID, id)
}
