package baseline

import (
	"encoding/json"
	"reflect"
	"sort"
	"time"
)

func (r *repository) extend(names []string, proposed []Allowed, task, reason string, previous stateSnapshot, result *Result) error {
	if previous.data == nil {
		return fail("baseline_missing", "許容範囲を追加する開始記録がありません。同じタスク名で編集前に begin を実行してください。", stateName+"/"+RecordName(task))
	}
	var record Record
	if err := json.Unmarshal(previous.data, &record); err != nil {
		return fail("baseline_unreadable", "開始記録を解析できません。", stateName+"/"+RecordName(task))
	}
	result.BaselineID = record.ID
	result.PreexistingChanges = record.PreexistingChanges
	result.InitialAllow = record.InitialAllow
	result.ScopeExtensions = record.ScopeExtensions
	added := []Allowed{}
	for _, allow := range proposed {
		if !withinScope(allow.Path, record.Allow) && !withinScope(allow.Path, added) {
			added = append(added, allow)
		}
	}
	if len(added) == 0 {
		return invalid("指定したパスはすでに許容範囲内です。追加する範囲を指定してください。", "")
	}
	current, err := r.stableSnapshot(record.Files)
	if current.Submodules != nil {
		result.Coverage.Submodules = current.Submodules
	}
	if err != nil {
		return err
	}
	if len(current.Issues) != 0 {
		return &failure{issue: current.Issues[0]}
	}
	if current.Head != record.Head {
		return fail("head_changed", "開始時からHEADが変わったため許容範囲を追加できません。元の開始記録は保持します。", "")
	}
	r.progress.stage("許容範囲の再確認")
	confirmed, err := r.normalizeAllow(names)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(proposed, confirmed) {
		return fail("snapshot_changed", "追加中に許容対象の種類や実体が変化しました。", "")
	}
	// 取得した状態は追加時点の説明だけに使い、最初のファイル・インデックスを更新しない。
	observed := NewResult()
	observed.compare(record, current)
	changed := map[string]bool{}
	remember := func(name string) {
		if withinScope(name, added) && !withinScope(name, record.Allow) {
			changed[name] = true
		}
	}
	for _, change := range observed.ChangesSinceBegin {
		remember(change.Path)
	}
	for _, change := range observed.IndexChanges {
		remember(change.Path)
	}
	for _, change := range current.Changes {
		remember(change.Path)
	}
	paths := []string{}
	for name := range changed {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	if record.InitialAllow == nil {
		record.InitialAllow = append([]Allowed{}, record.Allow...)
	}
	record.ScopeExtensions = append(record.ScopeExtensions, ScopeExtension{
		AddedAt: time.Now().UTC().Format(time.RFC3339Nano), Reason: reason, Allow: added, ChangedPaths: paths,
	})
	record.Allow = append(record.Allow, added...)
	sort.Slice(record.Allow, func(i, j int) bool { return record.Allow[i].Path < record.Allow[j].Path })
	if !r.validRecord(record) {
		return fail("invalid_scope_history", "追加履歴の整合性を確認できません。時計と開始記録を確認してください。元の開始記録は保持します。", "")
	}
	r.progress.stage("開始記録の保存")
	if err := r.save(record, previous); err != nil {
		return err
	}
	result.InitialAllow = record.InitialAllow
	result.ScopeExtensions = record.ScopeExtensions
	result.compare(record, current)
	result.Status = "extended"
	return nil
}
