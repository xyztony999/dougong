package panel

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"
	"time"
)

func TestIssuerDisplayName(t *testing.T) {
	if got := issuerDisplayName(nil); got != "" {
		t.Fatalf("nil cert: %q", got)
	}
	cn := issuerDisplayName(&x509.Certificate{Issuer: pkix.Name{CommonName: "R11", Organization: []string{"Let's Encrypt"}}})
	if cn != "R11" {
		t.Fatalf("CN preferred = %q", cn)
	}
	org := issuerDisplayName(&x509.Certificate{Issuer: pkix.Name{Organization: []string{"Let's Encrypt", "ISRG"}}})
	if org != "Let's Encrypt, ISRG" {
		t.Fatalf("org fallback = %q", org)
	}
	empty := issuerDisplayName(&x509.Certificate{Issuer: pkix.Name{}})
	if empty != "" {
		t.Fatalf("empty = %q", empty)
	}
}

func TestLiveIssuerFallsBackToPEM(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	certPEM, _, fields, err := IssueSelfSigned(defaultTestRand{}, now)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	stored := ""
	got := liveIssuer(&stored, certPEM)
	if got == nil || *got != fields.Issuer {
		t.Fatalf("liveIssuer from PEM = %v, want %q", got, fields.Issuer)
	}
	named := "Already Set"
	got = liveIssuer(&named, certPEM)
	if got == nil || *got != "Already Set" {
		t.Fatalf("non-empty stored must win: %v", got)
	}
}
