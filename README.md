# CaptchaFox for Go

Official [CaptchaFox](https://captchafox.com) integration for Go. It verifies the answers of the widget with
CaptchaFox.

## Compatibility

| Go | Library |
|---|---|
| 1.26, 1.27 | 1.x |

Go 1.25 and older are not supported: the module requires Go 1.26, so the go command refuses it or, where
`GOTOOLCHAIN` allows, switches your module to Go 1.26.

## Installation

```bash
go get github.com/CaptchaFox/captchafox-go
```

The library needs nothing but the standard library.

## Configuration

### Get your keys

In the [CaptchaFox portal](https://portal.captchafox.com), open your website under **Sites > Site Details** and copy
the **site key** and the **secret key**.

### Set up the library

Create a `Client` once with the secret key of your site and reuse it; it is safe for concurrent use. Keep the secret
key in your configuration or environment, not in source code:

```go
client, err := captchafox.NewClient(os.Getenv("CAPTCHAFOX_SECRET"))
if err != nil {
	// The secret key is empty.
}
```

### Options

| Option | Default | Meaning |
|---|---|---|
| `timeout` | 5 seconds | How long a verification may take, from connecting to CaptchaFox to the end of its answer: `captchafox.NewClientWithTimeout(secret, 3*time.Second)` |

## Usage

The widget adds the token to the form as the POST parameter `cf-captcha-response`. Verify it on the server before
the form is processed:

```go
import "github.com/CaptchaFox/captchafox-go"

token := r.PostFormValue("cf-captcha-response")
verification, err := client.VerifyWith(r.Context(), token, siteKey, remoteIP)
if err != nil {
	// CaptchaFox gave no usable answer: decide whether the form passes.
} else if _, passed := verification.(*captchafox.Passed); !passed {
	// Reject the form.
}
```

`Verify(ctx, token)` sends the token alone; `VerifyWith(ctx, token, sitekey, remoteIP)` also sends the site key the
token was issued for and the visitor's IP address, each only when it is not empty. A verification ends when the
context ends or the timeout passes, whichever comes first.

A verification has one of three outcomes:

- `*captchafox.Passed`: CaptchaFox accepted the token. `ChallengeTS` and `Hostname` hold the time of the challenge
  and the hostname of the site, where CaptchaFox sent them.
- `*captchafox.Rejected`: CaptchaFox rejected the token; `ErrorCodes` holds its error codes, such as
  `timeout-or-duplicate`, possibly none. An empty token is rejected with `missing-input-response` without asking
  CaptchaFox.
- `*captchafox.UnavailableError`: CaptchaFox gave no usable answer; see
  [If CaptchaFox is unreachable](#if-captchafox-is-unreachable).

## Operation

### If CaptchaFox is unreachable

When CaptchaFox cannot be reached, does not complete its answer within the timeout, answers with an HTTP status other
than 200, or with a body that is no valid answer, `Verify` and `VerifyWith` return an `*UnavailableError`; it is the
only error they return, and `errors.As` finds it. It wraps the cause, so `errors.Is(err, context.Canceled)` tells a
cancelled context. Your code decides whether the form passes then: letting it through keeps your forms available
during an outage, rejecting it keeps them protected. Tokens that CaptchaFox rejects are always `*captchafox.Rejected`.

### Log

The library logs nothing. Its errors never contain the secret key, and printing a `Client` with `fmt` does not show
it.

### Visitor IP

The visitor's IP address is sent only when you pass it as `remoteIP`. Behind a reverse proxy or load balancer, pass
the visitor's address that the proxy forwards, not the address of the proxy.

### Privacy

CaptchaFox is operated by Scoria Labs GmbH, Germany. How CaptchaFox processes data and how to get a data processing
agreement is described in the [CaptchaFox Privacy Center](https://captchafox.com/privacy). A suggested paragraph for
your privacy policy is on the [GDPR page](https://captchafox.com/privacy/gdpr).

## License

Apache-2.0. See [LICENSE](LICENSE).
