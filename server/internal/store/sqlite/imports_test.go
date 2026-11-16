package sqlite

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateImportIdempotencyAndActiveJob(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	user := mustUser(t, db)
	first, replay, err := db.CreateImport(ctx, Import{
		ID: "imp-1", UserID: user, IdempotencyKey: "key-1", ContentHash: []byte("abc"),
		Mode: ModeReplace, Filename: "export.xml", ByteSize: 3,
	})
	if err != nil || replay || first.Status != StatusAccepted {
		t.Fatalf("create replay=%v err=%v imp=%+v", replay, err, first)
	}
	again, replay, err := db.CreateImport(ctx, Import{
		ID: "imp-2", UserID: user, IdempotencyKey: "key-1", ContentHash: []byte("abc"),
		Mode: ModeReplace, Filename: "export.xml", ByteSize: 3,
	})
	if err != nil || !replay || again.ID != "imp-1" {
		t.Fatalf("replay id=%s replay=%v err=%v", again.ID, replay, err)
	}
	if _, _, err := db.CreateImport(ctx, Import{
		ID: "imp-3", UserID: user, IdempotencyKey: "key-1", ContentHash: []byte("zzz"),
		Mode: ModeReplace, ByteSize: 3,
	}); err != ErrIdempotencyConflict {
		t.Fatalf("conflict err = %v", err)
	}
	if _, _, err := db.CreateImport(ctx, Import{
		ID: "imp-4", UserID: user, Mode: ModeReplace, ByteSize: 1, ContentHash: []byte("q"),
	}); err != ErrImportActive {
		t.Fatalf("active err = %v", err)
	}
}

func openTestDB(t *testing.T) *DB {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "meta.sqlite"), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mustUser(t *testing.T, db *DB) string {
	t.Helper()
	tokenPath := filepath.Join(t.TempDir(), "bootstrap.token")
	if _, err := db.BootstrapToken(context.Background(), tokenPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	token := string(raw[:len(raw)-1])
	userID, err := db.UserIDByToken(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return userID
}
