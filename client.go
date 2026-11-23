// Package captchafox verifies tokens of the CaptchaFox widget with siteverify, with the secret key of a site.
package captchafox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	endpoint       = "https://api.captchafox.com/siteverify"
	defaultTimeout = 5 * time.Second
)

// Client verifies tokens with the secret key of a site. It is safe for concurrent use; create it once and reuse it.
type Client struct {
	// Behind a pointer, because fmt prints a pointer inside a struct as its address, so no verb shows the key.
	secret   *string
	timeout  time.Duration
	endpoint string
	http     *http.Client
}

// NewClient returns a Client that gives a verification 5 seconds, from connecting to siteverify to the end of its
// answer. It returns an error if the secret key is empty.
func NewClient(secret string) (*Client, error) {
	return NewClientWithTimeout(secret, defaultTimeout)
}

// NewClientWithTimeout returns a Client that gives a verification the timeout instead of 5 seconds, from connecting to
// siteverify to the end of its answer. It returns an error if the secret key is empty or the timeout is not positive.
func NewClientWithTimeout(secret string, timeout time.Duration) (*Client, error) {
	return newClient(secret, timeout, endpoint)
}

func newClient(secret string, timeout time.Duration, endpoint string) (*Client, error) {
	if secret == "" {
		return nil, errors.New("captchafox: the secret key is empty")
	}

	if timeout <= 0 {
		return nil, errors.New("captchafox: the timeout is not positive")
	}

	return &Client{
		secret:   &secret,
		timeout:  timeout,
		endpoint: endpoint,
		http: &http.Client{
			// A redirect would carry the secret key to another address; 307 and 308 resend the body.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

// Verify verifies a token. It returns *Passed or *Rejected; an empty token is rejected with missing-input-response
// without a request. Every error it returns is an *UnavailableError.
func (c *Client) Verify(ctx context.Context, token string) (Verification, error) {
	return c.VerifyWith(ctx, token, "", "")
}

// VerifyWith is Verify for a token that was issued for a site key to a visitor's IP address; it sends the site key
// and the IP address only when they are not empty.
func (c *Client) VerifyWith(ctx context.Context, token, sitekey, remoteIP string) (Verification, error) {
	if token == "" {
		return &Rejected{ErrorCodes: []string{"missing-input-response"}}, nil
	}

	form := url.Values{}
	form.Set("secret", *c.secret)
	form.Set("response", token)
	if sitekey != "" {
		form.Set("sitekey", sitekey)
	}
	if remoteIP != "" {
		form.Set("remoteIp", remoteIP)
	}

	// One deadline over connect, headers and body, whether or not ctx has one.
	ctx, cancel := context.WithTimeoutCause(ctx, c.timeout, errTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, unavailable("the request to siteverify could not be built", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := c.http.Do(request)
	if err != nil {
		return nil, c.failed(ctx, "siteverify could not be reached", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, unavailable(fmt.Sprintf("siteverify answered with HTTP status %d", response.StatusCode), nil)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, c.failed(ctx, "the answer of siteverify broke off", err)
	}

	return evaluate(body)
}

var errTimeout = errors.New("timeout")

func (c *Client) failed(ctx context.Context, message string, err error) *UnavailableError {
	if context.Cause(ctx) == errTimeout {
		return unavailable(fmt.Sprintf("siteverify did not answer within %s", c.timeout), err)
	}
	return unavailable(message, err)
}

func evaluate(body []byte) (Verification, error) {
	fields, err := members(body)
	if err != nil {
		return nil, err
	}

	// Into an interface, because null would leave a bool unchanged; a missing member does not decode and stays nil.
	var success any
	_ = json.Unmarshal(fields["success"], &success)
	passed, ok := success.(bool)
	if !ok {
		return nil, unavailable("siteverify answered without a boolean success", nil)
	}

	if passed {
		return &Passed{ChallengeTS: text(fields["challenge_ts"]), Hostname: text(fields["hostname"])}, nil
	} else {
		return &Rejected{ErrorCodes: texts(fields["error-codes"])}, nil
	}
}

// members reads the members of a JSON object by their exact names; decoding into a struct would match them
// regardless of case.
func members(body []byte) (map[string]json.RawMessage, error) {
	if !json.Valid(body) {
		return nil, unavailable("siteverify answered with a body that is no JSON", nil)
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	if start, err := decoder.Token(); err != nil || start != json.Delim('{') {
		return nil, unavailable("siteverify answered with JSON that is no object", nil)
	}

	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, unavailable("siteverify answered with a body that is no JSON", err)
		}
		name := token.(string)

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, unavailable("siteverify answered with a body that is no JSON", err)
		}

		// A repeated name, such as success twice, has no defined meaning, so the answer is malformed.
		if _, seen := fields[name]; seen {
			return nil, unavailable(fmt.Sprintf("siteverify answered with the member %q twice", name), nil)
		}
		fields[name] = value
	}
	return fields, nil
}

func text(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return value
}

// A rejection stays a rejection when its error codes are malformed; only the entries that are strings are kept.
func texts(raw json.RawMessage) []string {
	codes := []string{}
	var values []any
	if json.Unmarshal(raw, &values) != nil {
		return codes
	}

	for _, value := range values {
		if code, ok := value.(string); ok {
			codes = append(codes, code)
		}
	}
	return codes
}
