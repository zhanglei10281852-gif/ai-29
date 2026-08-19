package domain

import (
	"errors"
	"fmt"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrValidation        = errors.New("validation failed")
	ErrCapacityExceeded  = errors.New("capacity exceeded")
	ErrVersionConflict   = errors.New("version conflict")
	ErrExpired           = errors.New("expired")
	ErrAlreadyExists     = errors.New("already exists")
	ErrUnauthenticated   = errors.New("unauthenticated")
	ErrForbidden         = errors.New("forbidden")
)

type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e FieldError) Unwrap() error { return ErrValidation }

type TransitionError struct {
	Entity string
	From   string
	To     string
}

func (e TransitionError) Error() string {
	return fmt.Sprintf("%s cannot transition from %s to %s", e.Entity, e.From, e.To)
}

func (e TransitionError) Unwrap() error { return ErrInvalidTransition }

type ConflictError struct {
	Resource string
	Reason   string
}

func (e ConflictError) Error() string {
	return fmt.Sprintf("%s conflict: %s", e.Resource, e.Reason)
}

func (e ConflictError) Unwrap() error { return ErrConflict }
