package util

import (
	"encoding/base64"
	"strings"
)

// RemoveLeftPaddedZeros is a function that removes the left padded zeros from a large hex string
func RemoveLeftPaddedZeros(str string) string {

	// if string is just 0x, return 0x
	if str == "0x" {
		return "0"
	}

	// If the string is hex, remove the 0x prefix
	str = strings.TrimPrefix(str, "0x")

	// if string is just a bunch of zeros after 0x, return 0
	if strings.ReplaceAll(str, "0", "") == "" {
		return "0"
	}

	for i := 0; i < len(str); i++ {
		if str[i] != '0' {
			return str[i:]
		}
	}
	return str
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
