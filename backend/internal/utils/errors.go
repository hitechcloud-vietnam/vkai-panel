package utils

import (
	"errors"
	"fmt"
)

// Common errors
var (
	ErrNotFound          = errors.New("resource not found")
	ErrAlreadyExists     = errors.New("resource already exists")
	ErrInvalidInput      = errors.New("invalid input")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrInternal          = errors.New("internal error")
	ErrDatabase          = errors.New("database error")
	ErrValidation        = errors.New("validation error")
	ErrConflict          = errors.New("conflict")
	ErrTooManyRequests   = errors.New("too many requests")
	ErrServiceUnavailable = errors.New("service unavailable")
)

// AppError represents an application error
type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *AppError) Unwrap() error {
	return e.Err
}

// NewAppError creates a new AppError
func NewAppError(code, message string, err error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Err:     err,
	}
}

// NotFoundError creates a not found error
func NotFoundError(resource string) *AppError {
	return &AppError{
		Code:    "NOT_FOUND",
		Message: fmt.Sprintf("%s not found", resource),
		Err:     ErrNotFound,
	}
}

// AlreadyExistsError creates an already exists error
func AlreadyExistsError(resource string) *AppError {
	return &AppError{
		Code:    "ALREADY_EXISTS",
		Message: fmt.Sprintf("%s already exists", resource),
		Err:     ErrAlreadyExists,
	}
}

// NewValidationError creates a validation error
func NewValidationError(message string) *AppError {
	return &AppError{
		Code:    "VALIDATION_ERROR",
		Message: message,
		Err:     ErrValidation,
	}
}

// DatabaseError creates a database error
func DatabaseError(err error) *AppError {
	return &AppError{
		Code:    "DATABASE_ERROR",
		Message: "A database error occurred",
		Err:     err,
	}
}

// NewInternalError creates an internal error
func NewInternalError(err error) *AppError {
	return &AppError{
		Code:    "INTERNAL_ERROR",
		Message: "An internal error occurred",
		Err:     err,
	}
}

// UnauthorizedError creates an unauthorized error
func UnauthorizedError(message string) *AppError {
	return &AppError{
		Code:    "UNAUTHORIZED",
		Message: message,
		Err:     ErrUnauthorized,
	}
}

// ForbiddenError creates a forbidden error
func ForbiddenError(message string) *AppError {
	return &AppError{
		Code:    "FORBIDDEN",
		Message: message,
		Err:     ErrForbidden,
	}
}

// ConflictError creates a conflict error
func ConflictError(message string) *AppError {
	return &AppError{
		Code:    "CONFLICT",
		Message: message,
		Err:     ErrConflict,
	}
}

// IsNotFound checks if an error is a not found error
func IsNotFound(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == "NOT_FOUND"
	}
	return errors.Is(err, ErrNotFound)
}

// IsAlreadyExists checks if an error is an already exists error
func IsAlreadyExists(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == "ALREADY_EXISTS"
	}
	return errors.Is(err, ErrAlreadyExists)
}

// IsValidation checks if an error is a validation error
func IsValidation(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == "VALIDATION_ERROR"
	}
	return errors.Is(err, ErrValidation)
}

// IsDatabase checks if an error is a database error
func IsDatabase(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == "DATABASE_ERROR"
	}
	return errors.Is(err, ErrDatabase)
}

// IsUnauthorized checks if an error is an unauthorized error
func IsUnauthorized(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == "UNAUTHORIZED"
	}
	return errors.Is(err, ErrUnauthorized)
}

// IsForbidden checks if an error is a forbidden error
func IsForbidden(err error) bool {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.Code == "FORBIDDEN"
	}
	return errors.Is(err, ErrForbidden)
}
