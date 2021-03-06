package util

import (
	"math/big"
	"strings"
)

type errInvalidHex struct {
	Hex string
}

// NormalizeHexString converts a hex string with 0x prefix to a consistent hex string representation with no prefix
func NormalizeHexString(hex string) (string, error) {
	hex = strings.TrimPrefix(hex, "0x")

	i, ok := new(big.Int).SetString(hex, 16)
	if !ok {
		return "", errInvalidHex{hex}
	}
	return i.Text(16), nil
}

// HexToBigInt converts a hex string with 0x prefix to a big int
func HexToBigInt(hex string) (*big.Int, error) {
	hex = strings.TrimPrefix(hex, "0x")

	i, ok := new(big.Int).SetString(hex, 16)
	if !ok {
		return nil, errInvalidHex{hex}
	}
	return i, nil
}

func (e errInvalidHex) Error() string {
	return "invalid hex: " + e.Hex
}

// RemoveLeftPaddedZeros is a function that removes the left padded zeros from a large hex string
func RemoveLeftPaddedZeros(hex string) string {
	hex = strings.TrimPrefix(hex, "0x")
	for i := 0; i < len(hex); i++ {
		if hex[i] != '0' {
			return hex[i:]
		}
	}
	return hex
}
