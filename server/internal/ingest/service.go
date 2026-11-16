package ingest

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"com.bestony.health-connect/internal/domain"
	"com.bestony.health-connect/internal/store/sqlite"
)

const (
	phaseUnpack = "unpack"
	phaseParse  = "parse"
	phaseCommit = "commit"
)

// Queue accepts an import id for background work.
type Queue interface {
	Enqueue(id string)
}

// Service accepts uploads and reads job state.
type Service struct {
	meta  *sqlite.DB
	queue Queue
	inbox string
	log   *slog.Logger
}

// NewService builds an upload service. inbox is the directory that holds spooled files.
func NewService(meta *sqlite.DB, queue Queue, inbox string, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{meta: meta, queue: queue, inbox: inbox, log: log}
}

// Accept spools one upload and enqueues it.
func (s *Service) Accept(ctx context.Context, userID, filename, mode, idemKey string, r io.Reader) (sqlite.Import, error) {
	if mode == "" {
		mode = sqlite.ModeReplace
	}
	if mode != sqlite.ModeReplace && mode != sqlite.ModeMerge {
		return sqlite.Import{}, &Error{Code: "invalid_mode"}
	}
	idemKey = strings.TrimSpace(idemKey)
	if len(idemKey) > 128 {
		return sqlite.Import{}, &Error{Code: "invalid_idempotency_key"}
	}
	id, err := domain.NewID()
	if err != nil {
		return sqlite.Import{}, err
	}
	path := s.payloadPath(id)
	n, sum, err := writeSpool(path, r)
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(path))
		return sqlite.Import{}, err
	}
	if _, err := Classify(path); err != nil {
		_ = os.RemoveAll(filepath.Dir(path))
		return sqlite.Import{}, err
	}
	imp, replay, err := s.meta.CreateImport(ctx, sqlite.Import{
		ID: id, UserID: userID, IdempotencyKey: idemKey, ContentHash: sum,
		Mode: mode, Filename: cleanName(filename), ByteSize: n,
	})
	if err != nil || replay {
		_ = os.RemoveAll(filepath.Dir(path))
		return imp, err
	}
	s.queue.Enqueue(id)
	return imp, nil
}

// Get returns one job for the user.
func (s *Service) Get(ctx context.Context, userID, id string) (sqlite.Import, error) {
	return s.meta.GetImport(ctx, userID, id)
}

// List returns recent jobs for the user.
func (s *Service) List(ctx context.Context, userID string) ([]sqlite.Import, error) {
	return s.meta.ListImports(ctx, userID)
}

// Retry requeues a failed job whose spool is still on disk.
func (s *Service) Retry(ctx context.Context, userID, id string) (sqlite.Import, error) {
	if _, err := s.meta.GetImport(ctx, userID, id); err != nil {
		return sqlite.Import{}, err
	}
	if _, err := os.Stat(s.payloadPath(id)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sqlite.Import{}, ErrNoSpool
		}
		return sqlite.Import{}, err
	}
	imp, err := s.meta.Requeue(ctx, userID, id)
	if err != nil {
		return sqlite.Import{}, err
	}
	s.queue.Enqueue(id)
	return imp, nil
}

func (s *Service) payloadPath(id string) string {
	return filepath.Join(s.inbox, id, "payload")
}

// ErrNoSpool means the upload file is gone.
var ErrNoSpool = errors.New("no spool")

func writeSpool(path string, r io.Reader) (int64, []byte, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return 0, nil, fmt.Errorf("inbox: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, nil, fmt.Errorf("spool: %w", err)
	}
	h := sha256.New()
	n, copyErr := io.Copy(f, io.TeeReader(r, h))
	closeErr := f.Close()
	if copyErr != nil {
		return n, nil, copyErr
	}
	if closeErr != nil {
		return n, nil, closeErr
	}
	return n, h.Sum(nil), nil
}

func cleanName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "upload"
	}
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}
