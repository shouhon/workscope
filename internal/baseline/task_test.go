package baseline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func taskData(t *testing.T, dir, task string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, stateName, RecordName(task)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestTaskRecordsCoexistAndSelectIndependentBaselines(t *testing.T) {
	dir := repo(t)
	const firstTask, secondTask = "日本語 作業A", "日本語 作業a"
	legacy, code := Begin(dir, []string{"tracked.txt"}, false, "")
	if code != 0 {
		t.Fatalf("従来の記録失敗: %d %+v", code, legacy)
	}
	legacyData := taskData(t, dir, "")
	if bytes.Contains(legacyData, []byte(`"task"`)) {
		t.Fatal("タスク省略時に旧形式へ項目を追加しました")
	}
	first, code := Begin(dir, []string{"tracked.txt"}, false, firstTask)
	if code != 0 || first.Task != firstTask || first.BaselineID == legacy.BaselineID {
		t.Fatalf("1件目の記録失敗: %d %+v", code, first)
	}
	firstData := taskData(t, dir, firstTask)
	write(t, dir, "tracked.txt", "2件目の開始時")
	second, code := Begin(dir, []string{"future"}, false, secondTask)
	if code != 0 || second.Task != secondTask || second.BaselineID == first.BaselineID {
		t.Fatalf("2件目の記録失敗: %d %+v", code, second)
	}
	secondData := taskData(t, dir, secondTask)
	for _, tc := range []struct {
		task    string
		id      string
		changes int
	}{{"", legacy.BaselineID, 1}, {firstTask, first.BaselineID, 1}, {secondTask, second.BaselineID, 0}} {
		result, code := Check(dir, tc.task)
		if code != 0 || result.Task != tc.task || result.BaselineID != tc.id || len(result.ChangesSinceBegin) != tc.changes {
			t.Fatalf("別の記録と照合しました: %d %+v", code, result)
		}
	}
	missing, code := Check(dir, "未記録の作業")
	requireFailure(t, missing, code, "baseline_missing")
	if missing.BaselineID != "" {
		t.Fatal("未記録のタスクで別の開始記録を使いました")
	}
	write(t, dir, "tracked.txt", "2件目の開始後")
	result, code := Check(dir, secondTask)
	if code != 1 || len(result.OutOfScopeChanges) != 1 {
		t.Fatalf("タスクごとの許容範囲を使いませんでした: %d %+v", code, result)
	}
	result, code = Begin(dir, []string{"future"}, false, firstTask)
	requireFailure(t, result, code, "baseline_exists")
	if !bytes.Equal(firstData, taskData(t, dir, firstTask)) {
		t.Fatal("明示的な取り直しなしでタスクの開始記録を変更しました")
	}
	replaced, code := Begin(dir, []string{"future"}, true, firstTask)
	if code != 0 || replaced.BaselineID == first.BaselineID {
		t.Fatalf("タスクの取り直し失敗: %d %+v", code, replaced)
	}
	result, code = Check(dir, firstTask)
	if code != 0 || result.BaselineID != replaced.BaselineID || len(result.ChangesSinceBegin) != 0 {
		t.Fatalf("取り直した記録で照合しませんでした: %d %+v", code, result)
	}
	if !bytes.Equal(legacyData, taskData(t, dir, "")) || !bytes.Equal(secondData, taskData(t, dir, secondTask)) {
		t.Fatal("別タスクか従来の開始記録を変更しました")
	}
	children, err := os.ReadDir(filepath.Join(dir, stateName))
	if err != nil || len(children) != 3 {
		t.Fatalf("開始記録以外のファイルが残りました: %v %v", children, err)
	}
}

func TestTaskRecordsCanPrecedeDefaultRecord(t *testing.T) {
	dir := repo(t)
	result, code := Begin(dir, []string{"tracked.txt"}, false, "CON: 日本語")
	if code != 0 {
		t.Fatalf("タスクだけの初回記録失敗: %d %+v", code, result)
	}
	before := taskData(t, dir, result.Task)
	missing, code := Check(dir, "")
	requireFailure(t, missing, code, "baseline_missing")
	legacy, code := Begin(dir, []string{"future"}, false, "")
	if code != 0 || legacy.BaselineID == result.BaselineID {
		t.Fatalf("後から従来の記録を追加できません: %d %+v", code, legacy)
	}
	if !bytes.Equal(before, taskData(t, dir, result.Task)) {
		t.Fatal("従来の記録の追加がタスクの記録を変更しました")
	}
}

func TestTaskNamesRejectUnsafeAndAmbiguousArguments(t *testing.T) {
	dir := t.TempDir()
	for _, task := range []string{"../outside", `a\b`, "a/b", " 前後", "前後 ", "\t", "改\n行", "a\x00b", string([]byte{0xff})} {
		t.Run(task, func(t *testing.T) {
			for _, run := range []func() (Result, int){
				func() (Result, int) { return Begin(dir, []string{"."}, true, task) },
				func() (Result, int) { return Check(dir, task) },
			} {
				result, code := run()
				if code != 64 || result.Status != "invalid_arguments" || result.Coverage.Complete || len(result.Issues) != 1 || result.Issues[0].Code != "invalid_task" {
					t.Fatalf("不正なタスク名を拒否しませんでした: %d %+v", code, result)
				}
			}
		})
	}
	children, err := os.ReadDir(dir)
	if err != nil || len(children) != 0 {
		t.Fatalf("引数エラーで状態を作成しました: %v %v", children, err)
	}
}

func TestTaskRecordsProtectMismatchedAndUnreadableState(t *testing.T) {
	for _, kind := range []string{"task_mismatch", "filename_mismatch", "broken", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := repo(t)
			result, code := Begin(dir, []string{"."}, false, "元の作業")
			if code != 0 {
				t.Fatalf("記録失敗: %d %+v", code, result)
			}
			name := RecordName(result.Task)
			data := taskData(t, dir, result.Task)
			switch kind {
			case "task_mismatch":
				var record Record
				if err := json.Unmarshal(data, &record); err != nil {
					t.Fatal(err)
				}
				record.Task = "別の作業"
				data, _ = json.Marshal(record)
				write(t, dir, stateName+"/"+name, string(data))
			case "filename_mismatch":
				other := RecordName("別の作業")
				if err := os.Rename(filepath.Join(dir, stateName, name), filepath.Join(dir, stateName, other)); err != nil {
					t.Fatal(err)
				}
				name = other
			case "broken":
				data = []byte("{")
				write(t, dir, stateName+"/"+name, string(data))
			case "symlink":
				target := filepath.Join(t.TempDir(), "external.json")
				if err := os.WriteFile(target, data, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(dir, stateName, name)); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(dir, stateName, name)); err != nil {
					t.Skipf("この環境ではリンクを作成できません: %v", err)
				}
			}
			result, code = Check(dir, "元の作業")
			requireFailure(t, result, code, "foreign_state")
			result, code = Begin(dir, []string{"."}, true, "追加の作業")
			requireFailure(t, result, code, "foreign_state")
			after, err := os.ReadFile(filepath.Join(dir, stateName, name))
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("拒否した記録を変更しました")
			}
			children, err := os.ReadDir(filepath.Join(dir, stateName))
			if err != nil || len(children) != 1 {
				t.Fatalf("拒否した状態保存先へ書き込みました: %v %v", children, err)
			}
		})
	}
}

