package scripts_test

// Drives scripts/index-release.sh against local stand-ins for the Go module
// proxy and pkg.go.dev, so the release step that makes a pushed tag visible on
// pkg.go.dev is proven in both directions: it exits 0 only once the version
// page renders, and it names the failing URL and status when a step does not.

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	module        = "github.com/ekoDB/ekodb-client-go"
	escapedModule = "github.com/eko!d!b/ekodb-client-go"
	version       = "v1.2.3"
	proxyInfoPath = "/" + escapedModule + "/@v/" + version + ".info"
	fetchPath     = "/fetch/" + module + "@" + version
	pagePath      = "/" + module + "@" + version
)

// hangFor is how long a scripted status of 0 makes the recorder hold a request
// before answering; longer than any REQUEST_TIMEOUT the tests set, so curl
// gives up first and reports its own timeout.
const hangFor = 2 * time.Second

// recorder is an httptest handler that remembers every request it served and
// answers each path with a scripted sequence of status codes (the last code
// repeats once the sequence is exhausted; a 0 holds the request for hangFor).
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
	status := seq[i]
	if status == 0 {
		r.mu.Unlock()
		time.Sleep(hangFor)
		r.mu.Lock()
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(http.StatusText(status)))
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

// stubs starts a proxy and a pkg.go.dev stand-in with the given status
// scripts and returns their recorders and base URLs.
func stubs(t *testing.T, proxyStatuses, siteStatuses map[string][]int) (*recorder, string, *recorder, string) {
	t.Helper()
	proxy, site := newRecorder(proxyStatuses), newRecorder(siteStatuses)
	ps, ss := httptest.NewServer(proxy), httptest.NewServer(site)
	t.Cleanup(ps.Close)
	t.Cleanup(ss.Close)
	return proxy, ps.URL, site, ss.URL
}

// polling is the script's poll and timeout configuration, as the strings it
// reads; an empty timeout leaves the script's defaults in place.
type polling struct{ attempts, sleep, timeout string }

func fast(attempts string) polling { return polling{attempts: attempts, sleep: "0"} }

// run executes the repository's script against the two base URLs.
func run(t *testing.T, proxyURL, siteURL string, p polling, args ...string) (int, string) {
	t.Helper()
	script, err := filepath.Abs("index-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	return runScript(t, script, proxyURL, siteURL, p, args...)
}

// runDeadline bounds every script run: a script that hangs (a request with
// no timeout against a server that never answers) fails the test here rather
// than stalling the package until go test gives up.
const runDeadline = 20 * time.Second

// runScript executes the script at the given path against the two base URLs.
func runScript(t *testing.T, script, proxyURL, siteURL string, p polling, args ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), runDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", append([]string{script}, args...)...)
	// On the deadline the parent bash is killed, but a command substitution
	// still running a request holds the inherited pipes; WaitDelay closes
	// them so the run returns instead of waiting on that orphan.
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = append(os.Environ(),
		"GOPROXY_URL="+proxyURL,
		"PKGSITE_URL="+siteURL,
		"INDEX_ATTEMPTS="+p.attempts,
		"INDEX_SLEEP="+p.sleep,
	)
	if p.timeout != "" {
		cmd.Env = append(cmd.Env, "CONNECT_TIMEOUT="+p.timeout, "REQUEST_TIMEOUT="+p.timeout)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("script did not finish within %v:\n%s", runDeadline, out.String())
	}
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running script: %v", err)
	}
	return code, out.String()
}

// closedPort returns a loopback URL nothing is listening on.
func closedPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return "http://" + addr
}

// blackHole returns a URL whose server accepts every connection and never
// answers, so only a client-side timeout can end a request to it.
func blackHole(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var conns []net.Conn
	var mu sync.Mutex
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, c)
			mu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = l.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
	})
	return "http://" + l.Addr().String()
}

