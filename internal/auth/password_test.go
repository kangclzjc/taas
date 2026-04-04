package auth

import (
	"testing"
)

func TestValidatePasswordComplexity_Valid(t *testing.T) {
	validPasswords := []string{
		"P@ssw0rd",
		"Str0ng!Pass",
		"Ab1!defgh",
		"MyP@ss123",
		"Test1234!",
		"C0mpl3x$Pass",
	}
	for _, pw := range validPasswords {
		if err := ValidatePasswordComplexity(pw); err != nil {
			t.Errorf("expected password '%s' to be valid, got error: %v", pw, err)
		}
	}
}

func TestValidatePasswordComplexity_TooShort(t *testing.T) {
	err := ValidatePasswordComplexity("Ab1!")
	if err == nil {
		t.Error("expected error for short password")
	}
	if err != nil && !contains(err.Error(), "at least 8 characters") {
		t.Errorf("expected 'at least 8 characters' error, got: %v", err)
	}
}

func TestValidatePasswordComplexity_NoUppercase(t *testing.T) {
	err := ValidatePasswordComplexity("p@ssw0rd1")
	if err == nil {
		t.Error("expected error for password without uppercase")
	}
	if err != nil && !contains(err.Error(), "uppercase letter") {
		t.Errorf("expected 'uppercase letter' error, got: %v", err)
	}
}

func TestValidatePasswordComplexity_NoLowercase(t *testing.T) {
	err := ValidatePasswordComplexity("P@SSW0RD1")
	if err == nil {
		t.Error("expected error for password without lowercase")
	}
	if err != nil && !contains(err.Error(), "lowercase letter") {
		t.Errorf("expected 'lowercase letter' error, got: %v", err)
	}
}

func TestValidatePasswordComplexity_NoDigit(t *testing.T) {
	err := ValidatePasswordComplexity("P@ssword!")
	if err == nil {
		t.Error("expected error for password without digit")
	}
	if err != nil && !contains(err.Error(), "digit") {
		t.Errorf("expected 'digit' error, got: %v", err)
	}
}

func TestValidatePasswordComplexity_NoSpecial(t *testing.T) {
	err := ValidatePasswordComplexity("Passw0rdA")
	if err == nil {
		t.Error("expected error for password without special character")
	}
	if err != nil && !contains(err.Error(), "special character") {
		t.Errorf("expected 'special character' error, got: %v", err)
	}
}

func TestValidatePasswordComplexity_Empty(t *testing.T) {
	err := ValidatePasswordComplexity("")
	if err == nil {
		t.Error("expected error for empty password")
	}
}

func TestValidatePasswordComplexity_MultipleMissing(t *testing.T) {
	// Only lowercase letters, missing uppercase, digit, and special
	err := ValidatePasswordComplexity("abcdefghij")
	if err == nil {
		t.Error("expected error for password missing multiple criteria")
	}
	errMsg := err.Error()
	if !contains(errMsg, "uppercase letter") {
		t.Error("expected 'uppercase letter' in error")
	}
	if !contains(errMsg, "digit") {
		t.Error("expected 'digit' in error")
	}
	if !contains(errMsg, "special character") {
		t.Error("expected 'special character' in error")
	}
}

func TestValidatePasswordComplexity_ExactlyEightChars(t *testing.T) {
	err := ValidatePasswordComplexity("P@ssw0rd")
	if err != nil {
		t.Errorf("expected exactly 8-char password to be valid, got: %v", err)
	}
}

func TestValidatePasswordComplexity_SevenChars(t *testing.T) {
	err := ValidatePasswordComplexity("P@sw0rd")
	if err == nil {
		t.Error("expected error for 7-char password")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
