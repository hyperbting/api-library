package validator

import "github.com/go-playground/validator/v10"

var v *validator.Validate

func init() {
	v = validator.New()
	// Register any custom rules here once during startup
}

// Struct validates any struct using the shared validator instance
func Struct(s interface{}) error {
	return v.Struct(s)
}
