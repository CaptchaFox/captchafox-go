package captchafox

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"testing"
	"time"
)

// What the specification's cases leave out.

const testSecret = "ok_11111111000000001111111100000000"

func TestFieldsArriveUnchangedWhateverTheirCharacters(t *testing.T) {
	server := answering(t, 200, `{"success":true}`)
	secret := "ok_a+b&c=d %ä"
	token := "t+&=/?#% ä€"
	client, err := newClient(secret, 2*time.Second, server.endpoint())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.VerifyWith(context.Background(), token, "sk_1", "2001:db8::1"); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{"secret": secret, "response": token, "sitekey": "sk_1", "remoteIp": "2001:db8::1"}
	got := map[string]string{}
	for name, values := range server.requests()[0].form {
		got[name] = strings.Join(values, ",")
	}
	if !maps.Equal(got, want) {
		t.Errorf("the form is %q; want %q", got, want)
	}
}

func TestAnswersWithoutAOneBooleanSuccessCannotBeEvaluated(t *testing.T) {
	for _, body := range []string{
		`{"success":false,"success":true}`,
		`{"Success":true}`,
		`{"success":null}`,
		`{"success":true} {}`,
		`{"success":true,"x":` + strings.Repeat("[", 100_000) + strings.Repeat("]", 100_000) + `}`,
	} {
		verification, err := evaluate([]byte(body))

		var unavailable *UnavailableError
		if verification != nil || !errors.As(err, &unavailable) {
			t.Errorf("%.40s: got %#v, %v; want an *UnavailableError", body, verification, err)
		}
	}
}

func TestTheDefaultTimeoutIsFiveSeconds(t *testing.T) {
	client, err := NewClient(testSecret)
	if err != nil {
		t.Fatal(err)
	}

	if client.timeout != 5*time.Second {
		t.Errorf("the timeout is %s", client.timeout)
	}
}

func TestTheTimeoutEndsAnAnswerThatNeverCompletesAndClosesTheConnection(t *testing.T) {
	server := trickling(t, 200, `{"success":true`)
	client, err := newClient(testSecret, time.Second, server.endpoint())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()

	// Without a deadline of the context, the client's timeout alone ends the verification.
	_, err = bounded(t, func() (Verification, error) {
		return client.Verify(context.Background(), "token")
	})

	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("got %v; want an *UnavailableError", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("the verification took %s", elapsed)
	}

	limit := time.Now().Add(5 * time.Second)
	for server.brokenAtTime().IsZero() && time.Now().Before(limit) {
		time.Sleep(50 * time.Millisecond)
	}
	if server.brokenAtTime().IsZero() {
		t.Error("the connection is still open")
	}
}

func TestTheTimeoutHoldsWithinALongerDeadlineOfTheContext(t *testing.T) {
	server := stalling(t, 200, `{"success":`)
	client, err := newClient(testSecret, time.Second, server.endpoint())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	start := time.Now()

	_, err = bounded(t, func() (Verification, error) {
		return client.Verify(ctx, "token")
	})

	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("got %v; want an *UnavailableError", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("the verification took %s", elapsed)
	}
}

func TestACancelledContextEndsTheVerification(t *testing.T) {
	server := silent(t)
	client, err := newClient(testSecret, caseBound, server.endpoint())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)

	_, err = bounded(t, func() (Verification, error) {
		return client.Verify(ctx, "token")
	})

	var unavailable *UnavailableError
	if !errors.As(err, &unavailable) || !errors.Is(err, context.Canceled) {
		t.Errorf("got %v; want an *UnavailableError of a cancelled context", err)
	}
}

func TestARedirectIsNotFollowed(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		target := answering(t, 200, `{"success":true}`)
		server := redirecting(t, status, target.endpoint())
		client, err := newClient(testSecret, 2*time.Second, server.endpoint())
		if err != nil {
			t.Fatal(err)
		}

		verification, err := client.Verify(context.Background(), "token")

		var unavailable *UnavailableError
		if verification != nil || !errors.As(err, &unavailable) {
			t.Errorf("%d: got %#v, %v; want an *UnavailableError", status, verification, err)
		}
		if len(target.requests()) != 0 {
			t.Errorf("%d: the redirect was followed", status)
		}
	}
}

func TestTheSecretKeyIsInNoFormOfTheClient(t *testing.T) {
	client, err := NewClient(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	type holder struct {
		byValue   Client
		byPointer *Client
	}

	for _, value := range []any{client, *client, holder{*client, client}, &holder{*client, client}} {
		for _, form := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%d"} {
			if text := fmt.Sprintf(form, value); strings.Contains(text, testSecret) {
				t.Errorf("the secret key is in %s", text)
			}
		}
	}
}

func TestAnEmptySecretKeyOrNoPositiveTimeoutIsRefused(t *testing.T) {
	if _, err := NewClient(""); err == nil {
		t.Error("an empty secret key is accepted")
	}
	if _, err := NewClientWithTimeout(testSecret, 0); err == nil {
		t.Error("a timeout of 0 is accepted")
	}
	if _, err := NewClientWithTimeout(testSecret, -time.Second); err == nil {
		t.Error("a negative timeout is accepted")
	}
}
