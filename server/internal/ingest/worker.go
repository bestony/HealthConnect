package ingest

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"com.bestony.health-connect/internal/domain"
	"com.bestony.health-connect/internal/ingest/applexml"
	"com.bestony.health-connect/internal/store/duckdb"
	"com.bestony.health-connect/internal/store/sqlite"
)

// Worker parses one import at a time.
type Worker struct {
	jobs   chan string
	meta   *sqlite.DB
	facts  *duckdb.DB
	inbox  string
	limits Limits
	log    *slog.Logger
}

// NewWorker builds a worker. The queue capacity is 8.
func NewWorker(meta *sqlite.DB, facts *duckdb.DB, inbox string, limits Limits, log *slog.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	if limits.MaxEntries == 0 {
		limits.MaxEntries = 10000
	}
	return &Worker{
		jobs: make(chan string, 8), meta: meta, facts: facts,
		inbox: inbox, limits: limits, log: log,
	}
}

// Enqueue schedules an import. It blocks when the queue is full.
func (w *Worker) Enqueue(id string) { w.jobs <- id }

// Start reads the queue until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case id := <-w.jobs:
				w.run(ctx, id)
			}
		}
	}()
}

func (w *Worker) run(ctx context.Context, id string) {
	imp, err := w.meta.GetImportByID(ctx, id)
	if err != nil {
		w.log.Error("import lookup failed", "import_id", id, "error_code", "commit_failed")
		return
	}
	if imp.Status != sqlite.StatusAccepted {
		return
	}
	if err := w.meta.MarkRunning(ctx, id, phaseUnpack); err != nil {
		w.log.Error("import mark running failed", "import_id", id, "error_code", "commit_failed")
		return
	}
	if err := w.process(ctx, imp); err != nil {
		if ctx.Err() != nil {
			_ = w.meta.MarkFailed(context.Background(), id, "interrupted")
		} else {
			_ = w.meta.MarkFailed(context.Background(), id, codeOf(err))
		}
		_ = w.facts.DeleteStaging(context.Background(), id)
		return
	}
}

func (w *Worker) process(ctx context.Context, imp sqlite.Import) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	payload := filepath.Join(w.inbox, imp.ID, "payload")
	xmlReader, err := OpenXML(payload, w.limits)
	if err != nil {
		return err
	}
	defer xmlReader.Close()
	if err := w.meta.UpdateProgress(ctx, imp.ID, phaseParse, 0); err != nil {
		return err
	}
	var written int64
	var chars map[string]string
	var exportAt time.Time
	var sawExport bool
	var rows int64
	err = w.facts.WithLoader(ctx, func(loader *duckdb.Loader) error {
		sink := &sink{loader: loader, meta: w.meta, importID: imp.ID, userID: imp.UserID, activities: map[string]domain.ActivitySummary{}}
		if err := applexml.Parse(xmlReader, sink); err != nil {
			return err
		}
		for _, item := range sink.activities {
			if err := loader.AppendActivity(imp.UserID, imp.ID, item); err != nil {
				return err
			}
		}
		if err := w.meta.UpdateProgress(ctx, imp.ID, phaseParse, sink.rows); err != nil {
			return err
		}
		chars = sink.chars
		exportAt = sink.exportAt
		sawExport = sink.sawExport
		rows = sink.rows
		return nil
	})
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := w.meta.UpdateProgress(ctx, imp.ID, phaseCommit, rows); err != nil {
		return err
	}
	written, err = w.facts.CommitImport(ctx, imp.UserID, imp.ID, imp.Mode)
	if err != nil {
		return err
	}
	if chars != nil || imp.Mode == sqlite.ModeReplace {
		if chars == nil {
			chars = map[string]string{}
		}
		if err := w.meta.ReplaceCharacteristics(ctx, imp.UserID, imp.ID, chars); err != nil {
			return err
		}
	}
	if sawExport {
		if err := w.meta.SetExportDate(ctx, imp.ID, exportAt); err != nil {
			return err
		}
	}
	if err := w.meta.MarkSucceeded(ctx, imp.ID, written); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Dir(payload)); err != nil {
		w.log.Error("inbox cleanup failed", "import_id", imp.ID, "error_code", "commit_failed")
	}
	w.log.Info("import stored", "import_id", imp.ID, "user_id", imp.UserID, "rows_written", written)
	return nil
}

func codeOf(err error) string {
	var parsed *applexml.Error
	if errors.As(err, &parsed) {
		return parsed.Error()
	}
	var coded *Error
	if errors.As(err, &coded) {
		return coded.Error()
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no space") || strings.Contains(msg, "disk full") {
		return CodeDiskFull
	}
	return CodeCommitFailed
}

type sink struct {
	loader     *duckdb.Loader
	meta       *sqlite.DB
	importID   string
	userID     string
	rows       int64
	chars      map[string]string
	exportAt   time.Time
	sawExport  bool
	activities map[string]domain.ActivitySummary
}

func (s *sink) OnExportDate(t time.Time) error {
	s.exportAt = t
	s.sawExport = true
	return nil
}

func (s *sink) OnCharacteristics(attrs map[string]string) error {
	s.chars = attrs
	return nil
}

func (s *sink) OnIgnored(string) error { return nil }

func (s *sink) OnQuantity(sample domain.QuantitySample) error {
	s.rows++
	if err := s.tick(); err != nil {
		return err
	}
	return s.loader.AppendQuantity(s.userID, s.importID, sample)
}

func (s *sink) OnCategory(sample domain.CategorySample) error {
	s.rows++
	if err := s.tick(); err != nil {
		return err
	}
	return s.loader.AppendCategory(s.userID, s.importID, sample)
}

func (s *sink) OnCorrelation(item domain.Correlation) error {
	s.rows += 1 + int64(len(item.Quantities)+len(item.Categories))
	if err := s.tick(); err != nil {
		return err
	}
	return s.loader.AppendCorrelation(s.userID, s.importID, item)
}

func (s *sink) OnWorkout(item domain.Workout) error {
	s.rows++
	if err := s.tick(); err != nil {
		return err
	}
	return s.loader.AppendWorkout(s.userID, s.importID, item)
}

func (s *sink) OnActivity(item domain.ActivitySummary) error {
	s.rows++
	s.activities[item.Day.UTC().Format("2006-01-02")] = item
	return s.tick()
}

func (s *sink) tick() error {
	if s.rows == 0 || s.rows%100000 != 0 {
		return nil
	}
	return s.meta.UpdateProgress(context.Background(), s.importID, phaseParse, s.rows)
}
