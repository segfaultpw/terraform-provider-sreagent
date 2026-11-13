package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const script = "../../scripts/release-version.sh"

// repo is a throwaway git repository with a history built from commit subjects, so the rule runs
// against real `git describe` and `git log` and nothing of the real history.
type repo struct {
	t   *testing.T
	dir string
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	r := &repo{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	return r
}

func (r *repo) env() []string {
	return append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com",
	)
}

func (r *repo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...) //nolint:gosec // a fixed program with arguments written in this file
	cmd.Dir, cmd.Env = r.dir, r.env()
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (r *repo) commit(subject string, body ...string) *repo {
	r.t.Helper()
	args := []string{"-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", subject}
	for _, b := range body {
		args = append(args, "-m", b)
	}
	r.git(args...)
	return r
}

func (r *repo) tag(name string) *repo {
	r.t.Helper()
	r.git("-c", "tag.gpgsign=false", "tag", "-a", name, "-m", name)
	return r
}

func (r *repo) changelog(text string) *repo {
	r.t.Helper()
	if err := os.WriteFile(filepath.Join(r.dir, "CHANGELOG.md"), []byte(text), 0o600); err != nil {
		r.t.Fatal(err)
	}
	return r
}

func (r *repo) run(args ...string) (string, string, error) {
	r.t.Helper()
	abs, err := filepath.Abs(script)
	if err != nil {
		r.t.Fatal(err)
	}
	cmd := exec.Command("bash", append([]string{abs}, args...)...) //nolint:gosec // the script of this repository, with arguments written in this file
	cmd.Dir, cmd.Env = r.dir, r.env()
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

func (r *repo) next() string {
	r.t.Helper()
	out, stderr, err := r.run("compute")
	if err != nil {
		r.t.Fatalf("compute: %v\n%s", err, stderr)
	}
	return out
}

func TestTheNextVersionFollowsTheCommitsSinceTheLastTag(t *testing.T) {
	for _, c := range []struct {
		name string
		last string
		new  [][]string
		want string
	}{
		{"before 1.0 a feature bumps the minor", "v0.2.0", [][]string{{"feat(security): sreagent_secret_allow_rule"}}, "v0.3.0"},
		{"before 1.0 a fix bumps the patch", "v0.2.0", [][]string{{"fix(engine): a plan that read a null as a change"}}, "v0.2.1"},
		{"before 1.0 a fix on a later patch bumps the patch", "v0.2.4", [][]string{{"fix: x"}}, "v0.2.5"},
		{"before 1.0 a feature resets the patch", "v0.2.4", [][]string{{"feat: x"}}, "v0.3.0"},
		{"before 1.0 a breaking marker bumps the minor", "v0.2.0", [][]string{{"refactor(engine)!: rename an attribute"}}, "v0.3.0"},
		{"before 1.0 a breaking footer bumps the minor", "v0.2.0", [][]string{{"fix: x", "BREAKING CHANGE: the attribute is gone"}}, "v0.3.0"},
		{"a feature beats a fix", "v0.2.0", [][]string{{"fix: a"}, {"feat: b"}, {"fix: c"}}, "v0.3.0"},
		{"docs, ci, test, chore and refactor release nothing", "v0.2.0", [][]string{{"docs: a"}, {"ci: b"}, {"test: c"}, {"chore: d"}, {"refactor(engine): e"}}, "none"},
		{"no commit since the tag releases nothing", "v0.2.0", nil, "none"},
		{"a merge subject is not a conventional one", "v0.2.0", [][]string{{"Merge pull request #4 from segfaultpw/x"}}, "none"},
		{"a subject that only mentions feat is not a feature", "v0.2.0", [][]string{{"docs: say what a feat: subject does"}}, "none"},
		{"a scope that only contains a breaking marker is not breaking", "v1.4.2", [][]string{{"docs(readme): how fix(x)!: is read"}}, "none"},
		{"a body that mentions the footer in prose is not breaking", "v1.4.2", [][]string{{"docs: explain the rule", "A BREAKING CHANGE footer makes a major, and BREAKING CHANGES are listed in the changelog."}}, "none"},
		{"a footer line is breaking", "v1.4.2", [][]string{{"fix: x", "Explained at length.\n\nBREAKING CHANGE: the attribute is gone"}}, "v2.0.0"},
		{"a hyphenated footer is breaking", "v1.4.2", [][]string{{"fix: x", "BREAKING-CHANGE: the attribute is gone"}}, "v2.0.0"},
		{"a scope may not run past its parenthesis into a feature", "v0.2.0", [][]string{{"docs(a) then feat(b): not a feature"}}, "none"},
		{"from 1.0 a feature bumps the minor", "v1.4.2", [][]string{{"feat: x"}}, "v1.5.0"},
		{"from 1.0 a fix bumps the patch", "v1.4.2", [][]string{{"fix: x"}}, "v1.4.3"},
		{"from 1.0 a breaking marker bumps the major", "v1.4.2", [][]string{{"feat!: x"}}, "v2.0.0"},
		{"the branch of 0.3.0: four features and two refactors since v0.2.0", "v0.2.0", [][]string{
			{"refactor(engine): null as a value, the published bounds and defaults"},
			{"refactor(engine): plan checks, set lists, timestamps and the plan refusal"},
			{"feat(security): sreagent_security_sla_policy and sreagent_security_digest_settings"},
			{"feat(security): sreagent_security_code_classes"},
			{"feat(security): sreagent_security_notification_rule"},
			{"feat(security): sreagent_secret_allow_rule"},
			{"test: the security data sources keep their 0.2.0 schema, and the export adopts the security resources"},
			{"docs: the security resources, importing them, and 0.3.0"},
			{"ci(release): before 1.0 a feature bumps the minor"},
		}, "v0.3.0"},
		{"a branch of fixes only on v0.2.0", "v0.2.0", [][]string{{"fix(engine): a"}, {"fix(specs): b"}, {"docs: c"}}, "v0.2.1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRepo(t).commit("feat: before the tag").tag(c.last)
			for _, m := range c.new {
				r.commit(m[0], m[1:]...)
			}
			if got := r.next(); got != c.want {
				t.Fatalf("after %s the commits %v compute %q, want %q", c.last, c.new, got, c.want)
			}
		})
	}
}

