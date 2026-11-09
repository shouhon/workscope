package baseline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func extensionRepo(t *testing.T) string {
	t.Helper()
	dir := repo(t)
	write(t, dir, ".gitignore", "/.workscope/\n")
	write(t, dir, "second.txt", "追加対象の開始時の本文")
	write(t, dir, "outside.txt", "範囲外の開始時の本文")
	testGit(t, dir, "add", ".")
	commit(t, dir)
	return dir
}

func extend(t *testing.T, dir, task string, names ...string) Result {
	t.Helper()
	result, code := BeginWithOptions(dir, names, false, task, Options{Extend: true, Reason: "追加依頼への対応"})
	if code != 0 || result.Status != "extended" || !result.Coverage.Complete {
		t.Fatalf("範囲追加に失敗: %d %+v", code, result)
	}
	return result
}

func TestScopeExtensionPreservesOriginalStateAndOtherTasks(t *testing.T) {
	for _, task := range []string{"", "日本語 作業"} {
		t.Run(task, func(t *testing.T) {
			dir := extensionRepo(t)
			write(t, dir, "tracked.txt", "開始前の編集")
			if result, code := Begin(dir, []string{"tracked.txt"}, false, task); code != 0 {
				t.Fatalf("準備失敗: %d %+v", code, result)
			}
			var original Record
			if err := json.Unmarshal(taskData(t, dir, task), &original); err != nil {
				t.Fatal(err)
			}
			if result, code := Begin(dir, []string{"."}, false, "別の作業"); code != 0 {
				t.Fatalf("別タスクの準備失敗: %d %+v", code, result)
			}
			other := taskData(t, dir, "別の作業")
			write(t, dir, "tracked.txt", "開始後の追加編集")
			protected := map[string][]byte{}
			for _, name := range []string{".gitignore", ".git/index", ".git/HEAD", ".git/config", "tracked.txt", "second.txt", "outside.txt"} {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil {
					t.Fatal(err)
				}
				protected[name] = data
			}
			first := extend(t, dir, task, "second.txt")
			if first.BaselineID != original.ID || len(first.ScopeExtensions) != 1 || len(first.ScopeExtensions[0].ChangedPaths) != 0 {
				t.Fatalf("開始点か未編集対象の履歴が不正: %+v", first)
			}
			for name, before := range protected {
				after, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("対象かGit状態を変更: %s %v", name, err)
				}
			}
			write(t, dir, "second.txt", "追加後の編集")
			write(t, dir, "outside.txt", "許容しない編集")
			write(t, dir, "new.txt", "追加の前から存在する編集")
			second := extend(t, dir, task, "new.txt", "second.txt")
			if len(second.ScopeExtensions) != 2 || !reflect.DeepEqual(second.ScopeExtensions[1].ChangedPaths, []string{"new.txt"}) {
				t.Fatalf("追加前の編集を記録していません: %+v", second)
			}
			var saved Record
			if err := json.Unmarshal(taskData(t, dir, task), &saved); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(saved.InitialAllow, original.Allow) {
				t.Fatalf("当初の範囲を失いました: %+v", saved)
			}
			saved.Allow, saved.InitialAllow, saved.ScopeExtensions = original.Allow, nil, nil
			if !reflect.DeepEqual(saved, original) || !bytes.Equal(other, taskData(t, dir, "別の作業")) {
				t.Fatal("元の開始情報か別タスクを書き換えました")
			}
			before := taskData(t, dir, task)
			checked, code := Check(dir, task)
			if code != 1 || checked.BaselineID != original.ID || len(checked.ChangesSinceBegin) != 4 || len(checked.PreexistingChanges) != 1 || len(checked.ScopeExtensions) != 2 {
				t.Fatalf("開始時からの照合が不正: %d %+v", code, checked)
			}
			if len(checked.OutOfScopeChanges) != 1 || checked.OutOfScopeChanges[0].(Change).Path != "outside.txt" {
				t.Fatalf("無関係な変更を範囲内にしました: %+v", checked)
			}
			listed, code := List(dir, Options{})
			if code != 0 || len(listed.Records) != 2 {
				t.Fatalf("追加後の記録を一覧できません: %d %+v", code, listed)
			}
			if !bytes.Equal(before, taskData(t, dir, task)) {
				t.Fatal("checkかlistが追加後の記録を書き換えました")
			}
			replaced, code := Begin(dir, []string{"tracked.txt"}, true, task)
			if code != 0 || replaced.BaselineID == original.ID || len(replaced.ScopeExtensions) != 0 {
				t.Fatalf("明示した取り直しが不正: %d %+v", code, replaced)
			}
		})
	}
}

