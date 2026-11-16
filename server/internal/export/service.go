// Package export implements the iOS upload contract from docs/upload-api.md.
package export

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"com.bestony.health-connect/internal/ingest"
	"com.bestony.health-connect/internal/store/duckdb"
	"com.bestony.health-connect/internal/store/sqlite"
)

const MaxBatchBytes int64 = 32 << 20

// Service stores export batches and loads them when the export completes.
type Service struct {
	meta     *sqlite.DB
	facts    *duckdb.DB
	dir      string
	maxBatch int64
	log      *slog.Logger
	mu       sync.Mutex
}

// NewService builds the export API service.
func NewService(meta *sqlite.DB, facts *duckdb.DB, dataDir string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{meta: meta, facts: facts, dir: filepath.Join(dataDir, "exports"), maxBatch: MaxBatchBytes, log: log}
}

// SetMaxBatchBytes overrides the 32 MB batch limit. Tests use this.
func (s *Service) SetMaxBatchBytes(n int64) { s.maxBatch = n }

// Create starts an export. exportID is 32 lowercase hex characters.
// Any other open export for the user is abandoned first. One user has one open export.
func (s *Service) Create(ctx context.Context, userID, startedAt, mode string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if mode == "" {
		mode = sqlite.ModeReplace
	}
	open, err := s.meta.ListOpenExportIDs(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, id := range open {
		if err := s.abandonLocked(ctx, userID, id); err != nil {
			return "", err
		}
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)
	if _, err := s.meta.CreateExport(ctx, id, userID, startedAt, mode); err != nil {
		return "", err
	}
	return id, nil
}

// Abort marks one export abandoned and deletes its batch files.
// An unknown id returns sqlite.ErrNotFound.
// A completed export returns sqlite.ErrExportClosed.
// An export that is already abandoned returns nil.
func (s *Service) Abort(ctx context.Context, userID, exportID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.abandonLocked(ctx, userID, exportID)
}

// abandonLocked requires s.mu. It does not load batches.
func (s *Service) abandonLocked(ctx context.Context, userID, exportID string) error {
	if err := s.meta.AbandonExport(ctx, userID, exportID); err != nil {
		return err
	}
	dir := filepath.Join(s.dir, exportID)
	if err := os.RemoveAll(dir); err != nil {
		s.log.Error("export batch delete failed", "export_id", exportID, "user_id", userID)
		return err
	}
	s.log.Info("export batches removed", "export_id", exportID, "user_id", userID)
	return nil
}

// AddBatch stores one raw NDJSON body.
func (s *Service) AddBatch(ctx context.Context, userID, exportID string, r io.Reader) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	body, err := readLimited(r, s.maxBatch)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return ErrEmpty
	}
	index, err := s.meta.ReserveBatch(ctx, userID, exportID)
	if err != nil {
		return err
	}
	path := s.batchPath(exportID, index)
	if err := writeFile(path, body); err != nil {
		_ = s.meta.ReleaseBatch(ctx, exportID, index)
		return err
	}
	s.log.Info("export batch stored", "export_id", exportID, "user_id", userID, "batch_index", index, "byte_size", len(body))
	return nil
}

// Complete stores the manifest and loads stored batches.
func (s *Service) Complete(ctx context.Context, userID, exportID string, sampleCount, batchCount int64) (sqlite.Export, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, err := s.meta.GetExport(ctx, userID, exportID)
	if err != nil {
		return sqlite.Export{}, err
	}
	if exp.Status == sqlite.ExportCompleted {
		return exp, nil
	}
	if exp.Status != sqlite.ExportOpen {
		return sqlite.Export{}, sqlite.ErrExportClosed
	}
	if exp.ReceivedBatchCount > 0 {
		paths := make([]string, 0, exp.ReceivedBatchCount)
		for i := int64(0); i < exp.ReceivedBatchCount; i++ {
			paths = append(paths, s.batchPath(exportID, i))
		}
		mode := exp.Mode
		if mode == "" {
			mode = sqlite.ModeReplace
		}
		chars, err := ingest.LoadNDJSON(ctx, s.facts, userID, dashed(exportID), paths, mode)
		if err != nil {
			s.log.Error("export load failed", "export_id", exportID, "user_id", userID, "error_code", codeOf(err), "mode", mode)
			return sqlite.Export{}, err
		}
		if mode == sqlite.ModeMerge {
			err = s.meta.UpsertCharacteristics(ctx, userID, exportID, chars)
		} else {
			err = s.meta.ReplaceCharacteristics(ctx, userID, exportID, chars)
		}
		if err != nil {
			return sqlite.Export{}, err
		}
	}
	completedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	stored, err := s.meta.CompleteExport(ctx, userID, exportID, sampleCount, batchCount, completedAt)
	if err != nil {
		return sqlite.Export{}, err
	}
	s.log.Info("export manifest stored", "export_id", exportID, "user_id", userID, "received_batch_count", stored.ReceivedBatchCount)
	return stored, nil
}

func (s *Service) batchPath(exportID string, index int64) string {
	return filepath.Join(s.dir, exportID, fmt.Sprintf("%d.ndjson", index))
}

func dashed(hex32 string) string {
	return hex32[0:8] + "-" + hex32[8:12] + "-" + hex32[12:16] + "-" + hex32[16:20] + "-" + hex32[20:32]
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > max {
		return nil, ErrTooLarge
	}
	return body, nil
}

func writeFile(path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "batch-*.partial")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func codeOf(err error) string {
	var coded *ingest.Error
	if errors.As(err, &coded) {
		return coded.Error()
	}
	return "commit_failed"
}

// ErrEmpty means the batch body has no bytes.
var ErrEmpty = errors.New("empty batch")

// ErrTooLarge means the batch is larger than 32 MB.
var ErrTooLarge = errors.New("batch too large")
