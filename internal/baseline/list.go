package baseline

import (
	"errors"
	"sort"
)

// RecordSummary は記録の選択に使う情報であり、現在の照合結果を表さない。
type RecordSummary struct {
	Task       string `json:"task"`
	Default    bool   `json:"default"`
	BaselineID string `json:"baseline_id"`
	CreatedAt  string `json:"created_at"`
	AllowCount int    `json:"allow_count"`
}

type ListResult struct {
	SchemaVersion int             `json:"schema_version"`
	Tool          string          `json:"tool"`
	Command       string          `json:"command"`
	Status        string          `json:"status"`
	Root          string          `json:"root"`
	Records       []RecordSummary `json:"records"`
	Issues        []Issue         `json:"issues"`
}

// List は開始記録だけを読み取り、現在のHEADやファイルとの照合は行わない。
func List(dir string, options Options) (ListResult, int) {
	result := ListResult{SchemaVersion: SchemaVersion, Tool: "workscope", Command: "list", Status: "unknown", Records: []RecordSummary{}, Issues: []Issue{}}
	err := func() error {
		r, err := openRepositoryWithOptions(dir, options)
		if err != nil {
			return err
		}
		defer r.fs.Close()
		result.Root = r.root
		_, records, err := r.existingRecords("")
		if err != nil {
			return err
		}
		sort.Slice(records, func(i, j int) bool { return records[i].Task < records[j].Task })
		result.Records = records
		result.Status = "listed"
		return nil
	}()
	if err == nil {
		return result, 0
	}
	result.Issues = append(result.Issues, describeError(err))
	var f *failure
	if errors.As(err, &f) && f.invalid {
		result.Status = "invalid_arguments"
		return result, 64
	}
	return result, 2
}
