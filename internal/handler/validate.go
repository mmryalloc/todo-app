package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/go-playground/validator/v10/non-standard/validators"
)

var validate = validator.New()
var hexRGBPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func init() {
	validate.RegisterValidation("notblank", validators.NotBlank)
	validate.RegisterValidation("hexrgb", func(fl validator.FieldLevel) bool {
		return hexRGBPattern.MatchString(fl.Field().String())
	})
}

func bind(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := readJSON(r, dst); err != nil {
		badRequest(w, errorCodeBadRequest, "invalid request body", []errorDetail{
			{Message: err.Error()},
		})
		return false
	}

	details, err := validateStruct(dst)
	if err != nil {
		slog.Error("validate request", "error", err)
		internalError(w, "failed to validate request")
		return false
	}
	if len(details) > 0 {
		unprocessableEntity(w, details)
		return false
	}

	return true
}

func validateStruct(v any) ([]errorDetail, error) {
	err := validate.Struct(v)
	if err == nil {
		return nil, nil
	}

	errs, ok := err.(validator.ValidationErrors)
	if !ok {
		return nil, fmt.Errorf("validate struct: %w", err)
	}

	details := make([]errorDetail, 0, len(errs))
	for _, e := range errs {
		details = append(details, errorDetail{
			Field:   strings.ToLower(e.Field()),
			Rule:    e.Tag(),
			Message: formatError(e),
		})
	}
	return details, nil
}

func formatError(e validator.FieldError) string {
	switch e.Tag() {
	case "required":
		return fmt.Sprintf("%s is required", e.Field())
	case "notblank":
		return fmt.Sprintf("%s must not be blank", e.Field())
	case "max":
		return fmt.Sprintf("%s must be at most %s characters", e.Field(), e.Param())
	case "min":
		return fmt.Sprintf("%s must be at least %s characters", e.Field(), e.Param())
	case "email":
		return fmt.Sprintf("%s must be a valid email", e.Field())
	case "hexrgb":
		return fmt.Sprintf("%s must be a valid hex color", e.Field())
	default:
		return fmt.Sprintf("%s is invalid", e.Field())
	}
}
