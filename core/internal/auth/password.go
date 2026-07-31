package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	MinPasswordLength = 15
	MaxPasswordLength = 128
)

var commonPasswords = map[string]struct{}{
	"administrator":       {},
	"passwordpassword":    {},
	"password123456789":   {},
	"qwertyuiopasdfgh":    {},
	"ubnetdefubnetdef":    {},
	"espialespialespial":  {},
	"changemechangeme":    {},
	"letmeinletmein123":   {},
	"correcthorsebattery": {},
}

type PasswordHasher struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

func DefaultPasswordHasher() PasswordHasher {
	return PasswordHasher{
		Memory:      19 * 1024,
		Iterations:  2,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
}

func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if !utf8.ValidString(password) || length < MinPasswordLength || length > MaxPasswordLength {
		return fmt.Errorf("password must contain between %d and %d Unicode characters", MinPasswordLength, MaxPasswordLength)
	}
	if _, blocked := commonPasswords[strings.ToLower(password)]; blocked {
		return errors.New("password is too common")
	}
	return nil
}

func (hasher PasswordHasher) Hash(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}

	salt := make([]byte, hasher.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, hasher.Iterations, hasher.Memory, hasher.Parallelism, hasher.KeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		hasher.Memory,
		hasher.Iterations,
		hasher.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func (hasher PasswordHasher) Verify(password, encoded string) (bool, error) {
	parameters, salt, expected, err := parsePasswordHash(encoded)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey(
		[]byte(password),
		salt,
		parameters.Iterations,
		parameters.Memory,
		parameters.Parallelism,
		uint32(len(expected)),
	)
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func parsePasswordHash(encoded string) (PasswordHasher, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return PasswordHasher{}, nil, nil, errors.New("invalid password hash")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return PasswordHasher{}, nil, nil, errors.New("unsupported password hash version")
	}
	parameters := PasswordHasher{}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &parameters.Memory, &parameters.Iterations, &parameters.Parallelism); err != nil {
		return PasswordHasher{}, nil, nil, errors.New("invalid password hash parameters")
	}
	if parameters.Memory < 8*1024 || parameters.Memory > 1024*1024 ||
		parameters.Iterations < 1 || parameters.Iterations > 10 ||
		parameters.Parallelism < 1 || parameters.Parallelism > 16 {
		return PasswordHasher{}, nil, nil, errors.New("password hash parameters are out of bounds")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 16 || len(salt) > 64 {
		return PasswordHasher{}, nil, nil, errors.New("invalid password hash salt")
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) < 16 || len(expected) > 64 {
		return PasswordHasher{}, nil, nil, errors.New("invalid password hash value")
	}
	return parameters, salt, expected, nil
}