// A commit made before the tag is not a change since it.
func TestCommitsBeforeTheLastTagAreIgnored(t *testing.T) {
	r := newRepo(t).commit("feat!: old break").commit("feat: old feature").tag("v0.2.0").commit("fix: only this")
	if got := r.next(); got != "v0.2.1" {
		t.Fatalf("got %q, want v0.2.1", got)
	}
}

func TestWithNoTagTheFirstVersionIsTheNewestChangelogSection(t *testing.T) {
	r := newRepo(t).commit("feat: first").changelog("## [0.1.0]\n\nFEATURES:\n\n* a\n")
	if got := r.next(); got != "v0.1.0" {
		t.Fatalf("got %q, want v0.1.0", got)
	}
	r.changelog("## [0.2.0]\n\n* b\n\n## [0.1.0]\n\n* a\n")
	if got := r.next(); got != "v0.2.0" {
		t.Fatalf("the newest section is first, got %q, want v0.2.0", got)
	}
	if got := newRepo(t).commit("feat: first").next(); got != "none" {
		t.Fatalf("no tag and no section: got %q, want none", got)
	}
}

// The changelog is the human gate: the version the commits call for needs its section.
func TestTheChangelogGateRequiresASectionForTheComputedVersion(t *testing.T) {
	r := newRepo(t).commit("chore: start").tag("v0.2.0").commit("feat: a feature").
		changelog("## [0.2.0]\n\n* old\n")
	next := r.next()
	if next != "v0.3.0" {
		t.Fatalf("computed %q, want v0.3.0", next)
	}
	if _, stderr, err := r.run("require-changelog", next); err == nil || !strings.Contains(stderr, "no ## [0.3.0] section") {
		t.Fatalf("a changelog without the section must fail and name it, got err=%v stderr=%q", err, stderr)
	}

	r.changelog("## [0.3.0]\n\n* new\n\n## [0.2.0]\n\n* old\n")
	if _, stderr, err := r.run("require-changelog", next); err != nil {
		t.Fatalf("a changelog with the section must pass: %v %s", err, stderr)
	}

	// Another version's section does not satisfy it, and neither does a section whose dots match anything.
	r.changelog("## [0.2.1]\n\n* x\n## [0X3X0]\n\n* y\n")
	if _, _, err := r.run("require-changelog", next); err == nil {
		t.Fatal("a section for another version, or a lookalike, must not satisfy the gate")
	}
}

// The rule the lead decided, as a pair a reader can see: the same history gives 0.3.0 with a
// feature and 0.2.1 with only fixes.
func TestOneFeatureIsTheDifferenceBetweenAMinorAndAPatch(t *testing.T) {
	base := func() *repo {
		return newRepo(t).commit("chore: start").tag("v0.2.0").commit("fix: a").commit("docs: b")
	}
	if got := base().next(); got != "v0.2.1" {
		t.Fatalf("fixes only: got %q, want v0.2.1", got)
	}
	if got := base().commit("feat: c").next(); got != "v0.3.0" {
		t.Fatalf("with a feature: got %q, want v0.3.0", got)
	}
}

func TestTheScriptRefusesAnUnknownCommand(t *testing.T) {
	if _, _, err := newRepo(t).run("tag-it"); err == nil {
		t.Fatal("an unknown command must fail")
	}
}
