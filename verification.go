package captchafox

// Verification is the outcome of a token that siteverify evaluated: *Passed or *Rejected, never both and nothing
// else. A token whose verification cannot be evaluated has no Verification; Verify returns an *UnavailableError
// instead.
type Verification interface {
	// Unexported, so that *Passed and *Rejected are the only outcomes. With pointer receivers, a type switch on
	// Passed or Rejected without the pointer does not compile.
	verification()
}

// Passed means siteverify answered "success": true.
type Passed struct {
	// The time of the challenge as siteverify sent it, an ISO 8601 timestamp; empty if it sent none.
	ChallengeTS string
	// The hostname of the site where the challenge was solved; empty if siteverify sent none.
	Hostname string
}

// Rejected means siteverify answered "success": false, or the token was empty.
type Rejected struct {
	// The error codes of siteverify, such as timeout-or-duplicate; possibly none.
	ErrorCodes []string
}

func (*Passed) verification()   {}
func (*Rejected) verification() {}

// UnavailableError means siteverify could not be evaluated: it was not reached, did not answer in time, answered with
// an HTTP status other than 200, or with a body that is no JSON object with a boolean success. Whether the protected
// request passes then is the caller's decision. Its message never contains the secret key.
type UnavailableError struct {
	message string
	cause   error
}

func unavailable(message string, cause error) *UnavailableError {
	return &UnavailableError{message: message, cause: cause}
}

func (e *UnavailableError) Error() string {
	if e.cause == nil {
		return "captchafox: " + e.message
	}
	return "captchafox: " + e.message + ": " + e.cause.Error()
}

// Unwrap returns the error that made siteverify unavailable, such as that of the network or of a cancelled context.
func (e *UnavailableError) Unwrap() error {
	return e.cause
}
