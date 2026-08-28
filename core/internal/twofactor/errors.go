package twofactor

import "errors"

// Errors returned by the service. They are deliberately coarse where a finer
// distinction would tell an attacker something: a wrong password, a wrong code
// and a spent recovery code all surface as ErrVerificationFailed.
var (
	// ErrInvalidSecret means a stored or submitted base32 secret is unusable.
	ErrInvalidSecret = errors.New("two-factor secret is not valid base32")

	// ErrNotEnrolled means the account has no confirmed second factor.
	ErrNotEnrolled = errors.New("two-factor authentication is not enabled for this account")

	// ErrNoPendingEnrolment means confirmation was attempted without a live
	// enrolment, or after the enrolment window expired.
	ErrNoPendingEnrolment = errors.New("no pending two-factor enrolment; start enrolment again")

	// ErrAlreadyEnabled means enrolment was started on an account that already
	// has a confirmed second factor. Re-enrolling must go through disable
	// first, which requires the password and a current code.
	ErrAlreadyEnabled = errors.New("two-factor authentication is already enabled")

	// ErrVerificationFailed is the single answer for every failed credential
	// check.
	ErrVerificationFailed = errors.New("verification failed")

	// ErrLockedOut means too many failed verifications; the account is in a
	// cool-down.
	ErrLockedOut = errors.New("too many failed verification attempts; try again later")

	// ErrRateLimited means the shared rate limiter rejected the attempt.
	ErrRateLimited = errors.New("too many requests; slow down")

	// ErrCodeReplayed means the submitted code was correct but its time step
	// has already been spent by an earlier request.
	ErrCodeReplayed = errors.New("this code has already been used")

	// ErrSecretUnreadable means the stored ciphertext could not be decrypted,
	// which in practice means the encryption key changed or the row was
	// tampered with.
	ErrSecretUnreadable = errors.New("stored two-factor secret cannot be decrypted")
)
