package baseline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishNewRecordDoesNotReplaceExistingContents(t *testing.T) {
	for _, kind := range []string{"missing", "file", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			const temporary, name = "temporary.json", "記録.json"
			write(t, dir, temporary, "新しい記録")
			switch kind {
			case "file":
				write(t, dir, name, "並行処理の内容")
			case "directory":
				if err := os.Mkdir(filepath.Join(dir, name), 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				outside := filepath.Join(t.TempDir(), "external.json")
				if err := os.WriteFile(outside, []byte("並行処理の内容"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(dir, name)); err != nil {
					t.Skipf("リンクを作成できません: %v", err)
				}
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			file, err := root.OpenFile(temporary, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			err = publishNewRecord(root, file, temporary, name)
			if kind == "missing" {
				if err != nil {
					t.Fatalf("新規記録を確定できません: %v", err)
				}
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || string(data) != "新しい記録" {
					t.Fatalf("確定した内容が不正: %s %v", data, err)
				}
				return
			}
			if err == nil {
				t.Fatal("既存先を上書きしました")
			}
			if kind == "directory" {
				children, err := os.ReadDir(filepath.Join(dir, name))
				if err != nil || len(children) != 0 {
					t.Fatalf("既存のディレクトリを変更しました: %v %v", children, err)
				}
			} else {
				data, err := os.ReadFile(filepath.Join(dir, name))
				if err != nil || string(data) != "並行処理の内容" {
					t.Fatalf("既存の内容を変更しました: %s %v", data, err)
				}
				if kind == "symlink" {
					if info, err := os.Lstat(filepath.Join(dir, name)); err != nil || info.Mode()&os.ModeSymlink == 0 {
						t.Fatalf("既存のリンクを変更しました: %v", err)
					}
				}
			}
			data, err := os.ReadFile(filepath.Join(dir, temporary))
			if err != nil || string(data) != "新しい記録" {
				t.Fatalf("衝突時に一時記録を変更しました: %s %v", data, err)
			}
		})
	}
}
