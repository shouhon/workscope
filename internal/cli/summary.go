package cli

import "workscope/internal/baseline"

type summaryCounts struct {
	PreexistingChanges *int `json:"preexisting_changes"`
	ChangesSinceBegin  *int `json:"changes_since_begin"`
	IndexChanges       *int `json:"index_changes"`
	OutOfScopeChanges  *int `json:"out_of_scope_changes"`
	Issues             int  `json:"issues"`
}

// observedCounts は不完全な照合で検出できた件数であり、総件数の下限を表す。
type observedCounts struct {
	ChangesSinceBegin int `json:"changes_since_begin"`
	IndexChanges      int `json:"index_changes"`
	OutOfScopeChanges int `json:"out_of_scope_changes"`
}

type summaryResult struct {
	SchemaVersion  int               `json:"schema_version"`
	Tool           string            `json:"tool"`
	Command        string            `json:"command"`
	Status         string            `json:"status"`
	BaselineID     string            `json:"baseline_id"`
	Task           string            `json:"task,omitempty"`
	Root           string            `json:"root"`
	Coverage       baseline.Coverage `json:"coverage"`
	Counts         summaryCounts     `json:"counts"`
	ObservedCounts *observedCounts   `json:"observed_counts,omitempty"`
	Issues         []baseline.Issue  `json:"issues"`
}

func summarize(result baseline.Result) summaryResult {
	summary := summaryResult{
		SchemaVersion: result.SchemaVersion, Tool: result.Tool, Command: result.Command,
		Status: result.Status, BaselineID: result.BaselineID, Task: result.Task,
		Root: result.Root, Coverage: result.Coverage, Issues: result.Issues,
		Counts: summaryCounts{Issues: len(result.Issues)},
	}
	preexisting := len(result.PreexistingChanges)
	changes, index, outside := len(result.ChangesSinceBegin), len(result.IndexChanges), len(result.OutOfScopeChanges)
	if result.Coverage.Complete || result.BaselineID != "" {
		summary.Counts.PreexistingChanges = &preexisting
	}
	if result.Coverage.Complete {
		summary.Counts.ChangesSinceBegin = &changes
		summary.Counts.IndexChanges = &index
		summary.Counts.OutOfScopeChanges = &outside
	} else {
		// 未読や照合前の失敗を、変更がなかったことに置き換えない。
		summary.ObservedCounts = &observedCounts{changes, index, outside}
	}
	return summary
}
