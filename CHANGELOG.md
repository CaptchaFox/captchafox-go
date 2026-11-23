## 1.0.0

- Verify CaptchaFox tokens with siteverify: `Client.Verify` returns `*Passed` or `*Rejected`, and an
  `*UnavailableError` when CaptchaFox cannot be reached in time or answers unexpectedly.
- Go 1.26 and later, no dependencies.
