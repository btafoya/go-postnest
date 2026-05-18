package admin

import (
	"errors"
	"reflect"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/go-postnest/postnest/internal/api"
)

var validate *validator.Validate

func init() {
	validate = validator.New(validator.WithRequiredStructEnabled())
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	_ = validate.RegisterValidation("domainname", func(fl validator.FieldLevel) bool {
		re := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*$`)
		return re.MatchString(fl.Field().String())
	})
}

func mapValidationErrors(err error) []api.FieldError {
	if err == nil {
		return nil
	}
	var ve validator.ValidationErrors
	if !errors.As(err, &ve) {
		return nil
	}
	out := make([]api.FieldError, 0, len(ve))
	for _, e := range ve {
		out = append(out, api.FieldError{
			Field: e.Field(),
			Issue: e.Tag(),
			Param: e.Param(),
		})
	}
	return out
}
