//go:build live

package captchafox

import (
	"context"
	"slices"
	"testing"
)

// Verifies the public test keys against CaptchaFox's siteverify; needs the network.

const (
	livePassingSecret = "ok_11111111000000001111111100000000"
	liveFailingSecret = "ok_FFFFFFFF00000000FFFFFFFF00000000"
	liveSitekey       = "sk_11111111000000001111111100000000"
	liveToken         = "11111111000000001111111100000000"
)

func TestLiveTheTestKeyPasses(t *testing.T) {
	client, err := NewClient(livePassingSecret)
	if err != nil {
		t.Fatal(err)
	}

	if verification, err := client.Verify(context.Background(), liveToken); err != nil {
		t.Fatal(err)
	} else if _, ok := verification.(*Passed); !ok {
		t.Errorf("got %+v; want *Passed", verification)
	}
}

func TestLiveTheTestKeyPassesWithSitekeyAndRemoteIP(t *testing.T) {
	client, err := NewClient(livePassingSecret)
	if err != nil {
		t.Fatal(err)
	}

	if verification, err := client.VerifyWith(context.Background(), liveToken, liveSitekey, "203.0.113.7"); err != nil {
		t.Fatal(err)
	} else if _, ok := verification.(*Passed); !ok {
		t.Errorf("got %+v; want *Passed", verification)
	}
}

func TestLiveTheFailingTestKeyIsRejected(t *testing.T) {
	client, err := NewClient(liveFailingSecret)
	if err != nil {
		t.Fatal(err)
	}

	verification, err := client.Verify(context.Background(), liveToken)
	if err != nil {
		t.Fatal(err)
	}
	rejected, ok := verification.(*Rejected)
	if !ok || !slices.Equal(rejected.ErrorCodes, []string{"timeout-or-duplicate"}) {
		t.Errorf("got %+v; want *Rejected with timeout-or-duplicate", verification)
	}
}
