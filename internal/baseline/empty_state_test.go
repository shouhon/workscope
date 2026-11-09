package baseline

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestBeginUsesExistingEmptyDirectory(t *testing.T) {
	for _, task := range []string{"", "日本語 作業"} {
		for _, replace := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/replace=%t", task, replace), func(t *testing.T) {
				dir := repo(t)
				write(t, dir, ".gitignore", "/.workscope/\n")
				write(t, dir, "tracked.txt", "開始前の編集")
				state := filepath.Join(dir, stateName)
				if err := os.Mkdir(state, 0700); err != nil {
					t.Fatal(err)
				}
				opened, err := os.Open(state)
				if err != nil {
					t.Fatal(err)
				}
				defer opened.Close()
				identity, err := opened.Stat()
				if err != nil {
					t.Fatal(err)
				}
				protected := map[string][]byte{}
				for _, name := range []string{"tracked.txt", ".gitignore", ".git/index", ".git/HEAD", ".git/config"} {
					data, err := os.ReadFile(filepath.Join(dir, name))
					if err != nil {
						t.Fatal(err)
					}
					protected[name] = data
				}
				result, code := Begin(dir, []string{"tracked.txt"}, replace, task)
				if code != 0 || result.Status != "recorded" || result.BaselineID == "" || !result.Coverage.Complete {
					t.Fatalf("空の保存先で開始できません: %d %+v", code, result)
				}
				current, err := os.Stat(state)
				if err != nil || !os.SameFile(identity, current) {
					t.Fatalf("既存のディレクトリを置き換えました: %v", err)
				}
				children, err := os.ReadDir(state)
				if err != nil || len(children) != 1 || children[0].Name() != RecordName(task) {
					t.Fatalf("開始記録以外を残しました: %v %v", children, err)
				}
				for name, before := range protected {
					after, err := os.ReadFile(filepath.Join(dir, name))
					if err != nil || !bytes.Equal(before, after) {
						t.Fatalf("対象かGit状態を変更しました: %s %v", name, err)
					}
				}
				checked, code := Check(dir, task)
				if code != 0 || checked.BaselineID != result.BaselineID || len(checked.ChangesSinceBegin) != 0 {
					t.Fatalf("開始前の編集を開始後と扱いました: %d %+v", code, checked)
				}
				before := taskData(t, dir, task)
				rejected, code := Begin(dir, []string{"tracked.txt"}, false, task)
				requireFailure(t, rejected, code, "baseline_exists")
				if !bytes.Equal(before, taskData(t, dir, task)) {
					t.Fatal("追加指定なしで既存記録を上書きしました")
				}
			})
		}
	}
}

func TestBeginRejectsChangesToInitiallyEmptyDirectory(t *testing.T) {
	for _, kind := range []string{"replaced", "removed", "foreign", "hidden", "record", "locked"} {
		t.Run(kind, func(t *testing.T) {
			dir := repo(t)
			state := filepath.Join(dir, stateName)
			if err := os.Mkdir(state, 0700); err != nil {
				t.Fatal(err)
			}
			changed := false
			result, code := Begin(dir, []string{"tracked.txt"}, true, "", func(p Progress) {
				if p.Stage != "開始記録の保存" || changed {
					return
				}
				changed = true
				switch kind {
				case "replaced":
					if err := os.Rename(state, filepath.Join(dir, ".original-state")); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(state, 0700); err != nil {
						t.Fatal(err)
					}
				case "removed":
					if err := os.Remove(state); err != nil {
						t.Fatal(err)
					}
				default:
					name := map[string]string{"foreign": "notes.txt", "hidden": ".hidden", "record": recordName, "locked": "record.lock"}[kind]
					write(t, dir, stateName+"/"+name, "並行処理の内容")
				}
			})
			if !changed || code != 2 || result.Status != "unknown" || result.Coverage.Complete {
				t.Fatalf("保存先の競合を停止できません: changed=%v code=%d result=%+v", changed, code, result)
			}
			if kind == "removed" {
				if _, err := os.Lstat(state); !os.IsNotExist(err) {
					t.Fatalf("消えた保存先を作り直しました: %v", err)
				}
				return
			}
			children, err := os.ReadDir(state)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "replaced" {
				original, err := os.ReadDir(filepath.Join(dir, ".original-state"))
				if err != nil || len(original) != 0 || len(children) != 0 {
					t.Fatalf("差し替えた保存先を変更しました: %v %v %v", original, children, err)
				}
				return
			}
			if len(children) != 1 {
				t.Fatalf("競合した保存先の内容を変更しました: %v", children)
			}
			data, err := os.ReadFile(filepath.Join(state, children[0].Name()))
			if err != nil || string(data) != "並行処理の内容" {
				t.Fatalf("並行処理の内容を変更しました: %s %v", data, err)
			}
		})
	}
}
