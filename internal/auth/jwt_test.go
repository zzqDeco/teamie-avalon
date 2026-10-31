package auth_test

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/zzqDeco/teamie-avalon/internal/auth"
)

func TestVerifier_StrictHS256Validation(t *testing.T) {
	const (
		appID  = "11111111-2222-4333-8444-555555555555"
		secret = "tmsec_test_avalon_signing_secret_0123456789abcdef"
	)
	now := time.Date(2026, 10, 31, 15, 0, 0, 0, time.UTC)
	v, err := auth.NewVerifier(appID, secret)
	if err != nil {
		t.Fatalf("NewVerifier failed: %v", err)
	}
	v.WithNow(func() time.Time { return now })

	validTok, err := auth.SignToken(secret, auth.Claims{
		AppID:          appID,
		Slug:           "avalon",
		WorkspaceID:    "ws-001",
		ConversationID: "conv-001",
		UserID:         "user-sub-001",
		DisplayName:    "Arthur",
		IssuedAt:       now.Unix(),
		ExpiresAt:      now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("SignToken failed: %v", err)
	}

	claims, err := v.VerifyAuthorizationHeader("Bearer " + validTok)
	if err != nil {
		t.Fatalf("expected valid token, got error: %v", err)
	}
	if claims.UserID != "user-sub-001" || claims.WorkspaceID != "ws-001" || claims.ConversationID != "conv-001" {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	t.Run("missing bearer header", func(t *testing.T) {
		if _, err := v.VerifyAuthorizationHeader(""); err == nil {
			t.Fatal("expected error for empty header")
		}
		if _, err := v.VerifyAuthorizationHeader("Basic abc"); err == nil {
			t.Fatal("expected error for non-Bearer scheme")
		}
	})

	t.Run("forged signature rejected", func(t *testing.T) {
		forged, _ := auth.SignToken("tmsec_wrong_secret", auth.Claims{
			AppID:          appID,
			Slug:           "avalon",
			WorkspaceID:    "ws-001",
			ConversationID: "conv-001",
			UserID:         "user-sub-001",
			IssuedAt:       now.Unix(),
			ExpiresAt:      now.Add(time.Hour).Unix(),
		})
		if _, err := v.VerifyAuthorizationHeader("Bearer " + forged); err == nil {
			t.Fatal("expected forged signature to be rejected")
		}
	})

	t.Run("alg none rejected", func(t *testing.T) {
		hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
		pay := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"teamie","aud":"` + appID + `","sub":"u1","workspaceId":"w1","conversationId":"c1","exp":9999999999}`))
		if _, err := v.VerifyToken(hdr + "." + pay + "."); err == nil {
			t.Fatal("expected alg=none to be rejected")
		}
	})

	t.Run("HS384 rejected", func(t *testing.T) {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.MapClaims{
			"iss":            "teamie",
			"aud":            appID,
			"sub":            "u1",
			"workspaceId":    "w1",
			"conversationId": "c1",
			"exp":            now.Add(time.Hour).Unix(),
		})
		signed, _ := tok.SignedString([]byte(secret))
		if _, err := v.VerifyToken(signed); err == nil {
			t.Fatal("expected HS384 to be rejected when only HS256 is permitted")
		}
	})

	t.Run("wrong issuer or audience rejected", func(t *testing.T) {
		badIss := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"iss":            "other",
			"aud":            appID,
			"sub":            "u1",
			"workspaceId":    "w1",
			"conversationId": "c1",
			"exp":            now.Add(time.Hour).Unix(),
		})
		s1, _ := badIss.SignedString([]byte(secret))
		if _, err := v.VerifyToken(s1); err == nil {
			t.Fatal("expected wrong issuer to fail")
		}

		badAud := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"iss":            "teamie",
			"aud":            "other-app-id",
			"sub":            "u1",
			"workspaceId":    "w1",
			"conversationId": "c1",
			"exp":            now.Add(time.Hour).Unix(),
		})
		s2, _ := badAud.SignedString([]byte(secret))
		if _, err := v.VerifyToken(s2); err == nil {
			t.Fatal("expected wrong audience to fail")
		}
	})

	t.Run("expired token rejected", func(t *testing.T) {
		expTok, _ := auth.SignToken(secret, auth.Claims{
			AppID:          appID,
			Slug:           "avalon",
			WorkspaceID:    "ws-001",
			ConversationID: "conv-001",
			UserID:         "user-sub-001",
			IssuedAt:       now.Add(-2 * time.Hour).Unix(),
			ExpiresAt:      now.Add(-1 * time.Second).Unix(),
		})
		if _, err := v.VerifyToken(expTok); err == nil {
			t.Fatal("expected expired token to be rejected")
		}
	})

	t.Run("sub takes strict precedence over mismatched userId claim", func(t *testing.T) {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"iss":            "teamie",
			"aud":            appID,
			"sub":            "canonical-sub",
			"userId":         "spoofed-body-user",
			"workspaceId":    "ws-001",
			"conversationId": "conv-001",
			"exp":            now.Add(time.Hour).Unix(),
		})
		signed, _ := tok.SignedString([]byte(secret))
		got, err := v.VerifyToken(signed)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.UserID != "canonical-sub" {
			t.Fatalf("expected sub canonical-sub, got %s", got.UserID)
		}
	})
}
