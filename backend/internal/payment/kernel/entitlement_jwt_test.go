package kernel

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestEntitlementJWT_SignAndParse_OK(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	uid := uuid.New()
	scope := "post:" + uuid.NewString() + ":unlock"
	tok, err := SignEntitlement(secret, "paypal", uid, scope, 2*time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := ParseEntitlement(secret, tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.Subject != uid.String() {
		t.Fatalf("sub mismatch: %q", claims.Subject)
	}
	if claims.Provider != "paypal" {
		t.Fatalf("provider mismatch: %q", claims.Provider)
	}
	if claims.Scope != scope {
		t.Fatalf("scope mismatch: %q", claims.Scope)
	}
}

func TestEntitlementJWT_Parse_BadSecret(t *testing.T) {
	uid := uuid.New()
	scope := "post:" + uuid.NewString() + ":unlock"
	tok, err := SignEntitlement([]byte("0123456789abcdef0123456789abcdef"), "paypal", uid, scope, time.Minute)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if _, err := ParseEntitlement([]byte("different-secret-0123456789"), tok); err == nil {
		t.Fatalf("expected parse failure")
	}
}

