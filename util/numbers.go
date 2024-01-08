package util

import (
	"encoding/base64"
	"strings"
)

// RemoveLeftPaddedZeros is a function that removes the left padded zeros from a large hex string
func RemoveLeftPaddedZeros(hex string) string {

	// if string is just 0x, return 0x
	if hex == "0x" {
		return "0"
	}

	hex = strings.TrimPrefix(hex, "0x")

	// if string is just a bunch of zeros after 0x, return 0
	if strings.ReplaceAll(hex, "0", "") == "" {
		return "0"
	}

	for i := 0; i < len(hex); i++ {
		if hex[i] != '0' {
			return hex[i:]
		}
	}
	return hex
}

func Base64Decode(s string, encodings ...*base64.Encoding) ([]byte, error) {
	var lastError error
	for _, encoding := range encodings {
		bs, err := encoding.DecodeString(s)
		if err == nil {
			return bs, nil
		}
		lastError = err
	}
	return nil, lastError
}
