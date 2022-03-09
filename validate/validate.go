package validate

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/microcosm-cc/bluemonday"
)

var bannedUsernames = map[string]bool{
	"password":      true,
	"auth":          true,
	"welcome":       true,
	"edit":          true,
	"404":           true,
	"nuke":          true,
	"account":       true,
	"settings":      true,
	"artists":       true,
	"artist":        true,
	"collections":   true,
	"collection":    true,
	"nft":           true,
	"members":       true,
	"nfts":          true,
	"bookmarks":     true,
	"messages":      true,
	"guestbook":     true,
	"notifications": true,
	"explore":       true,
	"analytics":     true,
	"gallery":       true,
	"investors":     true,
	"team":          true,
	"faq":           true,
	"info":          true,
	"about":         true,
	"contact":       true,
	"terms":         true,
	"privacy":       true,
	"help":          true,
	"support":       true,
	"feed":          true,
	"feeds":         true,
	"membership":    true,
}

var alphanumericUnderscoresPeriodsRegex = regexp.MustCompile("^[\\w.]*$")

// SanitizationPolicy is a policy for sanitizing user input
var SanitizationPolicy = bluemonday.UGCPolicy()

func RegisterCustomValidators(v *validator.Validate) {
	v.RegisterValidation("eth_addr", EthValidator)
	v.RegisterValidation("nonce", NonceValidator)
	v.RegisterValidation("signature", SignatureValidator)
	v.RegisterValidation("username", UsernameValidator)
	v.RegisterValidation("max_string_length", MaxStringLengthValidator)
	v.RegisterAlias("medium", "max_string_length=600")
	v.RegisterAlias("collectors_note", "max_string_length=1200")
	v.RegisterAlias("collection_name", "max_string_length=200")
	v.RegisterAlias("collection_note", "max_string_length=600")
	v.RegisterAlias("nft_note", "max_string_length=1200")
	v.RegisterAlias("bio", "max_string_length=600")
}

// EthValidator validates ethereum addresses
var EthValidator validator.Func = func(fl validator.FieldLevel) bool {
	addr := fl.Field().String()
	if addr == "" {
		return true
	}
	return len(addr) == 42 && strings.HasPrefix(addr, "0x")
}

// SignatureValidator validates ethereum wallet signed messages
var SignatureValidator validator.Func = func(fl validator.FieldLevel) bool {
	sig := fl.Field().String()
	if sig == "" {
		return true
	}
	return len(sig) >= 80 && len(sig) <= 200
}

// NonceValidator validates nonces generated by the app
var NonceValidator validator.Func = func(fl validator.FieldLevel) bool {
	nonce := fl.Field().String()
	if nonce == "" {
		return true
	}
	return len(nonce) >= 10 && len(nonce) <= 150
}

// MaxStringLengthValidator validates strings with a given maximum length
var MaxStringLengthValidator validator.Func = func(fl validator.FieldLevel) bool {
	s := fl.Field().String()

	maxLength, err := strconv.Atoi(fl.Param())
	if err != nil {
		panic(fmt.Errorf("error parsing MaxStringLengthValidator parameter: %s", err))
	}

	return len(s) <= maxLength
}

// UsernameValidator ensures that usernames are not reserved, are alphanumeric with the exception of underscores and periods, and do not contain consecutive periods or underscores
var UsernameValidator validator.Func = func(fl validator.FieldLevel) bool {
	s := fl.Field().String()
	if s == "" {
		return true
	}
	if _, ok := bannedUsernames[s]; ok {
		return false
	}
	return len(s) >= 2 && len(s) <= 50 &&
		alphanumericUnderscoresPeriodsRegex.MatchString(s) &&
		!consecutivePeriodsOrUnderscores(s)
}

func consecutivePeriodsOrUnderscores(s string) bool {
	for i, r := range s {
		if r == '.' || r == '_' {
			if i > 0 && (rune(s[i-1]) == '.' || rune(s[i-1]) == '_') {
				return true
			}
			if i < len(s)-1 && (rune(s[i+1]) == '.' || rune(s[i+1]) == '_') {
				return true
			}
		}
	}
	return false
}
