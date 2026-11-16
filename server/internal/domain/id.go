package domain

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// NewID returns a random UUID string.
func NewID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("uuid: %w", err)
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
	return string(buf[:]), nil
}
