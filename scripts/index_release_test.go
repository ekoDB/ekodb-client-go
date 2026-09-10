package scripts_test

// Drives scripts/index-release.sh against local stand-ins for the Go module
// proxy and pkg.go.dev, so the release step that makes a pushed tag visible on
// pkg.go.dev is proven in both directions: it exits 0 only once the version
// page renders, and it names the failing URL and status when a step does not.

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const (
	module        = "github.com/ekoDB/ekodb-client-go"
	escapedModule = "github.com/eko!d!b/ekodb-client-go"
	version       = "v1.2.3"
	proxyInfoPath = "/" + escapedModule + "/@v/" + version + ".info"
	fetchPath     = "/fetch/" + module + "@" + version
	pagePath      = "/" + module + "@" + version
)

// recorder is an httptest handler that remembers every request it served and
// answers each path with a scripted sequence of status codes (the last code
// repeats once the sequence is exhausted).
type recorder struct {
	mu       sync.Mutex
	requests []string // "METHOD path"
	statuses map[string][]int
	served   map[string]int
}

func newRecorder(statuses map[string][]int) *recorder {
	return &recorder{statuses: statuses, served: map[string]int{}}
}

func (r *recorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req.Method+" "+req.URL.Path)
	seq, ok := r.statuses[req.URL.Path]
	if !ok {
		http.NotFound(w, req)
		return
	}
	i := r.served[req.URL.Path]
	if i >= len(seq) {
		i = len(seq) - 1
	}
	r.served[req.URL.Path]++
	w.WriteHeader(seq[i])
	_, _ = w.Write([]byte(http.StatusText(seq[i])))
}

func (r *recorder) count(prefix string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, req := range r.requests {
		if strings.HasPrefix(req, prefix) {
			n++
		}
	}
	return n
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.requests...)
}

// run executes the script with the two stub servers and a zero poll delay.
func run(t *testing.T, proxy, site *httptest.Server, attempts string, args ...string) (int, string) {
	t.Helper()
	script, err := filepath.Abs("index-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(),
		"GOPROXY_URL="+proxy.URL,
		"PKGSITE_URL="+site.URL,
		"INDEX_ATTEMPTS="+attempts,
		"INDEX_SLEEP=0",
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err = cmd.Run()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running script: %v", err)
	}
	return code, out.String()
}

func TestIndexRelease_SuccessRequestsProxyThenFetchThenPage(t *testing.T) {
	proxy := newRecorder(map[string][]int{proxyInfoPath: {200}})
	site := newRecorder(map[string][]int{fetchPath: {200}, pagePath: {200}})
	ps, ss := httptest.NewServer(proxy), httptest.NewServer(site)
	defer ps.Close()
	defer ss.Close()

	code, out := run(t, ps, ss, "3", version)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d:\n%s", code, out)
	}
	if got := proxy.snapshot(); len(got) != 1 || got[0] != "GET "+proxyInfoPath {
		t.Fatalf("proxy requests = %v, want exactly one GET of the escaped .info path", got)
	}
	if got := site.snapshot(); len(got) != 2 || got[0] != "POST "+fetchPath || got[1] != "GET "+pagePath {
		t.Fatalf("pkg.go.dev requests = %v, want POST fetch then GET page", got)
	}
	if !strings.Contains(out, ss.URL+pagePath) {
		t.Fatalf("success output should name the version page, got:\n%s", out)
	}
}

func TestIndexRelease_PollsPageUntilItRenders(t *testing.T) {
	proxy := newRecorder(map[string][]int{proxyInfoPath: {200}})
	site := newRecorder(map[string][]int{fetchPath: {200}, pagePath: {404, 404, 200}})
	ps, ss := httptest.NewServer(proxy), httptest.NewServer(site)
	defer ps.Close()
	defer ss.Close()

	code, out := run(t, ps, ss, "5", version)
	if code != 0 {
		t.Fatalf("expected exit 0 once the page renders, got %d:\n%s", code, out)
	}
	if n := site.count("GET " + pagePath); n != 3 {
		t.Fatalf("page polled %d times, want 3 (two 404s then a 200)", n)
	}
}

