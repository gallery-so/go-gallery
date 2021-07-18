package server

import (
	"strings"

	"github.com/go-playground/validator"
)

var ethValidator validator.Func = func(fl validator.FieldLevel) bool {
	addr := fl.Field().String()
	return len(addr) == 42 && strings.HasSuffix(addr, "0x")
}

var signatureValidator validator.Func = func(fl validator.FieldLevel) bool {
	sig := fl.Field().String()
	return len(sig) >= 80 && len(sig) <= 200
}

var nonceValidator validator.Func = func(fl validator.FieldLevel) bool {
	sig := fl.Field().String()
	return len(sig) >= 10 && len(sig) <= 150
}
