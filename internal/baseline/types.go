// Package baseline はGitの作業領域を読み取り、開始記録の保存と照合を行う。
package baseline

const SchemaVersion = 1

type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	Details string `json:"details,omitempty"`
}

// Options は今回の実行の指定。Gitの信頼設定は開始記録へ保存しない。
type Options struct {
	TrustRepository string
	Extend          bool
	Reason          string
	IncludeIgnored  []string
}

type Coverage struct {
	Included             []string `json:"included"`
	Excluded             []string `json:"excluded"`
	Submodules           []string `json:"submodules"`
	Complete             bool     `json:"complete"`
	IgnoredBaselinePaths []string `json:"ignored_baseline_paths,omitempty"`
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

// ScopeExtension は開始状態を変えずに追加した許容範囲と、その時点の変更を示す。
type ScopeExtension struct {
	AddedAt      string    `json:"added_at"`
	Reason       string    `json:"reason"`
	Allow        []Allowed `json:"allow"`
	ChangedPaths []string  `json:"changed_paths"`
}

type Record struct {
	SchemaVersion      int              `json:"schema_version"`
	Tool               string           `json:"tool"`
	ID                 string           `json:"baseline_id"`
	Task               string           `json:"task,omitempty"`
	Root               string           `json:"root"`
	CreatedAt          string           `json:"created_at"`
	Head               string           `json:"head"`
	Allow              []Allowed        `json:"allow"`
	InitialAllow       []Allowed        `json:"initial_allow,omitempty"`
	ScopeExtensions    []ScopeExtension `json:"scope_extensions,omitempty"`
	Coverage           Coverage         `json:"coverage"`
	PreexistingChanges []GitChange      `json:"preexisting_changes"`
	Files              []Entry          `json:"files"`
	Index              []IndexEntry     `json:"index"`
}

type Change struct {
	Path         string `json:"path"`
	Source       string `json:"source"`
	Kind         string `json:"kind"`
	BeforeKind   string `json:"before_kind"`
	AfterKind    string `json:"after_kind"`
	BeforeSHA256 string `json:"before_sha256"`
	AfterSHA256  string `json:"after_sha256"`
	WithinScope  bool   `json:"within_scope"`
}

type IndexChange struct {
	Path        string       `json:"path"`
	Source      string       `json:"source"`
	Kind        string       `json:"kind"`
	Before      []IndexEntry `json:"before"`
	After       []IndexEntry `json:"after"`
	WithinScope bool         `json:"within_scope"`
}

type Result struct {
	SchemaVersion      int              `json:"schema_version"`
	Tool               string           `json:"tool"`
	Command            string           `json:"command"`
	Status             string           `json:"status"`
	BaselineID         string           `json:"baseline_id"`
	Task               string           `json:"task,omitempty"`
	InitialAllow       []Allowed        `json:"initial_allow,omitempty"`
	ScopeExtensions    []ScopeExtension `json:"scope_extensions,omitempty"`
	Root               string           `json:"root"`
	Coverage           Coverage         `json:"coverage"`
	PreexistingChanges []GitChange      `json:"preexisting_changes"`
	ChangesSinceBegin  []Change         `json:"changes_since_begin"`
	IndexChanges       []IndexChange    `json:"index_changes"`
	OutOfScopeChanges  []any            `json:"out_of_scope_changes"`
	Issues             []Issue          `json:"issues"`
}

func NewResult() Result {
	return Result{SchemaVersion: SchemaVersion, Tool: "workscope", Command: "begin", Status: "unknown",
		Coverage: DefaultCoverage(), PreexistingChanges: []GitChange{}, ChangesSinceBegin: []Change{},
		IndexChanges: []IndexChange{}, OutOfScopeChanges: []any{}, Issues: []Issue{}}
}

type failure struct {
	issue   Issue
	invalid bool
}

func (f *failure) Error() string { return f.issue.Message }
func fail(code, message, path string) error {
	return &failure{issue: Issue{Code: code, Message: message, Path: path}}
}
func invalid(message, path string) error {
	return &failure{issue: Issue{Code: "invalid_allow", Message: message, Path: path}, invalid: true}
}
