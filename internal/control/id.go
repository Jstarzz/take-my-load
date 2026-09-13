package control

import (
	"crypto/rand"
	"encoding/hex"
)

func newID() (string, error) {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
