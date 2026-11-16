package sqlite

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMigrateAndBootstrap(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "meta.sqlite")
	tokenPath := filepath.Join(dir, "bootstrap.token")
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	db, err := Open(ctx, path, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}

	created, err := db.BootstrapToken(ctx, tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("expected token creation")
	}
	raw, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	token := string(raw)
	if len(token) < 8 || token[len(token)-1] != '\n' {
		t.Fatalf("token file = %q", token)
	}
	token = token[:len(token)-1]
	userID, err := db.UserIDByToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if userID == "" {
		t.Fatal("empty user id")
	}
	if _, err := db.UserIDByToken(ctx, "hc_nope"); err != ErrNotFound {
		t.Fatalf("bad token err = %v", err)
	}

	created, err = db.BootstrapToken(ctx, tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("second bootstrap created a token")
	}
	again, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != string(raw) {
		t.Fatal("bootstrap rotated the token file")
	}

	var tables int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name IN ('users','api_tokens','imports','characteristics','exports')`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 5 {
		t.Fatalf("tables = %d", tables)
	}

	otherID, otherToken, err := db.MintToken(ctx, "other")
	if err != nil {
		t.Fatal(err)
	}
	if otherID == userID {
		t.Fatal("mint reused the bootstrap user")
	}
	got, err := db.UserIDByToken(ctx, otherToken)
	if err != nil || got != otherID {
		t.Fatalf("mint lookup = %q %v", got, err)
	}
	if err := db.ReplaceCharacteristics(ctx, otherID, "import", map[string]string{
		"HKCharacteristicTypeIdentifierBiologicalSex": "HKBiologicalSexFemale",
		"HKCharacteristicTypeIdentifierDateOfBirth":   "2001-02-03",
	}); err != nil {
		t.Fatal(err)
	}
	items, err := db.ListCharacteristics(ctx, otherID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Key != "HKCharacteristicTypeIdentifierBiologicalSex" || items[1].Value != "2001-02-03" {
		t.Fatalf("characteristics = %+v", items)
	}
	ownerItems, err := db.ListCharacteristics(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ownerItems) != 0 {
		t.Fatalf("owner characteristics = %+v", ownerItems)
	}
}