// scriptBeside copies the script into a fresh directory whose parent holds
// the given go.mod content (none when goMod is nil), so the module lookup can
// be driven against a missing or incomplete file.
func scriptBeside(t *testing.T, goMod []byte) string {
	t.Helper()
	src, err := os.ReadFile("index-release.sh")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dir := filepath.Join(root, "scripts")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "index-release.sh")
	if err := os.WriteFile(script, src, 0o755); err != nil {
		t.Fatal(err)
	}
	if goMod != nil {
		if err := os.WriteFile(filepath.Join(root, "go.mod"), goMod, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return script
}

func TestIndexRelease_SuccessRequestsProxyThenFetchThenPage(t *testing.T) {
	proxy, proxyURL, site, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {200}},
		map[string][]int{fetchPath: {200}, pagePath: {200}})

	code, out := run(t, proxyURL, siteURL, fast("3"), version)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d:\n%s", code, out)
	}
	if got := proxy.snapshot(); len(got) != 1 || got[0] != "GET "+proxyInfoPath {
		t.Fatalf("proxy requests = %v, want exactly one GET of the escaped .info path", got)
	}
	if got := site.snapshot(); len(got) != 2 || got[0] != "POST "+fetchPath || got[1] != "GET "+pagePath {
		t.Fatalf("pkg.go.dev requests = %v, want POST fetch then GET page", got)
	}
	if !strings.Contains(out, siteURL+pagePath) {
		t.Fatalf("success output should name the version page, got:\n%s", out)
	}
}

func TestIndexRelease_PollsPageUntilItRenders(t *testing.T) {
	_, proxyURL, site, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {200}},
		map[string][]int{fetchPath: {200}, pagePath: {404, 404, 200}})

	code, out := run(t, proxyURL, siteURL, fast("5"), version)
	if code != 0 {
		t.Fatalf("expected exit 0 once the page renders, got %d:\n%s", code, out)
	}
	if n := site.count("GET " + pagePath); n != 3 {
		t.Fatalf("page polled %d times, want 3 (two 404s then a 200)", n)
	}
}

func TestIndexRelease_PollsProxyUntilItServesTheVersion(t *testing.T) {
	// Seconds after a tag push the proxy's first fetch from origin may not
	// have completed; the proxy step retries within the same budget.
	proxy, proxyURL, site, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {404, 200}},
		map[string][]int{fetchPath: {200}, pagePath: {200}})

	code, out := run(t, proxyURL, siteURL, fast("3"), version)
	if code != 0 {
		t.Fatalf("expected exit 0 once the proxy serves the version, got %d:\n%s", code, out)
	}
	if n := proxy.count("GET " + proxyInfoPath); n != 2 {
		t.Fatalf("proxy polled %d times, want 2 (a 404 then a 200)", n)
	}
	if got := site.snapshot(); len(got) != 2 {
		t.Fatalf("pkg.go.dev requests = %v, want fetch then page after the proxy succeeded", got)
	}
}

func TestIndexRelease_ProxyMissFailsBeforeTouchingPkgsite(t *testing.T) {
	proxy, proxyURL, site, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {404}},
		map[string][]int{fetchPath: {200}, pagePath: {200}})

	code, out := run(t, proxyURL, siteURL, fast("3"), version)
	if code != 1 {
		t.Fatalf("expected exit 1 on a proxy 404, got %d:\n%s", code, out)
	}
	if n := proxy.count("GET " + proxyInfoPath); n != 3 {
		t.Fatalf("proxy polled %d times, want exactly INDEX_ATTEMPTS=3", n)
	}
	if !strings.Contains(out, proxyURL+proxyInfoPath) || !strings.Contains(out, "HTTP 404") {
		t.Fatalf("failure output must name the proxy URL and status, got:\n%s", out)
	}
	if got := site.snapshot(); len(got) != 0 {
		t.Fatalf("pkg.go.dev must not be contacted after a proxy miss, got %v", got)
	}
}

func TestIndexRelease_ConnectionFailureReportsCurlDetailOnce(t *testing.T) {
	// No status at all is a different reading from a 404: the output carries a
	// single 000 and curl's own message, never a doubled "000000".
	proxyURL, siteURL := closedPort(t), closedPort(t)

	code, out := run(t, proxyURL, siteURL, fast("2"), version)
	if code != 1 {
		t.Fatalf("expected exit 1 when the proxy cannot be reached, got %d:\n%s", code, out)
	}
	if strings.Contains(out, "000000") {
		t.Fatalf("status must not be doubled on a connection failure, got:\n%s", out)
	}
	if !regexp.MustCompile(`HTTP 000 \(curl: \(\d+\) `).MatchString(out) {
		t.Fatalf("connection failure must print HTTP 000 with curl's message, got:\n%s", out)
	}
	if !strings.Contains(out, proxyURL+proxyInfoPath) {
		t.Fatalf("failure output must name the proxy URL, got:\n%s", out)
	}
}

