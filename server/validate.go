package server

import (
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"
)

var alphanumericUnderscoresPeriodsRegex = regexp.MustCompile(`/^[\w.]+$/i`)
var consecutivePeriodsUnderscoresRegex = regexp.MustCompile(`/^[\w.]+$/i`)
var sanitizationPolicy = bluemonday.UGCPolicy()

var ethValidator validator.Func = func(fl validator.FieldLevel) bool {
	addr := fl.Field().String()
	if addr == "" {
		return true
	}
	return len(addr) == 42 && strings.HasPrefix(addr, "0x")
}

var signatureValidator validator.Func = func(fl validator.FieldLevel) bool {
	sig := fl.Field().String()
	if sig == "" {
		return true
	}
	return len(sig) >= 80 && len(sig) <= 200
}

var nonceValidator validator.Func = func(fl validator.FieldLevel) bool {
	nonce := fl.Field().String()
	if nonce == "" {
		return true
	}
	return len(nonce) >= 10 && len(nonce) <= 150
}

var shortStringValidator validator.Func = func(fl validator.FieldLevel) bool {
	s := fl.Field().String()
	return len(s) > 4 && len(s) < 50
}

var mediumStringValidator validator.Func = func(fl validator.FieldLevel) bool {
	s := fl.Field().String()
	return len(s) < 500
}

var usernameValidator validator.Func = func(fl validator.FieldLevel) bool {
	s := fl.Field().String()
	if s == "" {
		return true
	}
	return len(s) >= 2 && len(s) <= 50 &&
		alphanumericUnderscoresPeriodsRegex.MatchString(s) &&
		consecutivePeriodsUnderscoresRegex.MatchString(s)
}
