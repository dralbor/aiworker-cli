// Package skillsmarket turns the local skills folder into a shared "skills
// marketplace": when a remote is configured, the actual git working copy
// lives at ~/.aiworker-cli/skills-repo (the *whole* shared repo - it also
// holds aiworker-cli's own source code, same repo, different top-level
// folder) and ~/.claude/skills is a directory link (junction on Windows,
// symlink elsewhere) into that repo's skills/ subfolder. Claude Code only
// ever sees ~/.claude/skills, same as always; writes through that path land
// straight inside the git working copy because the link is transparent.
//
// New skills/categories are committed and pushed under the developer's own
// git identity right after being written locally, and every entry into the
// Skills screen fetches + fast-forward-merges from the remote first - no
// cooldown, no "stay local for N minutes": the team wants this always
// connected and as close to real-time as a pull-based model gets. It's
// still never a blocking wait for the person creating something: the local
// write lands and shows up in the list instantly, the push (and the next
// person's pull) happen in the background.
//
// Everything here is best-effort on top of a plain local skills folder: with
// no remote configured, every function is a no-op and skills behave exactly
// like local-only files.
package skillsmarket

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"aiworker/cli/internal/gitrepo"
	"aiworker/cli/internal/state"
)

// skillsSubdir is where shared skills content lives inside the repo,
// alongside aiworker-cli's own source code at the repo root.
const skillsSubdir = "skills"

// NeedsRemotePrompt reports whether we should ask the user, once, for a
// shared skills repo URL (empty answer is remembered as "stay local-only").
func NeedsRemotePrompt() (bool, error) {
	st, err := state.Load()
	if err != nil {
		return false, err
	}
	return !st.SkillsRemoteAsked, nil
}

// SetRemote records the user's answer to the one-time remote prompt. An
// empty remote means "local-only, don't ask again".
func SetRemote(remote string) error {
	st, err := state.Load()
	if err != nil {
		return err
	}
	st.SkillsRemote = remote
	st.SkillsRemoteAsked = true
	return st.Save()
}

// Remote returns the configured shared-skills remote, or "" if running
// local-only.
func Remote() (string, error) {
	st, err := state.Load()
	if err != nil {
		return "", err
	}
	return st.SkillsRemote, nil
}

// RepoDir is where the actual git working copy of the shared repo lives -
// not ~/.claude/skills itself (that's just a link into RepoDir()/skills).
// Kept out of ~/.claude so Claude Code's own skill scan never walks into
// the rest of the repo (source code, .git, ...).
func RepoDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".aiworker-cli", "skills-repo"), nil
}

// NeedsBootstrap reports whether the repo needs cloning before use: a
// remote is configured, but RepoDir isn't a git repo yet.
func NeedsBootstrap(root string) (bool, error) {
	remote, err := Remote()
	if err != nil {
		return false, err
	}
	if remote == "" {
		return false, nil
	}
	repoDir, err := RepoDir()
	if err != nil {
		return false, err
	}
	return !gitrepo.Open(repoDir).IsRepo(), nil
}

// Bootstrap clones the shared repo (whole repo - code and skills/ both) into
// RepoDir if it isn't there yet, then links root (~/.claude/skills) into its
// skills/ subfolder. Only ever takes the fully-safe path automatically: a
// missing/empty RepoDir gets cloned straight from the remote, and root only
// gets linked if it's missing, empty, or already the same link - anything
// else (real pre-existing content that isn't ours) is left untouched, with a
// clear error instead of a guess.
func Bootstrap(root string) (bootstrapped bool, err error) {
	remote, err := Remote()
	if err != nil {
		return false, err
	}
	if remote == "" {
		return false, nil
	}

	repoDir, err := RepoDir()
	if err != nil {
		return false, err
	}

	repo := gitrepo.Open(repoDir)
	clonedNow := false
	if !repo.IsRepo() {
		entries, statErr := os.ReadDir(repoDir)
		if statErr != nil && !os.IsNotExist(statErr) {
			return false, statErr
		}
		if len(entries) > 0 {
			return false, fmt.Errorf(
				"%s ya tiene contenido y no es un repo git todavia - iniciarlo a mano: "+
					"'git init', 'git remote add origin %s', reconciliar con el remoto, y volver a intentar",
				repoDir, remote)
		}
		if err := os.RemoveAll(repoDir); err != nil {
			return false, err
		}
		if _, err := gitrepo.Clone(remote, repoDir); err != nil {
			return false, err
		}
		clonedNow = true
	}

	skillsDir := filepath.Join(repoDir, skillsSubdir)
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return clonedNow, fmt.Errorf("creando %s: %w", skillsDir, err)
	}
	if err := ensureLink(root, skillsDir); err != nil {
		return clonedNow, err
	}
	return clonedNow, nil
}