func TestIndexRelease_FetchNotFoundFailsWithoutPolling(t *testing.T) {
	_, proxyURL, site, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {200}},
		map[string][]int{fetchPath: {404}, pagePath: {200}})

	code, out := run(t, proxyURL, siteURL, fast("3"), version)
	if code != 1 {
		t.Fatalf("expected exit 1 on a fetch 404, got %d:\n%s", code, out)
	}
	if !strings.Contains(out, siteURL+fetchPath) || !strings.Contains(out, "HTTP 404") {
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
	_, proxyURL, site, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {200}},
		map[string][]int{fetchPath: {500}, pagePath: {404, 200}})

	code, out := run(t, proxyURL, siteURL, fast("3"), version)
	if code != 0 {
		t.Fatalf("expected exit 0 when the page renders after a transient fetch error, got %d:\n%s", code, out)
	}
	if n := site.count("GET " + pagePath); n != 2 {
		t.Fatalf("page polled %d times, want 2", n)
	}
}

func TestIndexRelease_PollTimeoutFailsNamingThePage(t *testing.T) {
	_, proxyURL, site, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {200}},
		map[string][]int{fetchPath: {200}, pagePath: {404}})

	code, out := run(t, proxyURL, siteURL, fast("3"), version)
	if code != 1 {
		t.Fatalf("expected exit 1 when the page never renders, got %d:\n%s", code, out)
	}
	if n := site.count("GET " + pagePath); n != 3 {
		t.Fatalf("page polled %d times, want exactly INDEX_ATTEMPTS=3", n)
	}
	if !strings.Contains(out, siteURL+pagePath) || !strings.Contains(out, "HTTP 404") {
		t.Fatalf("timeout output must name the page URL and last status, got:\n%s", out)
	}
}

func TestIndexRelease_SleepsBetweenAttemptsButNotAfterTheLast(t *testing.T) {
	_, proxyURL, _, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {200}},
		map[string][]int{fetchPath: {200}, pagePath: {404}})

	// The sleep is long relative to a run that makes no sleep at all (tens of
	// milliseconds), so the upper bound below has a wide margin on a loaded
	// runner while still failing if the last attempt were followed by a sleep.
	const sleep = 2 * time.Second
	start := time.Now()
	code, out := run(t, proxyURL, siteURL, polling{attempts: "2", sleep: "2"}, version)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d:\n%s", code, out)
	}
	if elapsed := time.Since(start); elapsed < sleep {
		t.Fatalf("two attempts with INDEX_SLEEP=2 took %v, want at least one sleep between them", elapsed)
	}

	start = time.Now()
	code, out = run(t, proxyURL, siteURL, polling{attempts: "1", sleep: "2"}, version)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d:\n%s", code, out)
	}
	if elapsed := time.Since(start); elapsed >= sleep {
		t.Fatalf("a single attempt with INDEX_SLEEP=2 took %v, want no sleep after the last attempt", elapsed)
	}
}

func TestIndexRelease_RejectsMalformedVersionBeforeAnyRequest(t *testing.T) {
	proxy, proxyURL, site, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {200}},
		map[string][]int{fetchPath: {200}, pagePath: {200}})

	for _, bad := range []string{"", "1.2.3", "v1.2", "v1.2.3-rc.1"} {
		args := []string{bad}
		if bad == "" {
			args = nil
		}
		code, out := run(t, proxyURL, siteURL, fast("3"), args...)
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

func TestIndexRelease_RejectsInvalidPollingBeforeAnyRequest(t *testing.T) {
	proxy, proxyURL, site, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {200}},
		map[string][]int{fetchPath: {200}, pagePath: {200}})

	for _, p := range []polling{
		{attempts: "0", sleep: "0"},
		{attempts: "abc", sleep: "0"},
		{attempts: "-1", sleep: "0"},
		// leading zeros: bash would read these as octal inside (( )) and
		// error, which must be a rejection rather than a skipped check
		{attempts: "08", sleep: "0"},
		{attempts: "007", sleep: "0"},
		{attempts: "3", sleep: "x"},
		{attempts: "3", sleep: "-1"},
		{attempts: "3", sleep: "08"},
		{attempts: "3", sleep: "0", timeout: "0"},
		{attempts: "3", sleep: "0", timeout: "abc"},
	} {
		code, out := run(t, proxyURL, siteURL, p, version)
		if code != 2 {
			t.Errorf("INDEX_ATTEMPTS=%q INDEX_SLEEP=%q timeout=%q: expected exit 2, got %d:\n%s", p.attempts, p.sleep, p.timeout, code, out)
		}
		if !strings.Contains(out, "must be an integer") {
			t.Errorf("INDEX_ATTEMPTS=%q INDEX_SLEEP=%q timeout=%q: expected the script's own message, got:\n%s", p.attempts, p.sleep, p.timeout, out)
		}
	}
	if got := proxy.snapshot(); len(got) != 0 {
		t.Fatalf("proxy must not be contacted with invalid polling values, got %v", got)
	}
	if got := site.snapshot(); len(got) != 0 {
		t.Fatalf("pkg.go.dev must not be contacted with invalid polling values, got %v", got)
	}
}

