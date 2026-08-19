package domain

import (
	"fmt"
	"regexp"
	"strings"
)

var identifierPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{1,63}$`)

func NormalizeCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func ValidateBusinessCode(field, value string) error {
	normalized := NormalizeCode(value)
	if !identifierPattern.MatchString(normalized) {
		return FieldError{Field: field, Message: "must contain 2-64 uppercase letters, digits, dots, underscores or dashes"}
	}
	return nil
}

func ValidateIdempotencyKey(value string) error {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 8 || len(trimmed) > 128 {
		return FieldError{Field: "idempotency_key", Message: "must be between 8 and 128 characters"}
	}
	if strings.ContainsAny(trimmed, "\r\n") {
		return FieldError{Field: "idempotency_key", Message: "must not contain line breaks"}
	}
	return nil
}

func ValidateRequestID(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > 128 {
		return FieldError{Field: "request_id", Message: "must be present and no longer than 128 characters"}
	}
	if strings.ContainsAny(trimmed, "\r\n") {
		return FieldError{Field: "request_id", Message: "must not contain line breaks"}
	}
	return nil
}

func DescribeValidation(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%v", err)
}
