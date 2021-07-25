package server

import (
	"testing"

	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
)

type testValue struct{
	value string
	description string
	shouldPassValidation bool
}

func TestValidate_ethValidator(pTest *testing.T) {
	var testEthAddresses = []testValue {
		{"0xZ493i1403D4aa1DF657a8712ED255B11Z61n42Z9", "Valid address", true},
		{"11Z493i1403D4aa1DF657a8712ED255B11Z61n42Z9", "Missing 0x prefix", false},
		{"0x57a8712ED255B11Z61n42Z9", "Too short", false},
		{"0xZ493i1403D4aa1DF657a8712ED255B11Z61n42Z919xZZ", "Too long", false},
	}
	testValidatorWithTestValues(pTest, ethValidator, testEthAddresses)
}

func TestValidate_signatureValidator(pTest *testing.T) {
	var testSignatures = []testValue {
		{"91Z493i1403D4aa1DF657a8712ED255B11Z61n42Z991Z493i1403D4aa1DF657a8712ED255B11Z61n42Z9", "Valid signature", true},
		{"11Z493i1403D4aa1DF657a", "Too short", false},
		{"255B11Z61n42Z90x57a8712ED255B11Z61n42Z90x57321412312412412fds9d9as87fd69012890123a871z"+
		"2ED255B11Z61n42Z957a8712ED255B11Z61n42Z90x57a8712ED255B11Z61n42Z90x57a8712ED255B11Z61n42Z90x57a8712ED255B11Z61n42Z9", "Too long", false},
	}
	testValidatorWithTestValues(pTest, signatureValidator, testSignatures)
}

func testValidatorWithTestValues(pTest *testing.T, validatorFunc validator.Func, testValues []testValue) {
	validate := validator.New()
	validate.RegisterValidation("validatorName", validatorFunc)

	for _, item := range testValues {
		err := validate.Var(item.value, "validatorName")
		if item.shouldPassValidation {
			assert.Nil(pTest, err, item.description)
		} else {
			assert.Error(pTest, err, item.description)
		}
	}
}
