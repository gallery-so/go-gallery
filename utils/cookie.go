package utils

import (
	"net/http"
	"time"
)

// AddCookie applies a cookie to the response of an HTTP request
func AddCookie(w http.ResponseWriter, key string, value string) {
	expiry := time.Now().Add(time.Minute * 10)
	cookie := http.Cookie{
		Name:    key,
		Value:   value,
		Expires: expiry,
		MaxAge:  60 * 10,
	}
	http.SetCookie(w, &cookie)
}

// RemoveCookie removes a cookie with a given key
func RemoveCookie(w http.ResponseWriter, key string) {
	deletion := http.Cookie{
		Name:   key,
		Value:  "",
		MaxAge: -1,
	}
	http.SetCookie(w, &deletion)
}