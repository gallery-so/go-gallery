package main

import (
	"net/http"

	"github.com/mikeydub/go-gallery/feedbot"
)

func main() {
	feedbot.Init()
	http.ListenAndServe(":4123", nil)
}
