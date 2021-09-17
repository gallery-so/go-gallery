package util

import (
	"errors"
	"math/big"
	"strings"
)

// NormalizeHex converts a hex string with 0x prefix to a consistent hex representation
func NormalizeHex(hex string) (string, error) {
	hex = strings.TrimPrefix(hex, "0x")

	i, ok := new(big.Int).SetString(hex, 16)
	if !ok {
		return "", errors.New("invalid hex")
	}
	return i.Text(16), nil
}
