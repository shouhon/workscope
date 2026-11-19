package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"workscope/internal/baseline"
	"workscope/internal/cli"
)

func TestWindowsCLIUnreadableAllowDiagnosticsAndModesPreserveState(t *testing.T) {
	dir := summaryRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "locked", "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	summaryWrite(t, dir, "locked/sub/validate.py", "開始時の本文")
	summaryGit(t, dir, "add", "locked/sub/validate.py")
	summaryCommit(t, dir)
	if result, code := baseline.Begin(dir, []string{"input.txt"}, false, ""); code != 0 {
		t.Fatalf("準備失敗: %d %+v", code, result)
	}
	protected := map[string][]byte{}
	for _, name := range []string{"input.txt", ".git/index", ".git/HEAD", ".git/config", ".workscope/baseline.json"} {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		protected[name] = data
	}

	lockedDir := filepath.Join(dir, "locked")
	utf16Path, err := syscall.UTF16PtrFromString(lockedDir)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(utf16Path, syscall.GENERIC_READ, 0, nil,
		syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(handle)

	t.Chdir(dir)
	for _, tc := range []struct {
		name string
		base []string
	}{
		{"begin", []string{"begin", "--task", "新規タスク", "--allow", "locked/sub/validate.py", "--no-progress"}},
		{"extend", []string{"begin", "--extend", "--allow", "locked/sub/validate.py", "--reason", "追加依頼", "--no-progress"}},
	} {
		for _, mode := range []string{"", "--json", "--json-summary"} {
			for _, verbose := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/verbose=%v", tc.name, mode, verbose), func(t *testing.T) {
					args := append([]string(nil), tc.base...)
					if mode != "" {
						args = append(args, mode)
					}
					if verbose {
						args = append(args, "--verbose")
					}
					var stdout, stderr bytes.Buffer
					code := cli.Run(args, &stdout, &stderr)
					if code != 2 {
						t.Fatalf("参照失敗が終了コード2ではありません: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
					}
					if mode == "" {
						if stdout.Len() != 0 {
							t.Fatalf("失敗時に標準出力へ書き込みました: %s", &stdout)
						}
						text := stderr.String()
						if !strings.Contains(text, "許容対象のパスを確認できません") || !strings.Contains(text, `("locked`) || !strings.Contains(text, "workscope自身の終了コード: 2") {
							t.Fatalf("通常表示の診断が不正です: %s", text)
						}
						if strings.Contains(text, "失敗した確認段階:") != verbose || strings.Contains(text, "原因区分:") != verbose {
							t.Fatalf("通常表示の詳細条件が不正です (verbose=%v): %s", verbose, text)
						}
						if strings.Contains(text, dir) || strings.Contains(text, filepath.ToSlash(dir)) {
							t.Fatalf("通常表示に絶対パスが漏れました: %s", text)
						}
						return
					}
					if !validCommandStderr(stderr.String(), 2) {
						t.Fatalf("JSON実行時の標準エラーが不正です: %s", &stderr)
					}
					var result baseline.Result
					decoder := json.NewDecoder(&stdout)
					if err := decoder.Decode(&result); err != nil {
						t.Fatal(err)
					}
					if err := decoder.Decode(new(any)); err != io.EOF {
						t.Fatalf("JSONが1件ではありません: %v", err)
					}
					if result.Status != "unknown" || result.Coverage.Complete || len(result.Issues) != 1 {
						t.Fatalf("JSONの判定が不正です: %+v", result)
					}
					issue := result.Issues[0]
					if issue.Code != "unreadable_path" || (issue.Path != "locked" && issue.Path != "locked/sub") || (issue.Details != "") != verbose {
						t.Fatalf("JSONの診断が不正です (verbose=%v): %+v", verbose, issue)
					}
					if verbose && (!strings.Contains(issue.Details, "失敗した確認段階:") || !strings.Contains(issue.Details, "原因区分:") || !strings.Contains(issue.Details, `対象要素: "`+issue.Path+`"`)) {
						t.Fatalf("JSON詳細の形式が不正です: %+v", issue)
					}
					if strings.Contains(issue.Message+issue.Details, dir) || strings.Contains(issue.Message+issue.Details, filepath.ToSlash(dir)) {
						t.Fatalf("JSON診断に絶対パスが漏れました: %+v", issue)
					}
				})
			}
		}
	}

	// 形式不正は引き続き64・invalid_arguments・invalid_allowを維持する。
	var stdout, stderr bytes.Buffer
	if code := cli.Run([]string{"begin", "--allow", "../outside", "--json", "--no-progress"}, &stdout, &stderr); code != 64 || !strings.Contains(stdout.String(), `"code":"invalid_allow"`) || stderr.Len() != 0 {
		t.Fatalf("形式不正の診断が変わりました: code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
	}

	for name, before := range protected {
		after, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("拒否した実行が状態を変更しました: %s %v", name, err)
		}
	}
	children, err := os.ReadDir(filepath.Join(dir, ".workscope"))
	if err != nil || len(children) != 1 || children[0].Name() != "baseline.json" {
		t.Fatalf("状態保存先が変化しました: %v %v", children, err)
	}
}
