package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if hash == "" {
		t.Fatalf("expected non-empty hash")
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Fatalf("expected password to verify")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Fatalf("wrong password verified")
	}
}

func TestVerifyPasswordRejectsMalformedHashes(t *testing.T) {
	t.Parallel()

	cases := []string{
		"",
		"not-a-hash",
		"$bcrypt$v=19$m=65536,t=3,p=2$salt$hash",
		"$argon2id$v=19$m=bad,t=3,p=2$salt$hash",
		"$argon2id$v=19$m=65536,t=3,p=2$not-base64$also-not-base64",
	}

	for _, candidate := range cases {
		if VerifyPassword(candidate, "password") {
			t.Fatalf("malformed hash verified: %q", candidate)
		}
	}
}
