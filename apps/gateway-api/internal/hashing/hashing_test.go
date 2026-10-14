package hashing

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

var testKey = bytes.Repeat([]byte{0x42}, 32)

func mustHasher(t *testing.T) *Hasher {
	t.Helper()
	h, err := New(testKey)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestNewRejectsShortKey(t *testing.T) {
	if _, err := New([]byte("short")); !errors.Is(err, ErrShortKey) {
		t.Fatalf("want ErrShortKey, got %v", err)
	}
}

func TestNewFromEnv(t *testing.T) {
	t.Setenv(EnvKey, base64.StdEncoding.EncodeToString(testKey))
	h, err := NewFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(h.Sum(KindText, "x"), mustHasher(t).Sum(KindText, "x")) {
		t.Fatal("env key differs from literal key")
	}
	t.Setenv(EnvKey, "")
	if _, err := NewFromEnv(); err == nil {
		t.Fatal("want error for empty key")
	}
}

func TestSumIsKeyedAndDomainSeparated(t *testing.T) {
	h := mustHasher(t)
	other, _ := New(bytes.Repeat([]byte{0x43}, 32))
	a := h.Sum(KindPhone, "9876543210")
	if len(a) != Size {
		t.Fatalf("digest size %d", len(a))
	}
	if Equal(a, other.Sum(KindPhone, "9876543210")) {
		t.Fatal("different keys must give different digests")
	}
	if Equal(a, h.Sum(KindText, "9876543210")) {
		t.Fatal("different kinds must give different digests")
	}
}

func TestEntityPhoneNormalization(t *testing.T) {
	h := mustHasher(t)
	want, masked, err := h.Entity(KindPhone, "9876543210")
	if err != nil {
		t.Fatal(err)
	}
	if masked != "98XXXXXX10" {
		t.Fatalf("masked = %q", masked)
	}
	for _, in := range []string{"+91 98765 43210", "09876543210", "91-9876-543-210"} {
		got, _, err := h.Entity(KindPhone, in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if !Equal(got, want) {
			t.Errorf("%q did not normalize to the same digest", in)
		}
	}
	if _, _, err := h.Entity(KindPhone, "12"); !errors.Is(err, ErrInvalidValue) {
		t.Fatalf("want ErrInvalidValue, got %v", err)
	}
}

func TestEntityUPI(t *testing.T) {
	h := mustHasher(t)
	a, masked, err := h.Entity(KindUPI, " XYZ123@ExampleBank ")
	if err != nil {
		t.Fatal(err)
	}
	b, _, _ := h.Entity(KindUPI, "xyz123@examplebank")
	if !Equal(a, b) {
		t.Fatal("UPI IDs must be case-insensitive")
	}
	if masked != "xy***@examplebank" {
		t.Fatalf("masked = %q", masked)
	}
	for _, bad := range []string{"noat", "@psp", "a@b@c", "a@", "a b@psp"} {
		if _, _, err := h.Entity(KindUPI, bad); err == nil {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestEntityDomain(t *testing.T) {
	h := mustHasher(t)
	a, masked, err := h.Entity(KindDomain, "https://WWW.Example-Bank.in./login")
	if err != nil {
		t.Fatal(err)
	}
	if masked != "example-bank.in" {
		t.Fatalf("masked = %q", masked)
	}
	b, _, _ := h.Entity(KindDomain, "example-bank.in")
	if !Equal(a, b) {
		t.Fatal("domain normalization mismatch")
	}
	if _, _, err := h.Entity(KindDomain, "localhost"); err == nil {
		t.Fatal("single-label host should be invalid")
	}
}

func TestInputText(t *testing.T) {
	h := mustHasher(t)
	a, _ := h.Input(KindText, "Your  KYC is\nEXPIRED")
	b, _ := h.Input(KindText, "your kyc is expired")
	if !Equal(a, b) {
		t.Fatal("whitespace/case variants must share a digest")
	}
	if _, err := h.Input(KindPhone, "x"); err == nil {
		t.Fatal("phone is not an input kind")
	}
}

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"HTTPS://Bit.LY:443/abc#frag":  "https://bit.ly/abc",
		"example.in":                   "https://example.in/",
		"http://example.in:8080/x?q=1": "http://example.in:8080/x?q=1",
	}
	for in, want := range cases {
		got, err := NormalizeURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestMasking(t *testing.T) {
	if got := MaskPhone("123"); got != "XXX" {
		t.Fatalf("got %q", got)
	}
	if got := MaskUPI("a@psp"); got != "a***@psp" {
		t.Fatalf("got %q", got)
	}
}
