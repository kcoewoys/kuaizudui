package domain

import "errors"

var (
	ErrInvalidInput       = errors.New("invalid input")
	ErrNotFound           = errors.New("not found")
	ErrConflict           = errors.New("conflict")
	ErrForbidden          = errors.New("forbidden")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrQueueEmpty         = errors.New("queue empty")
	ErrAlreadyUsed        = errors.New("already used")
	ErrCannotUseOwn       = errors.New("cannot use own content")
	ErrInsufficientPoints = errors.New("insufficient points")
)

type FieldError struct {
	Field   string
	Message string
}

func (e FieldError) Error() string { return e.Field + ": " + e.Message }
func (e FieldError) Unwrap() error { return ErrInvalidInput }
