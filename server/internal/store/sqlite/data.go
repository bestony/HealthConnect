package sqlite

import (
	"context"
	"fmt"
)

// DeleteCharacteristics removes the caller's stored profile attributes.
func (db *DB) DeleteCharacteristics(ctx context.Context, userID string) error {
	if _, err := db.sql.ExecContext(ctx, `DELETE FROM characteristics WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("sqlite delete characteristics: %w", err)
	}
	db.log.Info("characteristics deleted", "user_id", userID)
	return nil
}
