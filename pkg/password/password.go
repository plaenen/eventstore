package password

import (
	"errors"

	passwordvalidator "github.com/wagslane/go-password-validator"
	"golang.org/x/crypto/bcrypt"
)

const (
	MinCost           = 4
	MaxCost           = 31
	DefaultCost       = 12  // Increased from bcrypt.DefaultCost (10) for better security
	MaxPasswordLength = 128 // Prevent DoS via extremely long passwords
)

// Compare compares a hashed password with its possible plaintext equivalent.
// Returns an error if the passwords don't match or if there's an issue with the comparison.
// This approach prevents timing attacks that could leak information about password validity.
func Compare(hashedPassword, password string) error {
	if len(hashedPassword) == 0 {
		return errors.New("hashed password cannot be empty")
	}
	if len(password) == 0 {
		return errors.New("password cannot be empty")
	}
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

type HashOptions struct {
	Cost int
}

type HasOpt func(options *HashOptions)

// WithCost sets the bcrypt cost factor. Values between 4-31 are valid.
// Higher values provide better security but require more computational resources.
func WithCost(cost int) HasOpt {
	return func(options *HashOptions) {
		if cost >= MinCost && cost <= MaxCost {
			options.Cost = cost
		}
		// If invalid cost is provided, keep the default
	}
}

// Hash generates a bcrypt hash of the password with the specified options.
// Returns an error if the password is invalid or hashing fails.
func Hash(password string, opts ...HasOpt) (string, error) {
	// Input validation
	if len(password) == 0 {
		return "", errors.New("password cannot be empty")
	}
	if len(password) > MaxPasswordLength {
		return "", errors.New("password too long")
	}

	options := &HashOptions{
		Cost: DefaultCost,
	}
	for _, opt := range opts {
		opt(options)
	}

	// Validate cost range
	if options.Cost < MinCost || options.Cost > MaxCost {
		return "", errors.New("invalid cost factor")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), options.Cost)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

// ValidateStrength validates that the password meets minimum entropy requirements.
// Returns an error if the password is too weak.
// We could in the future deciede to use additonal checks like https://github.com/dropbox/zxcvbn
func ValidateStrength(password string) error {
	return passwordvalidator.Validate(password, 60) // 60 bits entropy minimum
}
