package baseline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestComparisonOrdersMixedOutOfScopeChangesInJSON(t *testing.T) {
	hash := strings.Repeat("a", 64)
	object := strings.Repeat("b", 40)
	indexOnlyFile := Entry{Path: "a.txt", Kind: "file", SHA256: hash}
	record := Record{
		Allow: []Allowed{{Path: "allowed", Kind: "directory"}},
		Files: []Entry{indexOnlyFile},
	}
	current := snapshot{
		Files: []Entry{
			{Path: "z.txt", Kind: "file", SHA256: hash},
			{Path: "m.txt", Kind: "file", SHA256: hash},
			{Path: "allowed/new.txt", Kind: "file", SHA256: hash},
			indexOnlyFile,
		},
		Index: []IndexEntry{
			{Path: "m.txt", Mode: "100644", Object: object},
			{Path: "allowed/new.txt", Mode: "100644", Object: object},
			{Path: "a.txt", Mode: "100644", Object: object},
		},
	}
	result := NewResult()
	result.compare(record, current)

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	type changeLocation struct {
		Path   string `json:"path"`
		Source string `json:"source"`
	}
	var output struct {
		OutOfScope []changeLocation `json:"out_of_scope_changes"`
	}
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	want := []changeLocation{
		{Path: "a.txt", Source: "index"},
		{Path: "m.txt", Source: "worktree"},
		{Path: "m.txt", Source: "index"},
		{Path: "z.txt", Source: "worktree"},
	}
	if !reflect.DeepEqual(output.OutOfScope, want) {
		t.Fatalf("JSONの範囲外一覧: 実際=%+v、期待=%+v", output.OutOfScope, want)
	}
}

func TestComparisonDetectsIndexMetadataChangesWithIdenticalContent(t *testing.T) {
	object := strings.Repeat("a", 40)
	file := Entry{Path: "tracked.txt", Kind: "file", SHA256: strings.Repeat("b", 64)}
	for _, tc := range []struct {
		name   string
		before IndexEntry
		after  IndexEntry
	}{
		{
			name:   "実行ビットのみ",
			before: IndexEntry{Path: file.Path, Mode: "100644", Object: object},
			after:  IndexEntry{Path: file.Path, Mode: "100755", Object: object},
		},
		{
			name:   "競合段階のみ",
			before: IndexEntry{Path: file.Path, Mode: "100644", Object: object, Stage: 2},
			after:  IndexEntry{Path: file.Path, Mode: "100644", Object: object, Stage: 3},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := Record{
				Allow: []Allowed{{Path: "allowed", Kind: "future"}},
				Files: []Entry{file},
				Index: []IndexEntry{tc.before},
			}
			current := snapshot{Files: []Entry{file}, Index: []IndexEntry{tc.after}}
			result := NewResult()
			result.compare(record, current)

			if len(result.ChangesSinceBegin) != 0 {
				t.Fatalf("作業ファイルに差分が出ました: %+v", result.ChangesSinceBegin)
			}
			want := IndexChange{
				Path: file.Path, Source: "index", Kind: "modified",
				Before: []IndexEntry{tc.before}, After: []IndexEntry{tc.after},
				WithinScope: false,
			}
			if !reflect.DeepEqual(result.IndexChanges, []IndexChange{want}) ||
				!reflect.DeepEqual(result.OutOfScopeChanges, []any{want}) {
				t.Fatalf("インデックス差分: 実際=%+v、範囲外=%+v、期待=%+v", result.IndexChanges, result.OutOfScopeChanges, want)
			}
		})
	}
}
