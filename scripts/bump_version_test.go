package scripts_test

// Drives the Makefile's bump-version target against a copy of the Makefile in
// a temporary directory: the cap's file changes (the changelog collapse and the
// version.json stamp) in one direction, and every refusal in the other, so a
// cap cut with it is what the release workflow's gates expect.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	bumpChangelog = "# Changelog\n\n## [Unreleased]\n\n### Added\n\n- **The new thing.**\n\n## [0.0.1] - 2026-01-01\n\n- Old.\n"
	bumpManifest  = "{\n  \"version\": \"0.0.1\"\n}\n"
)

// bumpFixture copies the repository's Makefile beside a changelog and a
// manifest in a fresh directory, so the target under test is the real recipe.
func bumpFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	makefile, err := os.ReadFile(filepath.Join("..", "Makefile"))
	if err != nil {
		t.Fatalf("reading the Makefile: %v", err)
	}
	for name, body := range map[string][]byte{
		"Makefile":     makefile,
		"CHANGELOG.md": []byte(bumpChangelog),
		"version.json": []byte(bumpManifest),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	return dir
}

// runBump invokes the target with the given VERSION assignment (or none) and
// returns its combined output and exit status, with make's inherited flags
// cleared so a parent `make VERSION=x test` cannot leak in.
func runBump(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("make", append([]string{"-C", dir, "bump-version"}, args...)...)
	cmd.Env = append(os.Environ(), "MAKEFLAGS=", "MFLAGS=", "VERSION=")
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("running make: %v\n%s", err, out)
	}
	return string(out), exit.ExitCode()
}

func readFixture(t *testing.T, dir, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(body)
}

func TestBumpVersionCollapsesTheChangelogAndStampsTheManifest(t *testing.T) {
	dir := bumpFixture(t)
	out, code := runBump(t, dir, "VERSION=1.0.0")
	if code != 0 {
		t.Fatalf("exit %d, want 0:\n%s", code, out)
	}
	changelog := readFixture(t, dir, "CHANGELOG.md")
	heading := "## [1.0.0] - " + time.Now().UTC().Format("2006-01-02")
	if !strings.Contains(changelog, heading+"\n") {
		t.Errorf("changelog lacks %q:\n%s", heading, changelog)
	}
	if strings.Contains(changelog, "## [Unreleased]") {
		t.Errorf("the [Unreleased] heading survived the collapse:\n%s", changelog)
	}
	if !strings.Contains(changelog, "## [0.0.1] - 2026-01-01\n") {
		t.Errorf("the released block moved:\n%s", changelog)
	}
	if got := readFixture(t, dir, "version.json"); got != "{\n  \"version\": \"1.0.0\"\n}\n" {
		t.Errorf("version.json = %q, want the 1.0.0 stamp", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "CHANGELOG.md.bak")); !os.IsNotExist(err) {
		t.Errorf("CHANGELOG.md.bak left behind (stat err = %v)", err)
	}
	if !strings.Contains(out, "chore(*): v1.0.0") {
		t.Errorf("the next steps do not name the cap subject:\n%s", out)
	}
}

func TestBumpVersionRefusesASecondBump(t *testing.T) {
	dir := bumpFixture(t)
	if out, code := runBump(t, dir, "VERSION=1.0.0"); code != 0 {
		t.Fatalf("first bump: exit %d:\n%s", code, out)
	}
	out, code := runBump(t, dir, "VERSION=1.0.1")
	if code == 0 || !strings.Contains(out, "no [Unreleased] block") {
		t.Errorf("second bump: exit %d, want non-zero naming the missing block:\n%s", code, out)
	}
}

func TestBumpVersionRefusesAPreReleaseAndChangesNothing(t *testing.T) {
	dir := bumpFixture(t)
	for _, v := range []string{"1.0.0-rc.1", "v1.0.0", "1.0"} {
		out, code := runBump(t, dir, "VERSION="+v)
		if code == 0 || !strings.Contains(out, "plain X.Y.Z") {
			t.Errorf("VERSION=%s: exit %d, want a plain-X.Y.Z refusal:\n%s", v, code, out)
		}
		if readFixture(t, dir, "CHANGELOG.md") != bumpChangelog || readFixture(t, dir, "version.json") != bumpManifest {
			t.Errorf("VERSION=%s: a refused bump changed the files", v)
		}
	}
}

func TestBumpVersionRefusesAMissingVersion(t *testing.T) {
	dir := bumpFixture(t)
	out, code := runBump(t, dir)
	if code == 0 || !strings.Contains(out, "usage: make bump-version VERSION=X.Y.Z") {
		t.Errorf("exit %d, want the usage refusal:\n%s", code, out)
	}
	if readFixture(t, dir, "version.json") != bumpManifest {
		t.Errorf("a refused bump stamped version.json")
	}
}

func TestBumpVersionRefusesANonCanonicalUnreleasedHeading(t *testing.T) {
	dir := bumpFixture(t)
	odd := strings.Replace(bumpChangelog, "## [Unreleased]\n", "## [Unreleased] - TBD\n", 1)
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(odd), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := runBump(t, dir, "VERSION=1.0.0")
	if code == 0 || !strings.Contains(out, "no [Unreleased] block") {
		t.Errorf("exit %d, want the missing-block refusal:\n%s", code, out)
	}
	if readFixture(t, dir, "CHANGELOG.md") != odd || readFixture(t, dir, "version.json") != bumpManifest {
		t.Errorf("a refused bump changed the files")
	}
}
