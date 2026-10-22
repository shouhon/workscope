package baseline

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
)

// Check は開始記録と現在の状態を照合する。対象と開始記録には書き込まない。
func Check(dir string) (Result, int) {
	result := NewResult()
	result.Command = "check"
	err := func() error {
		r, err := openRepository(dir)
		if err != nil {
			return err
		}
		defer r.fs.Close()
		result.Root = r.root
		data, err := r.existing()
		if err != nil {
			return err
		}
		if data == nil {
			return fail("baseline_missing", "開始記録がありません。先に begin --allow <対象> を実行してください。", stateName+"/"+recordName)
		}
		var record Record
		if err := json.Unmarshal(data, &record); err != nil {
			return fail("baseline_unreadable", "開始記録を解析できません。", stateName+"/"+recordName)
		}
		result.BaselineID = record.ID
		result.PreexistingChanges = record.PreexistingChanges
		current, err := r.stableSnapshot(record.Files)
		if current.Submodules != nil {
			result.Coverage.Submodules = current.Submodules
		}
		if err != nil {
			return err
		}
		result.Issues = append(result.Issues, current.Issues...)
		if current.Head != record.Head {
			result.Issues = append(result.Issues, Issue{Code: "head_changed", Message: "開始時からHEADが変わったため、完全な照合はできません。"})
		}
		result.compare(record, current)
		confirmed, err := r.existing()
		if err != nil || !bytes.Equal(data, confirmed) {
			return fail("baseline_changed", "照合中に開始記録が変化したか、読み取れなくなりました。再実行してください。", stateName+"/"+recordName)
		}
		return nil
	}()
	if err != nil {
		result.Issues = append(result.Issues, describeError(err))
	}
	result.Issues = sortedIssues(result.Issues)
	if len(result.Issues) != 0 {
		result.Coverage.Complete = false
		return result, 2
	}
	if len(result.OutOfScopeChanges) != 0 {
		result.Status = "out_of_scope"
		return result, 1
	}
	result.Status = "within_scope"
	return result, 0
}

func sortedIssues(issues []Issue) []Issue {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Path != issues[j].Path {
			return issues[i].Path < issues[j].Path
		}
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Message < issues[j].Message
	})
	unique := []Issue{}
	for _, issue := range issues {
		if len(unique) == 0 || unique[len(unique)-1] != issue {
			unique = append(unique, issue)
		}
	}
	return unique
}

func withinScope(name string, allowed []Allowed) bool {
	for _, allow := range allowed {
		if samePath(name, allow.Path) || (allow.Kind != "file" && beneath(name, allow.Path)) {
			return true
		}
	}
	return false
}

func (result *Result) compare(record Record, current snapshot) {
	before := map[string]Entry{}
	after := map[string]Entry{}
	names := map[string]string{}
	for _, entry := range record.Files {
		key := pathKey(entry.Path)
		before[key], names[key] = entry, entry.Path
	}
	for _, entry := range current.Files {
		key := pathKey(entry.Path)
		after[key], names[key] = entry, entry.Path
	}
	for key, name := range names {
		unknown := false
		for _, issue := range current.Issues {
			// 親リンクの配下を読めなくても、安全に取得したリンク自体は比較できる。
			if issue.Code == "unsafe_parent" && samePath(name, issue.Path) && after[key].Kind != "" {
				continue
			}
			if issue.Path == "" || beneath(name, issue.Path) {
				unknown = true
				break
			}
		}
		if unknown {
			continue
		}
		old, now := before[key], after[key]
		if old.Kind == "" {
			old.Kind = "missing"
		}
		if now.Kind == "" {
			now.Kind = "missing"
		}
		if old.Kind == now.Kind && old.SHA256 == now.SHA256 {
			continue
		}
		kind := "modified"
		switch {
		case old.Kind == "missing":
			kind = "added"
		case now.Kind == "missing":
			kind = "deleted"
		case old.Kind != now.Kind:
			kind = "type_changed"
		}
		change := Change{name, "worktree", kind, old.Kind, now.Kind, old.SHA256, now.SHA256, withinScope(name, record.Allow)}
		result.ChangesSinceBegin = append(result.ChangesSinceBegin, change)
	}
	sort.Slice(result.ChangesSinceBegin, func(i, j int) bool { return result.ChangesSinceBegin[i].Path < result.ChangesSinceBegin[j].Path })
	for _, change := range result.ChangesSinceBegin {
		if !change.WithinScope {
			result.OutOfScopeChanges = append(result.OutOfScopeChanges, change)
		}
	}
	oldIndex, newIndex := groupIndex(record.Index), groupIndex(current.Index)
	indexNames := map[string]string{}
	for key, entries := range oldIndex {
		indexNames[key] = entries[0].Path
	}
	for key, entries := range newIndex {
		indexNames[key] = entries[0].Path
	}
	for key, name := range indexNames {
		old, now := oldIndex[key], newIndex[key]
		if reflect.DeepEqual(old, now) {
			continue
		}
		kind := "modified"
		if old == nil {
			old, kind = []IndexEntry{}, "added"
		}
		if now == nil {
			now, kind = []IndexEntry{}, "deleted"
		}
		result.IndexChanges = append(result.IndexChanges, IndexChange{name, "index", kind, old, now, withinScope(name, record.Allow)})
	}
	sort.Slice(result.IndexChanges, func(i, j int) bool { return result.IndexChanges[i].Path < result.IndexChanges[j].Path })
	for _, change := range result.IndexChanges {
		if !change.WithinScope {
			result.OutOfScopeChanges = append(result.OutOfScopeChanges, change)
		}
	}
	// 範囲外はパス順、同じパスでは作業ファイル、インデックスの順にそろえる。
	sort.SliceStable(result.OutOfScopeChanges, func(i, j int) bool {
		path := func(change any) string {
			if value, ok := change.(Change); ok {
				return value.Path
			}
			return change.(IndexChange).Path
		}
		return path(result.OutOfScopeChanges[i]) < path(result.OutOfScopeChanges[j])
	})
}

func groupIndex(entries []IndexEntry) map[string][]IndexEntry {
	groups := map[string][]IndexEntry{}
	for _, entry := range entries {
		key := pathKey(entry.Path)
		groups[key] = append(groups[key], entry)
	}
	return groups
}
