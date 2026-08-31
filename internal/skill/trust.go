package skill

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TrustStore persists one-time per-project decisions about whether
// project-scope skill directories may be scanned, keyed by absolute
// project root path (ProjectRoot), at
// $XDG_STATE_HOME/liam/trusted-projects.json.
type TrustStore struct {
	path string
}

type trustRecord struct {
	Trusted   bool      `json:"trusted"`
	DecidedAt time.Time `json:"decidedAt"`
}

// OpenTrustStore resolves the trust store's file path. The file itself
// need not exist yet — it's created on the first Record call.
func OpenTrustStore() (*TrustStore, error) {
	base, err := xdgStateHome()
	if err != nil {
		return nil, fmt.Errorf("skill: locating state directory: %w", err)
	}
	return &TrustStore{path: filepath.Join(base, "liam", "trusted-projects.json")}, nil
}

func (s *TrustStore) load() (map[string]trustRecord, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]trustRecord{}, nil
		}
		return nil, fmt.Errorf("skill: reading trust store: %w", err)
	}
	var m map[string]trustRecord
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("skill: parsing trust store: %w", err)
	}
	return m, nil
}

// Decision returns root's persisted trust decision, if any. decided is
// false when no decision has been recorded yet.
func (s *TrustStore) Decision(root string) (trusted bool, decided bool, err error) {
	m, err := s.load()
	if err != nil {
		return false, false, err
	}
	rec, ok := m[root]
	return rec.Trusted, ok, nil
}

// Record persists a trust decision for root, creating the store's parent
// directory if needed.
func (s *TrustStore) Record(root string, trusted bool) error {
	m, err := s.load()
	if err != nil {
		return err
	}
	m[root] = trustRecord{Trusted: trusted, DecidedAt: time.Now().UTC()}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("skill: creating trust store directory: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("skill: encoding trust store: %w", err)
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
		return fmt.Errorf("skill: writing trust store: %w", err)
	}
	return nil
}

// Prompter asks a yes/no question and reports the answer — the
// interactive trust prompt's seam. Passing nil to ResolveProjectTrust
// means "can't prompt" (liam's headless mode, where blocking on stdin
// isn't appropriate).
type Prompter func(question string) (bool, error)

// TerminalPrompter builds a Prompter that writes question to out and
// reads a y/n answer from in — liam's actual interactive trust prompt,
// run once before the TUI starts.
func TerminalPrompter(in io.Reader, out io.Writer) Prompter {
	return func(question string) (bool, error) {
		fmt.Fprintf(out, "%s [y/N] ", question)
		line, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && err != io.EOF {
			return false, err
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		return answer == "y" || answer == "yes", nil
	}
}

// ResolveProjectTrust decides whether root's project-scope skill
// directories should be scanned, in order of precedence: a persisted
// decision from a prior run, then override (skills.trustProjectSkills
// config), then prompt (nil when running headlessly). Only an explicit
// decision (override or prompt answer) is persisted to store — a silent
// headless-mode default is not, so a later interactive run can still
// prompt. store may be nil (e.g. its location couldn't be resolved),
// which disables persistence but not the override/prompt/default logic.
func ResolveProjectTrust(store *TrustStore, root string, override *bool, prompt Prompter) (bool, error) {
	if store != nil {
		trusted, decided, err := store.Decision(root)
		if err != nil {
			return false, err
		}
		if decided {
			return trusted, nil
		}
	}

	if override != nil {
		if store != nil {
			if err := store.Record(root, *override); err != nil {
				return false, err
			}
		}
		return *override, nil
	}

	if prompt != nil {
		trusted, err := prompt(fmt.Sprintf("liam: trust project-level skills in %s?", root))
		if err != nil {
			return false, fmt.Errorf("skill: prompting for trust: %w", err)
		}
		if store != nil {
			if err := store.Record(root, trusted); err != nil {
				return false, err
			}
		}
		return trusted, nil
	}

	// Headless mode, no config override, no persisted decision: the safe
	// default is to not load untrusted project skills.
	return false, nil
}