func TestScopeExtensionReportsChangesAlreadyPresent(t *testing.T) {
	for _, kind := range []string{"worktree", "index", "preexisting", "deleted"} {
		t.Run(kind, func(t *testing.T) {
			dir := extensionRepo(t)
			if kind == "preexisting" {
				write(t, dir, "second.txt", "開始前からの編集")
			}
			if result, code := Begin(dir, []string{"tracked.txt"}, false, ""); code != 0 {
				t.Fatalf("準備失敗: %d %+v", code, result)
			}
			switch kind {
			case "worktree":
				write(t, dir, "second.txt", "追加前の編集")
			case "index":
				write(t, dir, "second.txt", "ステージにだけ残す編集")
				testGit(t, dir, "add", "second.txt")
				write(t, dir, "second.txt", "追加対象の開始時の本文")
			case "deleted":
				if err := os.Remove(filepath.Join(dir, "second.txt")); err != nil {
					t.Fatal(err)
				}
			}
			result := extend(t, dir, "", "second.txt")
			if !reflect.DeepEqual(result.ScopeExtensions[0].ChangedPaths, []string{"second.txt"}) {
				t.Fatalf("追加時点の変更を隠しました: %+v", result)
			}
			checked, code := Check(dir, "")
			if code != 0 || kind == "index" && len(checked.IndexChanges) != 1 {
				t.Fatalf("追加後の照合失敗: %d %+v", code, checked)
			}
		})
	}
}

