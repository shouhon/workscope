// Package baseline はGitの作業領域を読み取り、開始記録を安全に保存する。
package baseline

const SchemaVersion = 1

type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

type Coverage struct {
	Included   []string `json:"included"`
	Excluded   []string `json:"excluded"`
	Submodules []string `json:"submodules"`
	Complete   bool     `json:"complete"`
}

func DefaultCoverage() Coverage {
	return Coverage{
		Included:   []string{"tracked_files", "untracked_nonignored_files", "symlink_targets", "index"},
		Excluded:   []string{"ignored_untracked_files", "git_metadata", ".workscope/", "submodule_contents"},
		Submodules: []string{}, Complete: true,
	}
}

type Allowed struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type Entry struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256"`
	Target string `json:"target,omitempty"`
}

type IndexEntry struct {
	Path   string `json:"path"`
	Mode   string `json:"mode"`
	Object string `json:"object"`
	Stage  int    `json:"stage"`
}

type GitChange struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type Record struct {
	SchemaVersion      int          `json:"schema_version"`
	Tool               string       `json:"tool"`
	ID                 string       `json:"baseline_id"`
	Root               string       `json:"root"`
	CreatedAt          string       `json:"created_at"`
	Head               string       `json:"head"`
	Allow              []Allowed    `json:"allow"`
	Coverage           Coverage     `json:"coverage"`
	PreexistingChanges []GitChange  `json:"preexisting_changes"`
	Files              []Entry      `json:"files"`
	Index              []IndexEntry `json:"index"`
}

type Result struct {
	SchemaVersion      int         `json:"schema_version"`
	Tool               string      `json:"tool"`
	Command            string      `json:"command"`
	Status             string      `json:"status"`
	BaselineID         string      `json:"baseline_id"`
	Root               string      `json:"root"`
	Coverage           Coverage    `json:"coverage"`
	PreexistingChanges []GitChange `json:"preexisting_changes"`
	ChangesSinceBegin  []any       `json:"changes_since_begin"`
	IndexChanges       []any       `json:"index_changes"`
	OutOfScopeChanges  []any       `json:"out_of_scope_changes"`
	Issues             []Issue     `json:"issues"`
}

func NewResult() Result {
	return Result{SchemaVersion: SchemaVersion, Tool: "workscope", Command: "begin", Status: "unknown",
		Coverage: DefaultCoverage(), PreexistingChanges: []GitChange{}, ChangesSinceBegin: []any{},
		IndexChanges: []any{}, OutOfScopeChanges: []any{}, Issues: []Issue{}}
}

type failure struct {
	issue   Issue
	invalid bool
}

func (f *failure) Error() string            { return f.issue.Message }
func fail(code, message, path string) error { return &failure{issue: Issue{code, message, path}} }
func invalid(message, path string) error {
	return &failure{issue: Issue{"invalid_allow", message, path}, invalid: true}
}
