package auth

import (
    "time"

    "github.com/golang-jwt/jwt/v5"
    "github.com/google/uuid"
)

// MakeJWT creates a new JWT token for a user
func MakeJWT(userID uuid.UUID, tokenSecret string, expiresIn time.Duration) (string, error) {
    claims := jwt.RegisteredClaims{
        Issuer:    "chirpy-access",
        IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
        ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiresIn)),
        Subject:   userID.String(),
    }

    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(tokenSecret))
}

// ValidateJWT validates a JWT token and returns the user ID
func ValidateJWT(tokenString, tokenSecret string) (uuid.UUID, error) {
    token, err := jwt.ParseWithClaims(
        tokenString,
        &jwt.RegisteredClaims{},
        func(token *jwt.Token) (interface{}, error) {
            if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
                return nil, jwt.ErrSignatureInvalid
            }
            return []byte(tokenSecret), nil
        },
    )
    if err != nil {
        return uuid.Nil, err
    }

    claims, ok := token.Claims.(*jwt.RegisteredClaims)
    if !ok {
        return uuid.Nil, jwt.ErrInvalidKey
    }

    userID, err := uuid.Parse(claims.Subject)
    if err != nil {
        return uuid.Nil, err
    }

    return userID, nil
}