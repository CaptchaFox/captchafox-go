package captchafox_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CaptchaFox/captchafox-go"
)

// Uses the library from outside its package, as an integrator does: an identifier that stops being exported, or a
// signature that changes, fails to compile here.

const (
	secret  = "ok_11111111000000001111111100000000"
	sitekey = "sk_11111111000000001111111100000000"
)

var (
	_ func(string) (*captchafox.Client, error)                                                           = captchafox.NewClient
	_ func(string, time.Duration) (*captchafox.Client, error)                                            = captchafox.NewClientWithTimeout
	_ func(*captchafox.Client, context.Context, string) (captchafox.Verification, error)                 = (*captchafox.Client).Verify
	_ func(*captchafox.Client, context.Context, string, string, string) (captchafox.Verification, error) = (*captchafox.Client).VerifyWith
	_ func(*captchafox.UnavailableError) string                                                          = (*captchafox.UnavailableError).Error
	_ func(*captchafox.UnavailableError) error                                                           = (*captchafox.UnavailableError).Unwrap
)

// An empty token is rejected without a request, so no network is needed.
func TestEveryExportedIdentifierIsReachable(t *testing.T) {
	client, err := captchafox.NewClient(secret)
	if err != nil {
		t.Fatal(err)
	}
	withTimeout, err := captchafox.NewClientWithTimeout(secret, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}

	for _, verify := range []func() (captchafox.Verification, error){
		func() (captchafox.Verification, error) { return client.Verify(context.Background(), "") },
		func() (captchafox.Verification, error) {
			return withTimeout.VerifyWith(context.Background(), "", sitekey, "203.0.113.7")
		},
	} {
		verification, err := verify()
		if got := describe(verification, err); got != "rejected: missing-input-response" {
			t.Errorf("got %s", got)
		}
	}
}

func describe(verification captchafox.Verification, err error) string {
	var unavailable *captchafox.UnavailableError
	if errors.As(err, &unavailable) {
		return "unavailable: " + unavailable.Error()
	}

	switch v := verification.(type) {
	case *captchafox.Passed:
		return "passed: " + v.ChallengeTS + " " + v.Hostname
	case *captchafox.Rejected:
		return "rejected: " + strings.Join(v.ErrorCodes, ",")
	default:
		return "unknown"
	}
}
