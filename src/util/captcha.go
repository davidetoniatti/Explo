package util

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

const MaxSolverIterations = 1000000
const CookieValidity = 28 * time.Minute

type CaptchaSolver struct {
	HttpClient    *HttpClient
	CookieHeader  string
	CookieExpires time.Time
	Mu            sync.Mutex
}

type AltchaChallengeResponse struct {
	Parameters AltchaParameters `json:"parameters"`
}

type AltchaParameters struct {
	Nonce     string `json:"nonce"`
	Salt      string `json:"salt"`
	Cost      int    `json:"cost"`
	KeyLength int    `json:"keyLength"`
	KeyPrefix string `json:"keyPrefix"`
}

type AltchaSolution struct {
	Counter    int    `json:"counter"`
	DerivedKey string `json:"derivedKey"`
	Time       int64  `json:"time"`
}

func NewCaptchaSolver(httpClient *HttpClient) *CaptchaSolver {
	return &CaptchaSolver{
		HttpClient: httpClient,
	}
}

func (s *CaptchaSolver) GetCaptchaCookie(baseUrl string) (string, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	if s.CookieHeader != "" && time.Now().Before(s.CookieExpires) {
		return s.CookieHeader, nil
	}

	cookie, err := s.SolveAndVerify(baseUrl)
	if err != nil {
		return "", err
	}

	s.CookieHeader = cookie
	s.CookieExpires = time.Now().Add(CookieValidity)
	return s.CookieHeader, nil
}

// Refresh forces a fresh captcha solve (e.g. after the server rejected the cached
// cookie), stores the resulting cookie+expiry under the mutex, and returns it.
func (s *CaptchaSolver) Refresh(baseUrl string) (string, error) {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	cookie, err := s.SolveAndVerify(baseUrl)
	if err != nil {
		return "", err
	}

	s.CookieHeader = cookie
	s.CookieExpires = time.Now().Add(CookieValidity)
	return s.CookieHeader, nil
}

func (s *CaptchaSolver) SolveAndVerify(baseUrl string) (string, error) {
	trimmed := strings.TrimRight(baseUrl, "/")

	challengeBody, err := s.HttpClient.MakeRequest("GET", trimmed+"/api/altcha/challenge", nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get challenge: %w", err)
	}

	var challengeResp AltchaChallengeResponse
	if err := json.Unmarshal(challengeBody, &challengeResp); err != nil {
		return "", fmt.Errorf("failed to unmarshal challenge: %w", err)
	}

	start := time.Now()
	counter, derivedKey, err := SolveChallenge(challengeResp.Parameters)
	if err != nil {
		return "", err
	}
	elapsedMs := time.Since(start).Milliseconds()

	solution := AltchaSolution{
		Counter:    counter,
		DerivedKey: derivedKey,
		Time:       elapsedMs,
	}

	// Create payload: {"challenge": <original_challenge_json>, "solution": <solution_json>}
	// The C# code does some manual string manipulation to combine them
	var challengeRaw map[string]interface{}
	json.Unmarshal(challengeBody, &challengeRaw)

	payload := map[string]interface{}{
		"challenge": challengeRaw,
		"solution":  solution,
	}

	payloadBytes, _ := json.Marshal(payload)
	payloadB64 := base64.StdEncoding.EncodeToString(payloadBytes)

	verifyBody := map[string]string{
		"payload": payloadB64,
	}
	verifyBytes, _ := json.Marshal(verifyBody)

	// We need to use http.Client directly to get headers (Set-Cookie)
	req, err := http.NewRequest("POST", trimmed+"/api/altcha/verify", bytes.NewReader(verifyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("User-Agent", s.HttpClient.UserAgent)

	resp, err := s.HttpClient.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("verify failed with status %d", resp.StatusCode)
	}

	var captchaCookie string
	for _, cookie := range resp.Cookies() {
		if strings.HasPrefix(cookie.Name, "captcha_verified_at") {
			captchaCookie = fmt.Sprintf("%s=%s", cookie.Name, cookie.Value)
			break
		}
	}

	if captchaCookie == "" {
		// Try manual parsing of Set-Cookie header if Cookies() didn't find it
		for _, h := range resp.Header["Set-Cookie"] {
			if strings.HasPrefix(h, "captcha_verified_at=") {
				captchaCookie = strings.Split(h, ";")[0]
				break
			}
		}
	}

	if captchaCookie == "" {
		return "", fmt.Errorf("captcha verify response did not set captcha_verified_at cookie")
	}

	slog.Info("SquidWTF Qobuz captcha solved", "elapsedMs", elapsedMs, "counter", counter)

	return captchaCookie, nil
}

func SolveChallenge(p AltchaParameters) (int, string, error) {
	nonce, _ := hex.DecodeString(p.Nonce)
	salt, _ := hex.DecodeString(p.Salt)
	keyPrefix, _ := hex.DecodeString(p.KeyPrefix)

	password := make([]byte, len(nonce)+4)
	copy(password, nonce)

	initial := make([]byte, len(salt)+len(password))
	copy(initial, salt)

	derived := make([]byte, p.KeyLength)

	for counter := 0; counter < MaxSolverIterations; counter++ {
		binary.BigEndian.PutUint32(password[len(nonce):], uint32(counter))
		copy(initial[len(salt):], password)

		hash := sha256.Sum256(initial)
		copy(derived, hash[:p.KeyLength])

		for i := 1; i < p.Cost; i++ {
			hash = sha256.Sum256(derived)
			copy(derived, hash[:p.KeyLength])
		}

		if bytes.HasPrefix(derived, keyPrefix) {
			return counter, hex.EncodeToString(derived), nil
		}
	}

	return 0, "", fmt.Errorf("captcha solver exhausted %d iterations without finding a match", MaxSolverIterations)
}
