package captchafox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	caseTimeout = 5 * time.Second
	// Short, so that the cases of a siteverify that stops answering end quickly.
	silenceTimeout = 1 * time.Second
	caseBound      = 10 * time.Second
)

type specification struct {
	Endpoint string `json:"endpoint"`
	Cases    []struct {
		Name    string            `json:"name"`
		Input   map[string]string `json:"input"`
		Request map[string]string `json:"request"`
		Answer  *struct {
			Error  string `json:"error"`
			Status int    `json:"status"`
			Body   string `json:"body"`
		} `json:"answer"`
		Expect struct {
			Outcome     string   `json:"outcome"`
			ChallengeTs string   `json:"challengeTs"`
			Hostname    string   `json:"hostname"`
			ErrorCodes  []string `json:"errorCodes"`
		} `json:"expect"`
	} `json:"cases"`
}

func readSpecification(t *testing.T) specification {
	t.Helper()

	data, err := os.ReadFile("__fixtures__/siteverify-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec specification
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestTheEndpointIsTheOneOfTheSpecification(t *testing.T) {
	if spec := readSpecification(t); spec.Endpoint != endpoint {
		t.Errorf("the endpoint is %s, the specification has %s", endpoint, spec.Endpoint)
	}
}

func TestEverySiteverifyCase(t *testing.T) {
	spec := readSpecification(t)
	if len(spec.Cases) == 0 {
		t.Fatal("the specification has no cases")
	}

	for _, c := range spec.Cases {
		t.Run(c.Name, func(t *testing.T) {
			secret := c.Input["secret"]

			var server *localSiteverify
			var target string
			timeout := caseTimeout
			switch {
			case c.Answer == nil:
				server = answering(t, 500, "")
				target = server.endpoint()
			case c.Answer.Error == "":
				server = answering(t, c.Answer.Status, c.Answer.Body)
				target = server.endpoint()
			case c.Answer.Error == "timeout":
				server = silent(t)
				target = server.endpoint()
				timeout = silenceTimeout
			case c.Answer.Error == "stall":
				server = stalling(t, c.Answer.Status, c.Answer.Body)
				target = server.endpoint()
				timeout = silenceTimeout
			case c.Answer.Error == "connection":
				target = refusingEndpoint(t)
			default:
				t.Fatalf("unknown answer error %s", c.Answer.Error)
			}

			client, err := newClient(secret, timeout, target)
			if err != nil {
				t.Fatal(err)
			}
			verification, err := bounded(t, func() (Verification, error) {
				return client.VerifyWith(context.Background(), c.Input["response"], c.Input["sitekey"], c.Input["remoteIp"])
			})

			switch c.Expect.Outcome {
			case "passed":
				passed, ok := verification.(*Passed)
				if !ok || err != nil {
					t.Fatalf("got %#v, %v; want *Passed", verification, err)
				}
				if passed.ChallengeTS != c.Expect.ChallengeTs || passed.Hostname != c.Expect.Hostname {
					t.Errorf("got %+v; want challenge time %q and hostname %q", passed, c.Expect.ChallengeTs, c.Expect.Hostname)
				}
			case "rejected":
				rejected, ok := verification.(*Rejected)
				if !ok || err != nil {
					t.Fatalf("got %#v, %v; want *Rejected", verification, err)
				}
				if !slices.Equal(rejected.ErrorCodes, c.Expect.ErrorCodes) {
					t.Errorf("got error codes %q; want %q", rejected.ErrorCodes, c.Expect.ErrorCodes)
				}
			case "unavailable":
				var unavailable *UnavailableError
				if verification != nil || !errors.As(err, &unavailable) {
					t.Fatalf("got %#v, %v; want an *UnavailableError", verification, err)
				}
				expectNoSecret(t, secret, err)
			default:
				t.Fatalf("unknown outcome %s", c.Expect.Outcome)
			}

			expectRequest(t, c.Request, server)
		})
	}
}

// bounded fails a verification whose timeout does not take effect instead of letting it hang.
func bounded(t *testing.T, verify func() (Verification, error)) (Verification, error) {
	t.Helper()

	type result struct {
		verification Verification
		err          error
	}
	done := make(chan result, 1)
	go func() {
		verification, err := verify()
		done <- result{verification, err}
	}()

	select {
	case r := <-done:
		return r.verification, r.err
	case <-time.After(caseBound):
		t.Fatalf("the verification did not end within %s", caseBound)
		return nil, nil
	}
}

// expectNoSecret checks the error and every error it wraps, in every form fmt prints them.
func expectNoSecret(t *testing.T, secret string, err error) {
	t.Helper()

	if err == nil {
		return
	}
	for _, form := range []string{"%v", "%+v", "%#v", "%s"} {
		if text := fmt.Sprintf(form, err); strings.Contains(text, secret) {
			t.Errorf("the secret key is in %s", text)
		}
	}

	switch wrapped := err.(type) {
	case interface{ Unwrap() error }:
		expectNoSecret(t, secret, wrapped.Unwrap())
	case interface{ Unwrap() []error }:
		for _, e := range wrapped.Unwrap() {
			expectNoSecret(t, secret, e)
		}
	}
}

func expectRequest(t *testing.T, request map[string]string, server *localSiteverify) {
	t.Helper()

	if request == nil {
		if server != nil && len(server.requests()) != 0 {
			t.Errorf("a request arrived: %+v", server.requests())
		}
		return
	}

	got := server.requests()
	if len(got) != 1 {
		t.Fatalf("%d requests arrived; want one", len(got))
	}
	if got[0].method != "POST" {
		t.Errorf("the method is %s; want POST", got[0].method)
	}
	if got[0].query != "" {
		t.Errorf("the URL has the query %q; want none", got[0].query)
	}
	if got[0].contentType != "application/x-www-form-urlencoded" {
		t.Errorf("the content type is %s; want application/x-www-form-urlencoded", got[0].contentType)
	}

	form := map[string]string{}
	for name, values := range got[0].form {
		if len(values) != 1 {
			t.Errorf("the field %s was sent %d times", name, len(values))
		}
		form[name] = values[0]
	}
	if !maps.Equal(form, request) {
		t.Errorf("the form is %v; want %v", form, request)
	}
}
