package turn

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"
)

// newTestAPI assembles a humatest API with the TURN handler registered, mirroring
// how app wires the handler into the router. It returns a TestAPI whose Get drives
// the real huma typed handler end-to-end (request -> handler -> SuccessResponse envelope).
func newTestAPI(t *testing.T) humatest.TestAPI {
	t.Helper()
	_, api := humatest.New(t)
	NewHandler().Register(api)
	return api
}

// envelope mirrors the observable `{"data": Credentials}` JSON the SuccessResponse[Credentials]
// envelope produces. The test only depends on the wire contract, not on internal types.
type envelope struct {
	Data Credentials `json:"data"`
}

// getCredentials calls GET /turn-credentials and decodes the envelope, asserting 200.
func getCredentials(t *testing.T, api humatest.TestAPI) (Credentials, string) {
	t.Helper()
	resp := api.Get("/turn-credentials")
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /turn-credentials status = %d, want %d; body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
	raw := resp.Body.String()
	var env envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, raw)
	}
	return env.Data, raw
}

// TestGetCredentials_BasicIssuance covers AC-1: every field is populated on a normal request.
func TestGetCredentials_BasicIssuance(t *testing.T) {
	api := newTestAPI(t)

	creds, _ := getCredentials(t, api)

	if creds.Username == "" {
		t.Error("AC-1: username is empty")
	}
	if creds.Password == "" {
		t.Error("AC-1: password is empty")
	}
	if creds.TTL == 0 {
		t.Error("AC-1: ttl is zero")
	}
	if len(creds.URIs) == 0 {
		t.Error("AC-1: uris is empty")
	}
}

// TestGetCredentials_TTL covers AC-3 / FR-4: the credential carries an explicit 3600s TTL.
func TestGetCredentials_TTL(t *testing.T) {
	api := newTestAPI(t)

	creds, _ := getCredentials(t, api)

	if creds.TTL != 3600 {
		t.Errorf("AC-3: ttl = %d, want 3600", creds.TTL)
	}
}

// TestGetCredentials_URIs covers FR-3 / AC-2: at least one URI, default fallback, and
// comma-split with whitespace trimming. Cases mutate TURN_URIS via t.Setenv so they run serially.
func TestGetCredentials_URIs(t *testing.T) {
	t.Run("unset TURN_URIS yields at least one default URI", func(t *testing.T) {
		t.Setenv("TURN_URIS", "")

		api := newTestAPI(t)
		creds, _ := getCredentials(t, api)

		if len(creds.URIs) < 1 {
			t.Fatalf("AC-2: uris length = %d, want >= 1", len(creds.URIs))
		}
		for _, uri := range creds.URIs {
			if strings.TrimSpace(uri) == "" {
				t.Errorf("AC-2: uris contains empty entry: %#v", creds.URIs)
			}
		}
	})

	t.Run("comma separated TURN_URIS is split and trimmed", func(t *testing.T) {
		t.Setenv("TURN_URIS", "a, b ,c")

		api := newTestAPI(t)
		creds, _ := getCredentials(t, api)

		want := []string{"a", "b", "c"}
		if len(creds.URIs) != len(want) {
			t.Fatalf("uris = %#v, want %#v", creds.URIs, want)
		}
		for i := range want {
			if creds.URIs[i] != want[i] {
				t.Errorf("uris[%d] = %q, want %q (full: %#v)", i, creds.URIs[i], want[i], creds.URIs)
			}
		}
	})
}

// TestGetCredentials_UsernameFormat covers the username contract: `^\d+:cobrowsing$`
// where the numeric part is the expiry (now unix + 3600), within a small clock-skew window.
func TestGetCredentials_UsernameFormat(t *testing.T) {
	api := newTestAPI(t)

	before := time.Now().Unix()
	creds, _ := getCredentials(t, api)
	after := time.Now().Unix()

	re := regexp.MustCompile(`^\d+:cobrowsing$`)
	if !re.MatchString(creds.Username) {
		t.Fatalf("username = %q, want match %q", creds.Username, re.String())
	}

	expiryStr := strings.TrimSuffix(creds.Username, ":cobrowsing")
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		t.Fatalf("parse expiry from username %q: %v", creds.Username, err)
	}

	const ttl = 3600
	if expiry < before+ttl || expiry > after+ttl {
		t.Errorf("expiry = %d, want within [%d, %d] (now+%d)", expiry, before+ttl, after+ttl, ttl)
	}
}

// TestGetCredentials_HMAC covers HMAC-SHA1 correctness: with a known secret, the response
// password must equal base64(HMAC_SHA1(secret, username)) recomputed independently in the test.
// Mutates TURN_SECRET via t.Setenv so it must run serially.
func TestGetCredentials_HMAC(t *testing.T) {
	const secret = "key"
	t.Setenv("TURN_SECRET", secret)

	api := newTestAPI(t)
	creds, _ := getCredentials(t, api)

	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(creds.Username))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	if creds.Password != want {
		t.Errorf("password = %q, want %q (recomputed HMAC-SHA1 over %q)", creds.Password, want, creds.Username)
	}
}

// TestGetCredentials_SuccessiveCallsMayDiffer covers AC-5: two requests separated by >= 1s
// may receive different usernames/expiries. Kept lenient — within the same second they may match.
func TestGetCredentials_SuccessiveCallsMayDiffer(t *testing.T) {
	api := newTestAPI(t)

	first, _ := getCredentials(t, api)
	time.Sleep(1100 * time.Millisecond)
	second, _ := getCredentials(t, api)

	if first.Username == second.Username {
		t.Errorf("AC-5: usernames did not advance across a >1s gap: both = %q", first.Username)
	}
}

// TestGetCredentials_NoAuth covers FR-7 / AC-6: a request with no auth header or body still gets 200.
func TestGetCredentials_NoAuth(t *testing.T) {
	api := newTestAPI(t)

	resp := api.Get("/turn-credentials")
	if resp.Code != http.StatusOK {
		t.Errorf("AC-6: unauthenticated request status = %d, want %d", resp.Code, http.StatusOK)
	}
}

// TestGetCredentials_SecretNotExposed covers NFR-2: the configured TURN_SECRET must never
// appear anywhere in the response JSON (password is an HMAC, not the secret itself).
func TestGetCredentials_SecretNotExposed(t *testing.T) {
	const secret = "super-secret-signing-key"
	t.Setenv("TURN_SECRET", secret)

	api := newTestAPI(t)
	_, raw := getCredentials(t, api)

	if strings.Contains(raw, secret) {
		t.Errorf("NFR-2: response leaks TURN_SECRET %q in body: %s", secret, raw)
	}
	// Also guard the default secret value never leaking when it would be the active one.
	if strings.Contains(raw, "changeme") {
		t.Errorf("NFR-2: response contains default secret literal %q: %s", "changeme", raw)
	}
}
