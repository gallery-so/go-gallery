package inputcheck

import (
	"fmt"

	"github.com/go-playground/validator/v10"
)

type ValWithTags struct {
	Value interface{}
	Tag   string
}

type ValidationMap map[string]ValWithTags

func ValidateFields(validator *validator.Validate, fields ValidationMap) error {
	validationErr := ErrInvalidInput{}
	foundErrors := false

	for k, v := range fields {
		err := validator.Var(v.Value, v.Tag)
		if err != nil {
			foundErrors = true
			validationErr.Append(k, err.Error())
		}
	}

	if foundErrors {
		return validationErr
	}

	return nil
}

type ErrInvalidInput struct {
	Parameters []string
	Reasons    []string
}

func (e *ErrInvalidInput) Append(parameter string, reason string) {
	e.Parameters = append(e.Parameters, parameter)
	e.Reasons = append(e.Reasons, reason)
}

func (e ErrInvalidInput) Error() string {
	str := "invalid input:\n"

	for i := range e.Parameters {
		str += fmt.Sprintf("    parameter: %s, reason: %s\n", e.Parameters[i], e.Reasons[i])
	}

	return str
}
