package util

import (
	"encoding/hex"
	"testing"
)

func TestSolveChallenge(t *testing.T) {
	// These values are just examples, we don't have a known good pair yet
	// but we can at least test that it doesn't crash and returns an error for unsolvable challenges
	params := AltchaParameters{
		Nonce:     hex.EncodeToString([]byte("testnonce")),
		Salt:      hex.EncodeToString([]byte("testsalt")),
		Cost:      10,
		KeyLength: 10,
		KeyPrefix: hex.EncodeToString([]byte("nonexistentprefix")),
	}

	// This should fail after MaxSolverIterations
	_, _, err := SolveChallenge(params)
	if err == nil {
		t.Errorf("Expected failure for impossible challenge, but it succeeded")
	}
}

// A more realistic test would involve a known solvable challenge
// If I had access to a real challenge from the API, I could put it here.
