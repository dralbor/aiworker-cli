// Package gitrepo is a thin wrapper over the system git binary. It never
// configures its own identity or credentials — every commit/push runs with
// whatever git.user.* and credential helper the machine already has, so
// commits land under the actual developer's own account, same as if they'd
// typed the git commands themselves.
package gitrepo

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo is a working directory git operates against.
type Repo struct {
	Dir string
}

// Open wraps an existing (or not-yet-a-repo) directory.
func Open(dir string) *Repo {
	return &Repo{Dir: dir}
}

// IsRepo reports whether Dir is already a git working copy.
func (r *Repo) IsRepo() bool {
	_, err := os.Stat(filepath.Join(r.Dir, ".git"))
	return err == nil
}

func (r *Repo) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// Clone clones remote straight into dir. dir must not already exist (or must
// be empty) - git clone refuses a non-empty target on purpose, and this
// wrapper doesn't second-guess that.
func Clone(remote, dir string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	cmd := exec.Command("git", "clone", remote, dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git clone %s: %w", remote, err)
	}
	return string(out), nil
}

// InitWithRemote turns an existing empty directory into a fresh repo with
// origin set to remote (no commits yet).
func (r *Repo) InitWithRemote(remote string) error {
	if out, err := r.run("init"); err != nil {
		return fmt.Errorf("git init: %w: %s", err, out)
	}
	if out, err := r.run("remote", "add", "origin", remote); err != nil {
		return fmt.Errorf("git remote add origin: %w: %s", err, out)
	}
	return nil
}

// RemoteURL returns origin's current URL.
func (r *Repo) RemoteURL() (string, error) {
	out, err := r.run("remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin: %w: %s", err, out)
	}
	return strings.TrimSpace(out), nil
}

// SetRemoteURL repoints origin at a different URL (e.g. moving a shared
// skills repo from a personal placeholder to a team org). Existing history
// is untouched; a subsequent fetch + fast-forward-merge only succeeds if the
// new remote's history is actually related to what's already local.
func (r *Repo) SetRemoteURL(url string) error {
	if out, err := r.run("remote", "set-url", "origin", url); err != nil {
		return fmt.Errorf("git remote set-url origin: %w: %s", err, out)
	}
	return nil
}

// CurrentBranch returns the checked-out branch name.
func (r *Repo) CurrentBranch() (string, error) {
	out, err := r.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w: %s", err, out)
	}
	return strings.TrimSpace(out), nil
}

// Fetch updates the remote-tracking refs without touching the working tree.
func (r *Repo) Fetch() error {
	if out, err := r.run("fetch", "--quiet", "origin"); err != nil {
		return fmt.Errorf("git fetch: %w: %s", err, out)
	}
	return nil
}

// FastForwardMerge merges origin/<current branch> only if it's a pure
// fast-forward - never rewrites or discards local commits.
func (r *Repo) FastForwardMerge() error {
	branch, err := r.CurrentBranch()
	if err != nil {
		return err
	}
	out, err := r.run("merge", "--ff-only", "origin/"+branch)
	if err != nil {
		return fmt.Errorf("git merge --ff-only: %w: %s", err, out)
	}
	return nil
}

// AddCommitPush stages paths (relative to Dir), commits with message, and
// pushes to origin/<current branch>. If the push is rejected because the
// remote moved forward, it fetches and fast-forward-merges once and retries
// - never force-pushes. The local commit is left in place even if the push
// ultimately fails, so nothing is lost.
func (r *Repo) AddCommitPush(paths []string, message string) (output string, err error) {
	var log strings.Builder

	addArgs := append([]string{"add", "--"}, paths...)
	out, err := r.run(addArgs...)
	log.WriteString(out)
	if err != nil {
		return log.String(), fmt.Errorf("git add: %w", err)
	}

	out, err = r.run("commit", "-m", message)
	log.WriteString(out)
	if err != nil {
		return log.String(), fmt.Errorf("git commit: %w", err)
	}

	branch, err := r.CurrentBranch()
	if err != nil {
		return log.String(), err
	}

	out, err = r.run("push", "origin", "HEAD:"+branch)
	log.WriteString(out)
	if err == nil {
		return log.String(), nil
	}

	// Someone else pushed in the meantime: fetch + fast-forward + retry once.
	if fetchErr := r.Fetch(); fetchErr == nil {
		if mergeErr := r.FastForwardMerge(); mergeErr == nil {
			out, err = r.run("push", "origin", "HEAD:"+branch)
			log.WriteString(out)
			if err == nil {
				return log.String(), nil
			}
		}
	}

	return log.String(), fmt.Errorf("git push: %w (el commit local se hizo igual, no se perdio nada)", err)
}