func TestIndexRelease_UnresponsiveServerEndsAtTheRequestTimeout(t *testing.T) {
	// A server that accepts the connection and never answers can only be
	// ended by the client-side timeout; with the timeouts at one second the
	// run must finish far sooner than the 30-second default would allow.
	proxyURL := blackHole(t)
	siteURL := closedPort(t)

	start := time.Now()
	code, out := run(t, proxyURL, siteURL, polling{attempts: "1", sleep: "0", timeout: "1"}, version)
	elapsed := time.Since(start)
	if code != 1 {
		t.Fatalf("expected exit 1 when the proxy never answers, got %d:\n%s", code, out)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("run against an unresponsive server took %v, want it bounded by the one-second timeout", elapsed)
	}
	if !regexp.MustCompile(`HTTP 000 \(curl: \(28\) `).MatchString(out) {
		t.Fatalf("an unresponsive server must be reported as curl's timeout (28), got:\n%s", out)
	}
}

func TestIndexRelease_LaterAttemptDoesNotCarryAnEarlierRequestsDetail(t *testing.T) {
	// The proxy hangs on the first attempt (curl times out), answers 404 on
	// the second and 200 on the third. The 404 line must carry no trace of
	// the earlier timeout message, and the run must then succeed.
	proxy, proxyURL, site, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {0, 404, 200}},
		map[string][]int{fetchPath: {200}, pagePath: {200}})

	code, out := run(t, proxyURL, siteURL, polling{attempts: "3", sleep: "0", timeout: "1"}, version)
	if code != 0 {
		t.Fatalf("expected exit 0 once the proxy answers, got %d:\n%s", code, out)
	}
	if n := proxy.count("GET " + proxyInfoPath); n != 3 {
		t.Fatalf("proxy polled %d times, want 3", n)
	}
	if !regexp.MustCompile(`attempt 1/3: HTTP 000 \(curl: \(28\) `).MatchString(out) {
		t.Fatalf("first attempt must report curl's timeout, got:\n%s", out)
	}
	if !strings.Contains(out, "attempt 2/3: HTTP 404\n") {
		t.Fatalf("second attempt must report a bare 404 with no earlier detail, got:\n%s", out)
	}
	if got := site.snapshot(); len(got) != 2 {
		t.Fatalf("pkg.go.dev requests = %v, want fetch then page", got)
	}
}

func TestIndexRelease_MissingOrIncompleteGoModFailsBeforeAnyRequest(t *testing.T) {
	proxy, proxyURL, site, siteURL := stubs(t,
		map[string][]int{proxyInfoPath: {200}},
		map[string][]int{fetchPath: {200}, pagePath: {200}})

	for name, goMod := range map[string][]byte{
		"missing":        nil,
		"no module line": []byte("go 1.27\n"),
	} {
		script := scriptBeside(t, goMod)
		code, out := runScript(t, script, proxyURL, siteURL, fast("3"), version)
		if code != 1 {
			t.Errorf("%s go.mod: expected exit 1, got %d:\n%s", name, code, out)
		}
		if !strings.Contains(out, "could not read the module path from") {
			t.Errorf("%s go.mod: expected the script's own message, got:\n%s", name, out)
		}
		if strings.Contains(out, fmt.Sprintf("awk: can't open")) {
			t.Errorf("%s go.mod: awk's error must not reach the operator, got:\n%s", name, out)
		}
	}
	if got := proxy.snapshot(); len(got) != 0 {
		t.Fatalf("proxy must not be contacted without a module path, got %v", got)
	}
	if got := site.snapshot(); len(got) != 0 {
		t.Fatalf("pkg.go.dev must not be contacted without a module path, got %v", got)
	}
}