func TestScopeExtensionKeepsDirectoryBoundariesAndOriginalAllowKinds(t *testing.T) {
	dir := extensionRepo(t)
	write(t, dir, "src/item.txt", "開始点")
	write(t, dir, "src2/item.txt", "開始点")
	if result, code := Begin(dir, []string{"tracked.txt"}, false, ""); code != 0 {
		t.Fatalf("準備失敗: %d %+v", code, result)
	}
	if err := os.Remove(filepath.Join(dir, "tracked.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "tracked.txt/child.txt", "元はファイルだった対象の配下")
	extend(t, dir, "", "src", "future", "src/item.txt")
	write(t, dir, "src/item.txt", "追加後の範囲内")
	write(t, dir, "src2/item.txt", "似た名前の範囲外")
	write(t, dir, "future/child.txt", "未作成だった対象の配下")
	result, code := Check(dir, "")
	if code != 1 || len(result.OutOfScopeChanges) != 2 {
		t.Fatalf("境界を保って照合できません: %d %+v", code, result)
	}
	for _, change := range result.OutOfScopeChanges {
		name := change.(Change).Path
		if name != "src2/item.txt" && name != "tracked.txt/child.txt" {
			t.Fatalf("範囲内の変更を誤判定: %+v", result)
		}
	}
	if len(result.ScopeExtensions[0].Allow) != 2 || result.InitialAllow[0].Kind != "file" {
		t.Fatalf("重複した配下か元の種類の扱いが不正: %+v", result)
	}
}

func TestScopeExtensionRejectsFailuresWithoutChangingRecord(t *testing.T) {
	for _, kind := range []string{"missing", "broken", "other_root", "unsupported", "head", "locked", "concurrent", "directory_changed", "duplicate", "protected", "outside", "no_reason", "replace", "reason_without_extend"} {
		t.Run(kind, func(t *testing.T) {
			dir := extensionRepo(t)
			if kind == "missing" {
				result, code := BeginWithOptions(dir, []string{"second.txt"}, false, "", Options{Extend: true, Reason: "追加"})
				requireFailure(t, result, code, "baseline_missing")
				if _, err := os.Lstat(filepath.Join(dir, stateName)); !os.IsNotExist(err) {
					t.Fatalf("記録なしで保存先を作成: %v", err)
				}
				return
			}
			if result, code := Begin(dir, []string{"tracked.txt"}, false, ""); code != 0 {
				t.Fatalf("準備失敗: %d %+v", code, result)
			}
			names, options, replace := []string{"second.txt"}, Options{Extend: true, Reason: "追加"}, false
			want := "foreign_state"
			switch kind {
			case "broken":
				write(t, dir, stateName+"/"+recordName, "{")
			case "other_root", "unsupported":
				record, _ := readRecord(t, dir)
				if kind == "other_root" {
					record.Root = t.TempDir()
				} else {
					record.SchemaVersion = 999
				}
				data, _ := json.Marshal(record)
				write(t, dir, stateName+"/"+recordName, string(data))
			case "head":
				write(t, dir, "tracked.txt", "新しいコミット")
				testGit(t, dir, "add", "tracked.txt")
				commit(t, dir)
				want = "head_changed"
			case "locked":
				write(t, dir, stateName+"/record.lock", "他の処理")
			case "concurrent", "directory_changed":
				want = "state_changed"
			case "duplicate":
				names, want = []string{"tracked.txt"}, "invalid_allow"
			case "protected":
				names, want = []string{".git/config"}, "invalid_allow"
			case "outside":
				names, want = []string{"../outside"}, "invalid_allow"
			case "no_reason":
				options.Reason, want = "", "invalid_arguments"
			case "replace":
				replace, want = true, "invalid_arguments"
			case "reason_without_extend":
				options.Extend, want = false, "invalid_arguments"
			}
			before := taskData(t, dir, "")
			changed := false
			result, code := BeginWithOptions(dir, names, replace, "", options, func(p Progress) {
				if p.Stage != "開始記録の保存" || changed {
					return
				}
				changed = true
				if kind == "concurrent" {
					if result, code := Begin(dir, []string{"outside.txt"}, true, ""); code != 0 {
						t.Fatalf("競合を再現できません: %d %+v", code, result)
					}
					before = taskData(t, dir, "")
				}
				if kind == "directory_changed" {
					if err := os.Rename(filepath.Join(dir, stateName), filepath.Join(dir, "old-state")); err != nil {
						t.Fatal(err)
					}
					write(t, dir, stateName+"/"+recordName, string(before))
				}
			})
			requireFailure(t, result, code, want)
			if (kind == "concurrent" || kind == "directory_changed") && !changed {
				t.Fatal("保存時の競合を再現していません")
			}
			if !bytes.Equal(before, taskData(t, dir, "")) {
				t.Fatal("失敗時に既存の記録を書き換えました")
			}
		})
	}
}

func TestScopeExtensionRejectsInvalidHistory(t *testing.T) {
	dir := extensionRepo(t)
	if result, code := Begin(dir, []string{"tracked.txt"}, false, ""); code != 0 {
		t.Fatalf("準備失敗: %d %+v", code, result)
	}
	write(t, dir, "second.txt", "追加前の編集")
	extend(t, dir, "", "second.txt")
	original, _ := readRecord(t, dir)
	for _, kind := range []string{"initial_missing", "history_missing", "allow_missing", "duplicate", "date", "reason", "changed_paths_missing", "changed_path_outside"} {
		t.Run(kind, func(t *testing.T) {
			data, _ := json.Marshal(original)
			var record Record
			if err := json.Unmarshal(data, &record); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "initial_missing":
				record.InitialAllow = nil
			case "history_missing":
				record.ScopeExtensions = nil
			case "allow_missing":
				record.Allow = record.InitialAllow
			case "duplicate":
				record.ScopeExtensions = append(record.ScopeExtensions, record.ScopeExtensions[0])
			case "date":
				record.ScopeExtensions[0].AddedAt = "2000-01-01T00:00:00Z"
			case "reason":
				record.ScopeExtensions[0].Reason = "\n"
			case "changed_paths_missing":
				record.ScopeExtensions[0].ChangedPaths = nil
			case "changed_path_outside":
				record.ScopeExtensions[0].ChangedPaths = []string{"outside.txt"}
			}
			data, _ = json.Marshal(record)
			write(t, dir, stateName+"/"+recordName, string(data))
			result, code := Check(dir, "")
			requireFailure(t, result, code, "foreign_state")
			result, code = BeginWithOptions(dir, []string{"new.txt"}, false, "", Options{Extend: true, Reason: "追加"})
			requireFailure(t, result, code, "foreign_state")
			if !bytes.Equal(data, taskData(t, dir, "")) {
				t.Fatal("不正な記録を変更しました")
			}
		})
	}
}