// ensureLink makes link (~/.claude/skills) resolve to target
// (RepoDir()/skills): a directory junction on Windows (no admin needed,
// unlike symlinks), a plain symlink elsewhere. Refuses to touch a link path
// that already has real content of its own instead of guessing how to merge
// it.
func ensureLink(link, target string) error {
	entries, err := os.ReadDir(link)
	switch {
	case os.IsNotExist(err):
		return createDirLink(link, target)
	case err != nil:
		return err
	case len(entries) == 0:
		// Empty real directory (not yet a link) - safe to replace.
		if err := os.Remove(link); err != nil {
			return fmt.Errorf("removiendo %s antes de enlazarlo: %w", link, err)
		}
		return createDirLink(link, target)
	default:
		same, err := sameDir(link, target)
		if err != nil {
			return err
		}
		if same {
			return nil // already linked correctly
		}
		return fmt.Errorf(
			"%s ya tiene contenido que no es del repo compartido - moverlo a mano antes de conectar Skills (se esperaba un link a %s)",
			link, target)
	}
}

func sameDir(a, b string) (bool, error) {
	ai, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(ai, bi), nil
}

func createDirLink(link, target string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if err != nil {
			return fmt.Errorf("mklink /J %s -> %s: %w: %s", link, target, err, out)
		}
		return nil
	}
	return os.Symlink(target, link)
}

// Prepare makes sure root (~/.claude/skills) reflects the shared remote as
// needed: clones + links on first use (see Bootstrap), repoints origin if
// the configured remote changed since last time (e.g. `aiworker skills
// set-remote` pointed at a different repo), then always pulls latest (see
// Sync) - no throttling. No-op running local-only.
func Prepare(root string) error {
	needsBootstrap, err := NeedsBootstrap(root)
	if err != nil {
		return err
	}
	if needsBootstrap {
		_, err := Bootstrap(root)
		return err
	}
	// Not a fresh clone, but the link might still be missing/wrong (e.g.
	// someone deleted ~/.claude/skills by hand) - Bootstrap's linking step
	// is idempotent, so re-run it too.
	if _, err := Bootstrap(root); err != nil {
		return err
	}
	if err := reconcileRemote(); err != nil {
		return err
	}
	return Sync(root)
}

// reconcileRemote repoints RepoDir's git "origin" at the currently
// configured remote if it doesn't match already. A no-op in the common case
// (nothing changed).
func reconcileRemote() error {
	remote, err := Remote()
	if err != nil || remote == "" {
		return err
	}
	repoDir, err := RepoDir()
	if err != nil {
		return err
	}
	repo := gitrepo.Open(repoDir)
	current, err := repo.RemoteURL()
	if err != nil {
		return err
	}
	if current == remote {
		return nil
	}
	return repo.SetRemoteURL(remote)
}

// Sync fetches and fast-forward-merges from origin, then records the sync
// time. A no-op (nil error) if no remote is configured or the repo isn't
// cloned yet. Network/merge failures are returned as-is so the caller can
// fall back to showing the local copy without blocking on them.
func Sync(root string) error {
	remote, err := Remote()
	if err != nil {
		return err
	}
	if remote == "" {
		return nil
	}
	repoDir, err := RepoDir()
	if err != nil {
		return err
	}
	repo := gitrepo.Open(repoDir)
	if !repo.IsRepo() {
		return nil
	}
	if err := repo.Fetch(); err != nil {
		return err
	}
	if err := repo.FastForwardMerge(); err != nil {
		return err
	}

	st, err := state.Load()
	if err != nil {
		return err
	}
	st.SkillsLastSync = time.Now()
	return st.Save()
}

// ErrLocalOnly is returned by Publish when no remote is configured - not a
// failure, just "nothing to push".
var ErrLocalOnly = errors.New("skills: sin remoto configurado, guardado solo local")

// Publish commits path (a path under root, i.e. under ~/.claude/skills) and
// pushes it under the caller's own git identity. Returns ErrLocalOnly (not a
// real error) when running without a configured remote. Internally this
// operates on RepoDir (the actual git repo) with a path rewritten to
// RepoDir()/skills/... - callers only ever deal in ~/.claude/skills paths.
func Publish(root, path, message string) (output string, err error) {
	remote, err := Remote()
	if err != nil {
		return "", err
	}
	if remote == "" {
		return "", ErrLocalOnly
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("skills: %s no esta bajo %s: %w", path, root, err)
	}

	repoDir, err := RepoDir()
	if err != nil {
		return "", err
	}
	repo := gitrepo.Open(repoDir)
	if !repo.IsRepo() {
		return "", fmt.Errorf("skills: %s no es un repo git (falto el bootstrap inicial)", repoDir)
	}
	return repo.AddCommitPush([]string{filepath.Join(skillsSubdir, rel)}, message)
}
