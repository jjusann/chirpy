package auth

import (
    "errors"
    "net/http"
    "strings"
    "github.com/alexedwards/argon2id"
    "crypto/rand"
    "encoding/hex"
)


// HashPassword hashes a plaintext password using Argon2id
func HashPassword(password string) (string, error) {
    hash, err := argon2id.CreateHash(password, argon2id.DefaultParams)
    if err != nil {
        return "", err
    }
    return hash, nil
}

// CheckPasswordHash compares a plaintext password with a stored hash
func CheckPasswordHash(password, hash string) (bool, error) {
    match, err := argon2id.ComparePasswordAndHash(password, hash)
    if err != nil {
        return false, err
    }
    return match, nil
}

func GetBearerToken(headers http.Header) (string, error) {
    authHeader := headers.Get("Authorization")
    if authHeader == "" {
        return "", errors.New("authorization header is missing")
    }

    // Check if the Authorization header starts with "Bearer "

    parts := strings.Split(authHeader, " ")
    if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
        return "", errors.New("invalid authorization header format")
    }

    token:= strings.TrimSpace(parts[1])
    if token == ""{
        return "", errors.New("bearer token is missing")
    }
    return token, nil
}

func MakeRefreshToken() (string, error) {
    // Generate a random 32-byte token
    tokenBytes := make([]byte, 32)
    _, err := rand.Read(tokenBytes)
    if err != nil {
        return "", err
    }

    // Convert the byte slice to a hexadecimal string
    return hex.EncodeToString(tokenBytes), nil
}