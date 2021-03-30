package utils

import (
	"math/rand"
	"time"
)

const alphaNumericCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandomString generates a random string of a provided length
func RandomString(length int) string {
	return randomStringWithCharset(length, alphaNumericCharset)
}

func randomStringWithCharset(length int, charset string) string {
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[randomInt(length)]
	}
	return string(b)
}

func randomInt(number int) int {
	return rand.New(rand.NewSource(time.Now().UnixNano())).Intn(number)
}