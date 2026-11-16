package sqlite

import (
	"context"
	"fmt"
)

// Characteristic is one stored profile attribute.
type Characteristic struct {
	Key   string
	Value string
}

// ListCharacteristics returns the caller's profile attributes in key order.
func (db *DB) ListCharacteristics(ctx context.Context, userID string) ([]Characteristic, error) {
	rows, err := db.sql.QueryContext(ctx, `
		SELECT attr_key, attr_value FROM characteristics
		WHERE user_id = ? ORDER BY attr_key`, userID)
	if err != nil {
		return nil, fmt.Errorf("sqlite list characteristics: %w", err)
	}
	defer rows.Close()
	var out []Characteristic
	for rows.Next() {
		var item Characteristic
		if err := rows.Scan(&item.Key, &item.Value); err != nil {
			return nil, fmt.Errorf("sqlite list characteristics: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite list characteristics: %w", err)
	}
	return out, nil
}

// DeleteCharacteristics removes the caller's stored profile attributes.
func (db *DB) DeleteCharacteristics(ctx context.Context, userID string) error {
	if _, err := db.sql.ExecContext(ctx, `DELETE FROM characteristics WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("sqlite delete characteristics: %w", err)
	}
	db.log.Info("characteristics deleted", "user_id", userID)
	return nil
}
