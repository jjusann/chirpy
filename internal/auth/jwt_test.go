package auth

import (
    "testing"
    "time"

    "github.com/google/uuid"
)

func TestMakeJWT(t *testing.T) {
    userID := uuid.New()
    secret := "test-secret"
    expiresIn := 1 * time.Hour

    token, err := MakeJWT(userID, secret, expiresIn)
    if err != nil {
        t.Fatalf("MakeJWT failed: %v", err)
    }

    if token == "" {
        t.Fatal("Expected token, got empty string")
    }
}

func TestValidateJWT(t *testing.T) {
    userID := uuid.New()
    secret := "test-secret"
    expiresIn := 1 * time.Hour

    token, err := MakeJWT(userID, secret, expiresIn)
    if err != nil {
        t.Fatalf("MakeJWT failed: %v", err)
    }

    parsedID, err := ValidateJWT(token, secret)
    if err != nil {
        t.Fatalf("ValidateJWT failed: %v", err)
    }

    if parsedID != userID {
        t.Errorf("Expected user ID %v, got %v", userID, parsedID)
    }
}

func TestValidateJWT_WrongSecret(t *testing.T) {
    userID := uuid.New()
    secret := "test-secret"
    expiresIn := 1 * time.Hour

    token, err := MakeJWT(userID, secret, expiresIn)
    if err != nil {
        t.Fatalf("MakeJWT failed: %v", err)
    }

    _, err = ValidateJWT(token, "wrong-secret")
    if err == nil {
        t.Fatal("Expected error for wrong secret, got nil")
    }
}

func TestValidateJWT_Expired(t *testing.T) {
    userID := uuid.New()
    secret := "test-secret"
    expiresIn := -1 * time.Hour // expired

    token, err := MakeJWT(userID, secret, expiresIn)
    if err != nil {
        t.Fatalf("MakeJWT failed: %v", err)
    }

    _, err = ValidateJWT(token, secret)
    if err == nil {
        t.Fatal("Expected error for expired token, got nil")
    }
}