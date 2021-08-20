package util

import (
	"math/rand"
)

const alphanumeric = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// RandStringBytes returns a random alphanumeric string of given length
func RandStringBytes(n int) string {
    b := make([]byte, n)
    for i := range b {
        b[i] = alphanumeric[rand.Intn(len(alphanumeric))]
    }
    return string(b)
}