func TestTaskSaveRejectsRecordCreatedAfterInitialRead(t *testing.T) {
	dir := repo(t)
	first, code := Begin(dir, []string{"."}, false, "作業")
	if code != 0 {
		t.Fatalf("記録失敗: %d %+v", code, first)
	}
	before := taskData(t, dir, first.Task)
	var record Record
	if err := json.Unmarshal(before, &record); err != nil {
		t.Fatal(err)
	}
	record.ID = token()
	r, err := openRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.fs.Close()
	// 初回読取では記録がなかった状況を再現し、保存時の競合検出を確認する。
	if err := r.save(record, nil); err == nil || describeError(err).Code != "state_changed" {
		t.Fatalf("後から作成された記録を上書きしました: %v", err)
	}
	if !bytes.Equal(before, taskData(t, dir, first.Task)) {
		t.Fatal("競合検出後に開始記録が変化しました")
	}
}

func TestTaskSaveRejectsEmptyDirectoryCreatedAfterInitialRead(t *testing.T) {
	dir := repo(t)
	result, code := Begin(dir, []string{"."}, false, "")
	if code != 0 {
		t.Fatalf("記録失敗: %d %+v", code, result)
	}
	record, _ := readRecord(t, dir)
	if err := os.Remove(filepath.Join(dir, stateName, recordName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, stateName)); err != nil {
		t.Fatal(err)
	}
	r, err := openRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.fs.Close()
	if data, err := r.existing(""); err != nil || data != nil {
		t.Fatalf("記録なしの開始状態を再現できません: %v", err)
	}
	// 状態取得中に別用途の空ディレクトリが作られた状況を再現する。
	if err := os.Mkdir(filepath.Join(dir, stateName), 0700); err != nil {
		t.Fatal(err)
	}
	if err := r.save(record, nil); err == nil || describeError(err).Code != "state_changed" {
		t.Fatalf("後から作成された空ディレクトリを使用しました: %v", err)
	}
	children, err := os.ReadDir(filepath.Join(dir, stateName))
	if err != nil || len(children) != 0 {
		t.Fatalf("別用途のディレクトリへ記録を書き込みました: %v %v", children, err)
	}
}
