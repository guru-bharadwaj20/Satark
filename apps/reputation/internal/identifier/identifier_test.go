package identifier

import "testing"

// Fixtures use obviously synthetic numbers and VPAs (see CONTRIBUTING.md).
func TestNormalize(t *testing.T) {
	cases := []struct {
		k        Kind
		in, want string
		err      bool
	}{
		{Phone, "+91 98XXXXXX21", "", true},
		{Phone, "+91 90000 00001", "9000000001", false},
		{Phone, "09000000001", "9000000001", false},
		{Phone, "12345", "", true},
		{UPI, " XY.Test@ExampleBank ", "xy.test@examplebank", false},
		{UPI, "not-a-vpa", "", true},
		{Domain, "https://WWW.Example-Bank.in/login?x=1", "example-bank.in", false},
		{Domain, "kyc-update.example/path", "kyc-update.example", false},
		{Domain, "localhost", "", true},
	}
	for _, c := range cases {
		got, err := Normalize(c.k, c.in)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("Normalize(%s,%q) = %q,%v; want %q err=%v", c.k, c.in, got, err, c.want, c.err)
		}
	}
}

func TestHasherStable(t *testing.T) {
	h := NewHasher("pepper")
	if h.Subject(Phone, "9000000001") != h.Subject(Phone, "9000000001") {
		t.Fatal("hash not stable")
	}
	if h.Subject(Phone, "9000000001") == NewHasher("other").Subject(Phone, "9000000001") {
		t.Fatal("pepper ignored")
	}
}
