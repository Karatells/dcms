package account

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"testing"
)

func TestAppMFAECDSAVerification(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubB64 := base64.StdEncoding.EncodeToString(pubDER)

	sign := func(p string) string {
		h := sha256.Sum256([]byte(p))
		sig, err := ecdsa.SignASN1(rand.Reader, priv, h[:])
		if err != nil {
			t.Fatal(err)
		}
		return base64.StdEncoding.EncodeToString(sig)
	}

	payload := "dcms-2fa-v1:ch123:approve:47:nonce999"
	sig := sign(payload)
	if !verifyECDSASig(pubB64, payload, sig) {
		t.Fatal("expected valid ECDSA signature to verify")
	}
	if verifyECDSASig(pubB64, "dcms-2fa-v1:ch123:approve:99:nonce999", sig) {
		t.Fatal("expected tampered match_code to fail verification")
	}

	// Другой ключ не должен проходить проверку
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	otherDER, _ := x509.MarshalPKIXPublicKey(&other.PublicKey)
	if verifyECDSASig(base64.StdEncoding.EncodeToString(otherDER), payload, sig) {
		t.Fatal("expected signature from different key to fail")
	}

	// Проверка генерации 3 уникальных чисел с правильным вариантом
	choices := makeNumberChoices("47")
	if len(choices) != 3 {
		t.Fatalf("expected 3 choices, got %d", len(choices))
	}
	found := false
	for _, c := range choices {
		if c == "47" {
			found = true
		}
	}
	if !found {
		t.Fatalf("correct choice 47 missing in %v", choices)
	}
}
