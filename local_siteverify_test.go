package captchafox

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

// received is a request that a localSiteverify received.
type received struct {
	method      string
	query       string
	contentType string
	form        url.Values
}

// localSiteverify is a siteverify on the loopback interface that records every request it receives.
type localSiteverify struct {
	server *httptest.Server
	closed chan struct{}

	mu       sync.Mutex
	log      []received
	brokenAt time.Time
}

func newLocalSiteverify(t *testing.T, answer func(s *localSiteverify, w http.ResponseWriter, r *http.Request)) *localSiteverify {
	t.Helper()

	s := &localSiteverify{closed: make(chan struct{})}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/siteverify" {
			http.NotFound(w, r)
			return
		}

		if err := r.ParseForm(); err != nil {
			t.Errorf("the request has no form: %v", err)
		}
		s.mu.Lock()
		s.log = append(s.log, received{method: r.Method, query: r.URL.RawQuery, contentType: r.Header.Get("Content-Type"), form: r.PostForm})
		s.mu.Unlock()

		answer(s, w, r)
	}))
	// Closing the channel first ends the handlers that wait, so that Close does not wait for them.
	t.Cleanup(func() {
		close(s.closed)
		s.server.Close()
	})
	return s
}

func answering(t *testing.T, status int, body string) *localSiteverify {
	return newLocalSiteverify(t, func(_ *localSiteverify, w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

// silent receives requests and never answers them while it runs.
func silent(t *testing.T) *localSiteverify {
	return newLocalSiteverify(t, func(s *localSiteverify, _ http.ResponseWriter, r *http.Request) {
		s.await(r)
	})
}

// stalling sends the status, the headers and the start of a body, then nothing more while it runs.
func stalling(t *testing.T, status int, start string) *localSiteverify {
	return newLocalSiteverify(t, func(s *localSiteverify, w http.ResponseWriter, r *http.Request) {
		sendPart(w, status, start)
		s.await(r)
	})
}

// trickling sends the status, the headers and the start of a body, then a space every 100 ms until the client goes.
func trickling(t *testing.T, status int, start string) *localSiteverify {
	return newLocalSiteverify(t, func(s *localSiteverify, w http.ResponseWriter, r *http.Request) {
		sendPart(w, status, start)

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				// The server cancels the request's context when the client closes the connection.
				s.mu.Lock()
				s.brokenAt = time.Now()
				s.mu.Unlock()
				return
			case <-s.closed:
				return
			case <-ticker.C:
				_, _ = w.Write([]byte(" "))
				w.(http.Flusher).Flush()
			}
		}
	})
}

// redirecting answers with the status and a Location to the target.
func redirecting(t *testing.T, status int, target string) *localSiteverify {
	return newLocalSiteverify(t, func(_ *localSiteverify, w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, status)
	})
}

// refusingEndpoint is an endpoint at a port of the loopback interface where nothing listens.
func refusingEndpoint(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return "http://" + address + "/siteverify"
}

func (s *localSiteverify) endpoint() string {
	return s.server.URL + "/siteverify"
}

func (s *localSiteverify) requests() []received {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]received(nil), s.log...)
}

// brokenAtTime is when a trickling server found the connection closed; zero while it is open.
func (s *localSiteverify) brokenAtTime() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.brokenAt
}

func (s *localSiteverify) await(r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-s.closed:
	}
}

// sendPart sends a body without a length, which has no end until the handler returns.
func sendPart(w http.ResponseWriter, status int, start string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(start))
	w.(http.Flusher).Flush()
}