func TestIndexRelease_ProxyMissFailsBeforeTouchingPkgsite(t *testing.T) {
	proxy := newRecorder(map[string][]int{proxyInfoPath: {404}})
	site := newRecorder(map[string][]int{fetchPath: {200}, pagePath: {200}})
	ps, ss := httptest.NewServer(proxy), httptest.NewServer(site)
	defer ps.Close()
	defer ss.Close()

	code, out := run(t, ps, ss, "3", version)
	if code != 1 {
		t.Fatalf("expected exit 1 on a proxy 404, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, ps.URL+proxyInfoPath) || !strings.Contains(out, "404") {
		t.Fatalf("failure output must name the proxy URL and status, got:\n%s", out)
	}
	if got := site.snapshot(); len(got) != 0 {
		t.Fatalf("pkg.go.dev must not be contacted after a proxy miss, got %v", got)
	}
}

func TestIndexRelease_FetchNotFoundFailsWithoutPolling(t *testing.T) {
	proxy := newRecorder(map[string][]int{proxyInfoPath: {200}})
	site := newRecorder(map[string][]int{fetchPath: {404}, pagePath: {200}})
	ps, ss := httptest.NewServer(proxy), httptest.NewServer(site)
	defer ps.Close()
	defer ss.Close()

	code, out := run(t, ps, ss, "3", version)
	if code != 1 {
		t.Fatalf("expected exit 1 on a fetch 404, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, ss.URL+fetchPath) || !strings.Contains(out, "404") {
		t.Fatalf("failure output must name the fetch URL and status, got:\n%s", out)
	}
	if n := site.count("GET " + pagePath); n != 0 {
		t.Fatalf("page must not be polled after fetch reports not found, polled %d times", n)
	}
}

func TestIndexRelease_FetchTransientFailureStillPolls(t *testing.T) {
	// pkg.go.dev answers the fetch endpoint with a 5xx while its worker is
	// still processing; the page rendering is the success criterion, so the
	// script keeps polling rather than giving up on that code.
	proxy := newRecorder(map[string][]int{proxyInfoPath: {200}})
	site := newRecorder(map[string][]int{fetchPath: {500}, pagePath: {404, 200}})
	ps, ss := httptest.NewServer(proxy), httptest.NewServer(site)
	defer ps.Close()
	defer ss.Close()

	code, out := run(t, ps, ss, "3", version)
	if code != 0 {
		t.Fatalf("expected exit 0 when the page renders after a transient fetch error, got %d:\n%s", code, out)
	}
	if n := site.count("GET " + pagePath); n != 2 {
		t.Fatalf("page polled %d times, want 2", n)
	}
}

func TestIndexRelease_PollTimeoutFailsNamingThePage(t *testing.T) {
	proxy := newRecorder(map[string][]int{proxyInfoPath: {200}})
	site := newRecorder(map[string][]int{fetchPath: {200}, pagePath: {404}})
	ps, ss := httptest.NewServer(proxy), httptest.NewServer(site)
	defer ps.Close()
	defer ss.Close()

	code, out := run(t, ps, ss, "3", version)
	if code != 1 {
		t.Fatalf("expected exit 1 when the page never renders, got %d:\n%s", code, out)
	}
	if n := site.count("GET " + pagePath); n != 3 {
		t.Fatalf("page polled %d times, want exactly INDEX_ATTEMPTS=3", n)
	}
	if !strings.Contains(out, ss.URL+pagePath) || !strings.Contains(out, "404") {
		t.Fatalf("timeout output must name the page URL and last status, got:\n%s", out)
	}
}

func TestIndexRelease_RejectsMalformedVersionBeforeAnyRequest(t *testing.T) {
	proxy := newRecorder(map[string][]int{proxyInfoPath: {200}})
	site := newRecorder(map[string][]int{fetchPath: {200}, pagePath: {200}})
	ps, ss := httptest.NewServer(proxy), httptest.NewServer(site)
	defer ps.Close()
	defer ss.Close()

	for _, bad := range []string{"", "1.2.3", "v1.2", "v1.2.3-rc.1"} {
		args := []string{bad}
		if bad == "" {
			args = nil
		}
		code, out := run(t, ps, ss, "3", args...)
		if code != 2 {
			t.Errorf("version %q: expected exit 2, got %d:\n%s", bad, code, out)
		}
	}
	if got := proxy.snapshot(); len(got) != 0 {
		t.Fatalf("proxy must not be contacted for a malformed version, got %v", got)
	}
	if got := site.snapshot(); len(got) != 0 {
		t.Fatalf("pkg.go.dev must not be contacted for a malformed version, got %v", got)
	}
}
