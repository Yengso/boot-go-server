package auth

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "thissecretpassword"
	
	hash, err := HashPassword(password)
	if err != nil {
		t.Errorf("HashPassword returned an  error: %v", err)
	}
	if hash == "" {
		t.Errorf("HashPassword returned an empty hash")
	}
}

func TestCheckPasswordHash(t *testing.T) {
	password := "thissecretpassword"

	hash, _ := HashPassword(password)

	match, err := CheckPasswordHash(password, hash)
	if err != nil {
		t.Errorf("CheckPasswordHash returned an error: %v", err)
	}
	if !match {
		t.Errorf("Expected password to match hash, but it did not")
	}

	match, err = CheckPasswordHash("wrongpassword", hash)
	if err != nil {
		t.Errorf("CheckPasswordHash returned an error: %v", err)
	}
	if match {
		t.Errorf("Expected passwords not to match, but they did")
	}
}