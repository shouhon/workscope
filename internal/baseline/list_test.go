package baseline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListRecordsWithoutCheckingCurrentHead(t *testing.T) {
	for _, worktree := range []bool{false, true} {
		t.Run(map[bool]string{false: "通常", true: "worktree"}[worktree], func(t *testing.T) {
			dir := repo(t)
			if worktree {
				other := filepath.Join(t.TempDir(), "追加 作業ツリー")
				testGit(t, dir, "worktree", "add", "--detach", "-q", other)
				dir = other
			}
			var want []RecordSummary
			for _, task := range []string{"", "日本語 作業A", "日本語 作業a"} {
				result, code := Begin(dir, []string{"tracked.txt", "future"}, false, task)
				if code != 0 {
					t.Fatalf("記録失敗: %d %+v", code, result)
				}
				var record Record
				if err := json.Unmarshal(taskData(t, dir, task), &record); err != nil {
					t.Fatal(err)
				}
				want = append(want, RecordSummary{Task: task, Default: task == "", BaselineID: result.BaselineID, CreatedAt: record.CreatedAt, AllowCount: 2})
			}
			write(t, dir, "tracked.txt", "HEADを変更する本文")
			testGit(t, dir, "add", "tracked.txt")
			commit(t, dir)
			protected := map[string][]byte{}
			for _, task := range []string{"", "日本語 作業A", "日本語 作業a"} {
				protected[filepath.Join(dir, stateName, RecordName(task))] = taskData(t, dir, task)
			}
			for _, name := range []string{"tracked.txt", ".git"} {
				full := filepath.Join(dir, name)
				if info, err := os.Stat(full); err == nil && info.Mode().IsRegular() {
					protected[full], err = os.ReadFile(full)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			for _, args := range [][]string{{"--git-path", "index"}, {"--git-path", "HEAD"}, {"--git-common-dir"}} {
				name, err := gitLine(dir, nil, Options{}, append([]string{"rev-parse", "--path-format=absolute"}, args...)...)
				if err != nil {
					t.Fatal(err)
				}
				if args[0] == "--git-common-dir" {
					name = filepath.Join(name, "config")
				}
				protected[name], err = os.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				result, code := List(dir, Options{TrustRepository: dir})
				if code != 0 || result.Status != "listed" || result.Command != "list" || len(result.Issues) != 0 || !reflect.DeepEqual(result.Records, want) {
					t.Fatalf("HEAD変更後の一覧が不正: %d %+v", code, result)
				}
			}
			checked, code := Check(dir, "日本語 作業A")
			if code != 2 || checked.Status != "unknown" {
				t.Fatalf("一覧が照合を成功扱いにしました: %d %+v", code, checked)
			}
			for name, before := range protected {
				after, err := os.ReadFile(name)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("一覧が既存状態を変更しました: %s %v", name, err)
				}
			}
		})
	}
}

func TestListDistinguishesMissingStateFromInvalidRecords(t *testing.T) {
	for _, kind := range []string{"missing", "empty", "broken", "version", "root", "task", "extra", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := repo(t)
			if kind == "missing" {
				result, code := List(dir, Options{})
				if code != 0 || result.Status != "listed" || result.Records == nil || len(result.Records) != 0 {
					t.Fatalf("未作成の保存先の一覧が不正: %d %+v", code, result)
				}
				if _, err := os.Stat(filepath.Join(dir, stateName)); !os.IsNotExist(err) {
					t.Fatal("一覧が状態保存先を作成しました")
				}
				return
			}
			if kind == "empty" {
				if err := os.Mkdir(filepath.Join(dir, stateName), 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if result, code := Begin(dir, []string{"future"}, false, ""); code != 0 {
					t.Fatalf("準備失敗: %d %+v", code, result)
				}
				record, original := readRecord(t, dir)
				switch kind {
				case "broken":
					write(t, dir, stateName+"/"+recordName, "{")
				case "version", "root", "task":
					if kind == "version" {
						record.SchemaVersion++
					} else if kind == "root" {
						record.Root = t.TempDir()
					} else {
						record.Task = "別の名前"
					}
					data, err := json.Marshal(record)
					if err != nil {
						t.Fatal(err)
					}
					write(t, dir, stateName+"/"+recordName, string(data))
				case "extra":
					write(t, dir, stateName+"/other.txt", "別用途")
				case "symlink":
					external := filepath.Join(t.TempDir(), "external.json")
					if err := os.WriteFile(external, original, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Remove(filepath.Join(dir, stateName, recordName)); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(external, filepath.Join(dir, stateName, recordName)); err != nil {
						t.Skipf("リンクを作成できません: %v", err)
					}
				}
			}
			result, code := List(dir, Options{})
			if code != 2 || result.Status != "unknown" || len(result.Issues) == 0 || len(result.Records) != 0 {
				t.Fatalf("不正な状態を空の正常一覧にしました: %d %+v", code, result)
			}
		})
	}
}
