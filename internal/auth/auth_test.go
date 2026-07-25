package auth

import (
	"errors"
	"net/http"
	"testing"
)

func TestGetBearerToken(t *testing.T) {
	tests := []struct {
		name          string
		headers       http.Header
		wantToken     string
		wantError     bool
	}{
		{
			name: "Valid Bearer Token",
			headers: http.Header{
				"Authorization": []string{"Bearer validtoken123"},
			},
			wantToken: "validtoken123",
			wantError: false,
		},
		{
			name: "Missing Authorization Header",
			headers: http.Header{},
			wantToken: "",
			wantError: true,
		},
		{
			name: "Invalid Authorization Header Format",
			headers: http.Header{
				"Authorization": []string{"InvalidFormat"},
			},
			wantToken: "",
			wantError: true,
		},
		{
			name: "Empty Bearer Token",
			headers: http.Header{
				"Authorization": []string{"Bearer "},
			},
			wantToken:	"",
			wantError: true,
		},
		{
			name: "Bearer Token with Lowercase 'bearer'",
			headers: http.Header{
				"Authorization": []string{"bearer validtoken123"},
			},
			wantToken: "",
			wantError: true,
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := GetBearerToken(tt.headers)
			if (err != nil) != tt.wantError {
				t.Errorf("GetBearerToken() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if token != tt.wantToken {
				t.Errorf("GetBearerToken() token = %v, want %v", token, tt.wantToken)
			}
		})
	}
}