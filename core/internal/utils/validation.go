package utils

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

// Error implements the error interface
func (ve ValidationErrors) Error() string {
	var messages []string
	for _, e := range ve {
		messages = append(messages, e.Field+": "+e.Message)
	}
	return strings.Join(messages, "; ")
}

// ValidateRequired validates that a field is not empty
func ValidateRequired(value, fieldName string) *ValidationError {
	if strings.TrimSpace(value) == "" {
		return &ValidationError{
			Field:   fieldName,
			Message: fieldName + " is required",
		}
	}
	return nil
}

// ValidateEmail validates email format
func ValidateEmail(email, fieldName string) *ValidationError {
	if email == "" {
		return nil
	}

	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return &ValidationError{
			Field:   fieldName,
			Message: fieldName + " must be a valid email address",
		}
	}
	return nil
}

// ValidateMinLength validates minimum string length
func ValidateMinLength(value, fieldName string, minLength int) *ValidationError {
	if len(value) < minLength {
		return &ValidationError{
			Field:   fieldName,
			Message: fieldName + " must be at least " + string(rune(minLength+'0')) + " characters long",
		}
	}
	return nil
}

// ValidateMaxLength validates maximum string length
func ValidateMaxLength(value, fieldName string, maxLength int) *ValidationError {
	if len(value) > maxLength {
		return &ValidationError{
			Field:   fieldName,
			Message: fieldName + " must be at most " + string(rune(maxLength+'0')) + " characters long",
		}
	}
	return nil
}

// ValidatePasswordStrength validates password strength
func ValidatePasswordStrength(password, fieldName string) *ValidationError {
	if password == "" {
		return nil
	}

	var (
		hasMinLen  = len(password) >= 8
		hasUpper   = false
		hasLower   = false
		hasNumber  = false
		hasSpecial = false
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	if !hasMinLen || !hasUpper || !hasLower || !hasNumber || !hasSpecial {
		return &ValidationError{
			Field:   fieldName,
			Message: fieldName + " must be at least 8 characters long and contain uppercase, lowercase, number, and special character",
		}
	}
	return nil
}

// ValidateDomain validates domain format
func ValidateDomain(domain, fieldName string) *ValidationError {
	if domain == "" {
		return nil
	}

	domainRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?)*\.[a-zA-Z]{2,}$`)
	if !domainRegex.MatchString(domain) {
		return &ValidationError{
			Field:   fieldName,
			Message: fieldName + " must be a valid domain name",
		}
	}
	return nil
}

// ValidateIP validates IP address format
func ValidateIP(ip, fieldName string) *ValidationError {
	if ip == "" {
		return nil
	}

	ipRegex := regexp.MustCompile(`^((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)$`)
	if !ipRegex.MatchString(ip) {
		return &ValidationError{
			Field:   fieldName,
			Message: fieldName + " must be a valid IP address",
		}
	}
	return nil
}

// ValidatePort validates port number
func ValidatePort(port int, fieldName string) *ValidationError {
	if port < 1 || port > 65535 {
		return &ValidationError{
			Field:   fieldName,
			Message: fieldName + " must be between 1 and 65535",
		}
	}
	return nil
}

// ValidateURL validates URL format
func ValidateURL(url, fieldName string) *ValidationError {
	if url == "" {
		return nil
	}

	urlRegex := regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)
	if !urlRegex.MatchString(url) {
		return &ValidationError{
			Field:   fieldName,
			Message: fieldName + " must be a valid URL",
		}
	}
	return nil
}

// ValidateCronExpression validates cron expression format
func ValidateCronExpression(expr, fieldName string) *ValidationError {
	if expr == "" {
		return nil
	}

	// Basic cron validation (5 or 6 fields)
	parts := strings.Fields(expr)
	if len(parts) < 5 || len(parts) > 6 {
		return &ValidationError{
			Field:   fieldName,
			Message: fieldName + " must be a valid cron expression",
		}
	}
	return nil
}

// ValidateAndRespond validates a struct and returns errors if any
func ValidateAndRespond(c *gin.Context, errors ValidationErrors) bool {
	if len(errors) > 0 {
		BadRequest(c, errors.Error())
		return false
	}
	return true
}

// SanitizeString removes potentially dangerous characters
func SanitizeString(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Trim whitespace
	input = strings.TrimSpace(input)

	return input
}

// SanitizeHTML removes HTML tags
func SanitizeHTML(input string) string {
	// Simple HTML tag removal
	htmlRegex := regexp.MustCompile(`<[^>]*>`)
	return htmlRegex.ReplaceAllString(input, "")
}
