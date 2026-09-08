package auth

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "testPassword123!"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hash == password {
		t.Error("Hash should not equal plain password")
	}

	if len(hash) == 0 {
		t.Error("Hash should not be empty")
	}
}

func TestHashPasswordTooShort(t *testing.T) {
	password := "short"

	_, err := HashPassword(password)
	if err == nil {
		t.Error("HashPassword should fail for passwords shorter than 8 characters")
	}
}

func TestVerifyPassword(t *testing.T) {
	password := "testPassword123!"
	hash, _ := HashPassword(password)

	if err := VerifyPassword(password, hash); err != nil {
		t.Errorf("VerifyPassword failed for correct password: %v", err)
	}

	if err := VerifyPassword("wrongPassword", hash); err == nil {
		t.Error("VerifyPassword should fail for wrong password")
	}
}

func TestValidatePasswordStrength(t *testing.T) {
	tests := []struct {
		password  string
		shouldErr bool
	}{
		{"short", true},
		{"exactly8", false},
		{"verylongpassword", false},
		{"P@ssw0rd!", false},
		{"1234567", true},
		{"12345678", false},
	}

	for _, tt := range tests {
		err := ValidatePasswordStrength(tt.password)
		if tt.shouldErr && err == nil {
			t.Errorf("ValidatePasswordStrength(%q) should have errored", tt.password)
		}
		if !tt.shouldErr && err != nil {
			t.Errorf("ValidatePasswordStrength(%q) should not have errored: %v", tt.password, err)
		}
	}
}
