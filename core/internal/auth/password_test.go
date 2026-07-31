package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hasher := testPasswordHasher()
	hash, err := hasher.Hash("a long local password")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	valid, err := hasher.Verify("a long local password", hash)
	if err != nil || !valid {
		t.Fatalf("verify valid password: valid=%t err=%v", valid, err)
	}
	valid, err = hasher.Verify("a different password", hash)
	if err != nil || valid {
		t.Fatalf("verify wrong password: valid=%t err=%v", valid, err)
	}
}

func TestPasswordHashesUseUniqueSalts(t *testing.T) {
	hasher := testPasswordHasher()
	first, err := hasher.Hash("a long local password")
	if err != nil {
		t.Fatal(err)
	}
	second, err := hasher.Hash("a long local password")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("password hashes are identical")
	}
}

func TestPasswordPolicy(t *testing.T) {
	for _, password := range []string{"short", "passwordpassword", strings.Repeat("a", MaxPasswordLength+1)} {
		if err := ValidatePassword(password); err == nil {
			t.Fatalf("password %q was accepted", password)
		}
	}
	if err := ValidatePassword("correct horse battery staple"); err != nil {
		t.Fatalf("valid passphrase rejected: %v", err)
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	if _, err := testPasswordHasher().Verify("a long local password", "$argon2id$broken"); err == nil {
		t.Fatal("malformed hash was accepted")
	}
}

func testPasswordHasher() PasswordHasher {
	return PasswordHasher{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 16}
}
