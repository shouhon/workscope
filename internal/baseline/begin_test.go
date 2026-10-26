package baseline

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testGit(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	out, err := git(dir, nil, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

func write(t *testing.T, dir, name, text string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testGit(t, dir, "init", "-q")
	testGit(t, dir, "config", "core.autocrlf", "false")
	write(t, dir, "tracked.txt", "開始時の本文\n")
	testGit(t, dir, "add", ".")
	commit(t, dir)
	return dir
}

func commit(t *testing.T, dir string) {
	t.Helper()
	testGit(t, dir, "-c", "user.name=workscope-test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "-qm", "検証用")
}

func readRecord(t *testing.T, dir string) (Record, []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, stateName, recordName))
	if err != nil {
		t.Fatal(err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	return record, data
}

func requireFailure(t *testing.T, result Result, code int, issue string) {
	t.Helper()
	if code == 0 || result.Status == "recorded" || len(result.Issues) == 0 || result.Issues[0].Code != issue {
		t.Fatalf("期待した失敗 %s ではありません: code=%d result=%+v", issue, code, result)
	}
}

func TestBeginRecordsContentIndexAndGitStateWithoutChangingInputs(t *testing.T) {
	dir := repo(t)
	write(t, dir, ".gitignore", "ignored/\ntracked.txt\n")
	write(t, dir, "ignored/hidden.txt", "記録しない")
	write(t, dir, "tracked.txt", "ステージした本文\n")
	testGit(t, dir, "add", "tracked.txt")
	write(t, dir, "tracked.txt", "追加編集した秘密の本文\n")
	write(t, dir, "空白 日本語/new.txt", "未追跡の本文\n")
	indexBefore, err := os.ReadFile(filepath.Join(dir, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	configBefore, err := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	result, code := Begin(dir, []string{"空白 日本語", "future/new.txt", "tracked.txt"}, false, "")
	if code != 0 || result.Status != "recorded" || !result.Coverage.Complete {
		t.Fatalf("記録失敗: %d %+v", code, result)
	}
	record, data := readRecord(t, dir)
	if bytes.Contains(data, []byte("追加編集した秘密の本文")) {
		t.Fatal("開始記録に本文が保存されました")
	}
	if record.SchemaVersion != 1 || record.Tool != "workscope" || record.ID != result.BaselineID || len(record.ID) != 32 || record.Head == "" {
		t.Fatalf("識別情報が不正: %+v", record)
	}
	if _, err := time.Parse(time.RFC3339Nano, record.CreatedAt); err != nil {
		t.Fatal(err)
	}
	paths := []string{}
	for _, entry := range record.Files {
		paths = append(paths, entry.Path)
		if entry.Path == "tracked.txt" {
			hash := sha256.Sum256([]byte("追加編集した秘密の本文\n"))
			if entry.SHA256 != hex.EncodeToString(hash[:]) || entry.Kind != "file" {
				t.Fatalf("ハッシュが不正: %+v", entry)
			}
		}
	}
	if !reflect.DeepEqual(paths, []string{".gitignore", "tracked.txt", "空白 日本語/new.txt"}) {
		t.Fatalf("対象が不正: %v", paths)
	}
	if !reflect.DeepEqual(record.PreexistingChanges, []GitChange{{".gitignore", "??"}, {"tracked.txt", "MM"}, {"空白 日本語/new.txt", "??"}}) {
		t.Fatalf("開始前の変更が不正: %+v", record.PreexistingChanges)
	}
	if len(record.Index) != 1 || record.Index[0].Path != "tracked.txt" || record.Index[0].Object == "" {
		t.Fatalf("インデックスが不正: %+v", record.Index)
	}
	indexAfter, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
	configAfter, _ := os.ReadFile(filepath.Join(dir, ".git", "config"))
	if !bytes.Equal(indexBefore, indexAfter) || !bytes.Equal(configBefore, configAfter) {
		t.Fatal("記録がインデックスかGit設定を変更しました")
	}
	content, _ := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	if string(content) != "追加編集した秘密の本文\n" {
		t.Fatal("対象ファイルが変更されました")
	}
}

func TestBeginProtectsExistingRecordAndReplacesExplicitly(t *testing.T) {
	dir := repo(t)
	first, code := Begin(dir, []string{"tracked.txt"}, false, "")
	if code != 0 {
		t.Fatalf("%+v", first)
	}
	_, before := readRecord(t, dir)
	write(t, dir, "tracked.txt", "更新した本文")
	result, code := Begin(dir, []string{"tracked.txt"}, false, "")
	requireFailure(t, result, code, "baseline_exists")
	_, after := readRecord(t, dir)
	if !bytes.Equal(before, after) {
		t.Fatal("許可なしで上書きされました")
	}
	result, code = Begin(dir, []string{"tracked.txt"}, true, "")
	if code != 0 || result.BaselineID == first.BaselineID {
		t.Fatalf("取り直し失敗: %d %+v", code, result)
	}
	record, _ := readRecord(t, dir)
	if len(record.PreexistingChanges) != 1 || record.PreexistingChanges[0].Path != "tracked.txt" {
		t.Fatalf("新しい状態ではありません: %+v", record)
	}
	children, _ := os.ReadDir(filepath.Join(dir, stateName))
	if len(children) != 1 || children[0].Name() != recordName {
		t.Fatalf("一時ファイルが残っています: %v", children)
	}
}

func TestBeginRejectsForeignStateEvenWithReplace(t *testing.T) {
	for _, kind := range []string{"empty", "foreign", "unsupported", "broken", "extra", "other_root"} {
		t.Run(kind, func(t *testing.T) {
			dir := repo(t)
			if err := os.Mkdir(filepath.Join(dir, stateName), 0700); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "foreign":
				write(t, dir, stateName+"/notes.txt", "別用途の内容")
			case "unsupported":
				write(t, dir, stateName+"/"+recordName, `{"tool":"workscope","schema_version":999}`)
			case "broken":
				write(t, dir, stateName+"/"+recordName, "{")
			case "extra", "other_root":
				if err := os.Remove(filepath.Join(dir, stateName)); err != nil {
					t.Fatal(err)
				}
				result, code := Begin(dir, []string{"."}, false, "")
				if code != 0 {
					t.Fatalf("%+v", result)
				}
				if kind == "extra" {
					write(t, dir, stateName+"/notes.txt", "別用途の内容")
				} else {
					record, _ := readRecord(t, dir)
					record.Root = t.TempDir()
					data, _ := json.Marshal(record)
					write(t, dir, stateName+"/"+recordName, string(data))
				}
			}
			before := map[string]string{}
			children, _ := os.ReadDir(filepath.Join(dir, stateName))
			for _, child := range children {
				data, _ := os.ReadFile(filepath.Join(dir, stateName, child.Name()))
				before[child.Name()] = string(data)
			}
			result, code := Begin(dir, []string{"."}, true, "")
			requireFailure(t, result, code, "foreign_state")
			for name, want := range before {
				data, err := os.ReadFile(filepath.Join(dir, stateName, name))
				if err != nil || string(data) != want {
					t.Fatal("別用途のファイルが変化しました")
				}
			}
		})
	}
}

func TestAllowNormalizationAndRejection(t *testing.T) {
	dir := repo(t)
	write(t, dir, "src/日本語.txt", "内容")
	r, err := openRepository(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer r.fs.Close()
	allowed, err := r.normalizeAllow([]string{"src/../src", "./src/", "new/path", "src/日本語.txt"})
	if err != nil || !reflect.DeepEqual(allowed, []Allowed{{"new/path", "future"}, {"src", "directory"}, {"src/日本語.txt", "file"}}) {
		t.Fatalf("正規化失敗: %+v %v", allowed, err)
	}
	if beneath("src2/file", "src") || !beneath("src/file", "src") {
		t.Fatal("ディレクトリ境界が不正です")
	}
	for _, name := range []string{"", "../outside", ".git", ".git/config", ".workscope/baseline.json", "src/*", "src/?", "src/[a]", dir, "tracked.txt/child", "new/../../outside"} {
		result, code := Begin(dir, []string{name}, false, "")
		if code != 64 || result.Status != "invalid_arguments" {
			t.Errorf("%q を拒否しませんでした: %d %+v", name, code, result)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, stateName)); !os.IsNotExist(err) {
		t.Fatal("誤った引数で保存先が作成されました")
	}
}

func TestBeginUnbornAndTrackedDeletionAndKindChange(t *testing.T) {
	t.Run("コミット前", func(t *testing.T) {
		dir := t.TempDir()
		testGit(t, dir, "init", "-q")
		write(t, dir, "new.txt", "本文")
		result, code := Begin(dir, []string{"new.txt"}, false, "")
		if code != 0 {
			t.Fatalf("%d %+v", code, result)
		}
		record, _ := readRecord(t, dir)
		if record.Head != "" || len(record.Files) != 1 {
			t.Fatalf("%+v", record)
		}
	})
	for _, kind := range []string{"missing", "directory"} {
		t.Run(kind, func(t *testing.T) {
			dir := repo(t)
			if err := os.Remove(filepath.Join(dir, "tracked.txt")); err != nil {
				t.Fatal(err)
			}
			if kind == "directory" {
				write(t, dir, "tracked.txt/child", "配下の本文")
			}
			result, code := Begin(dir, []string{"tracked.txt"}, false, "")
			if code != 0 {
				t.Fatalf("%d %+v", code, result)
			}
			record, _ := readRecord(t, dir)
			if record.Files[0].Path != "tracked.txt" || record.Files[0].Kind != kind {
				t.Fatalf("種類が不正: %+v", record.Files)
			}
		})
	}
}

func TestBeginFromWorktreeSubdirectory(t *testing.T) {
	dir := repo(t)
	worktree := filepath.Join(t.TempDir(), "追加 作業ツリー")
	testGit(t, dir, "worktree", "add", "-qb", "test-worktree", worktree)
	write(t, worktree, "sub/new.txt", "作業ツリーの本文")
	result, code := Begin(filepath.Join(worktree, "sub"), []string{"sub"}, false, "")
	if code != 0 {
		t.Fatalf("%d %+v", code, result)
	}
	record, _ := readRecord(t, worktree)
	actual, _ := filepath.EvalSymlinks(worktree)
	if record.Root != actual || len(record.Files) != 2 {
		t.Fatalf("作業ツリーのルートが不正: %+v", record)
	}
	if _, err := os.Stat(filepath.Join(dir, stateName)); !os.IsNotExist(err) {
		t.Fatal("主作業ツリーへ記録しました")
	}
}

func TestBeginDoesNotFollowLinks(t *testing.T) {
	dir := repo(t)
	outside := t.TempDir()
	write(t, outside, "secret.txt", "リンク先の本文")
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Skipf("この環境ではリンクを作成できません: %v", err)
	}
	result, code := Begin(dir, []string{"link"}, false, "")
	if code != 0 {
		t.Fatalf("%d %+v", code, result)
	}
	record, data := readRecord(t, dir)
	if len(record.Files) != 2 || record.Files[0].Path != "link" || record.Files[0].Kind != "symlink" || record.Files[0].Target != outside {
		t.Fatalf("リンクの記録が不正: %+v", record.Files)
	}
	if bytes.Contains(data, []byte("リンク先の本文")) || bytes.Contains(data, []byte("secret.txt")) {
		t.Fatal("リンク先を走査しました")
	}
	result, code = Begin(dir, []string{"link/secret.txt"}, true, "")
	if code != 64 {
		t.Fatalf("リンク配下を許容しました: %+v", result)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "state-link")); err != nil {
		t.Fatal(err)
	}
	// 開始記録の置換でも、保存先にあるリンクをたどらない。
	other := repo(t)
	if err := os.Symlink(outside, filepath.Join(other, stateName)); err != nil {
		t.Fatal(err)
	}
	result, code = Begin(other, []string{"."}, true, "")
	requireFailure(t, result, code, "foreign_state")
}

func TestBeginSubmoduleNeverReportsCompleteSuccess(t *testing.T) {
	dir := repo(t)
	hash := strings.TrimSpace(string(testGit(t, dir, "rev-parse", "HEAD")))
	testGit(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+hash+",module")
	result, code := Begin(dir, []string{"."}, false, "")
	requireFailure(t, result, code, "unsupported_submodule")
	if result.Coverage.Complete || !reflect.DeepEqual(result.Coverage.Submodules, []string{"module"}) {
		t.Fatalf("対象外の説明が不正: %+v", result.Coverage)
	}
	if _, err := os.Stat(filepath.Join(dir, stateName)); !os.IsNotExist(err) {
		t.Fatal("不完全な開始記録を保存しました")
	}
}

func TestGitParsersPreserveWhitespaceAndNewlines(t *testing.T) {
	name := "空白 日本語/line\n\tname"
	index, err := parseIndex([]byte("100644 " + strings.Repeat("a", 40) + " 0\t" + name + "\x00"))
	if err != nil || len(index) != 1 || index[0].Path != name {
		t.Fatalf("%+v %v", index, err)
	}
	status, err := parseStatus([]byte(" M " + name + "\x00"))
	if err != nil || status[0].Path != name {
		t.Fatalf("%+v %v", status, err)
	}
	for _, data := range [][]byte{[]byte(" M path\n"), []byte("x\x00"), []byte(" M ../outside\x00"), []byte("R  new\x00old\x00")} {
		if _, err := parseStatus(data); err == nil {
			t.Fatalf("不正な状態を受理: %q", data)
		}
	}
}

func TestBeginRejectsFileChangingDuringCapture(t *testing.T) {
	dir := repo(t)
	name := filepath.Join(dir, "large.bin")
	if err := os.WriteFile(name, make([]byte, 32<<20), 0600); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan error, 1)
	started := make(chan struct{})
	var writes atomic.Int64
	go func() {
		for i := int64(1); ; i++ {
			select {
			case <-stop:
				done <- nil
				return
			default:
			}
			file, err := os.OpenFile(name, os.O_WRONLY, 0)
			if err != nil {
				done <- err
				return
			}
			_, err = file.WriteAt([]byte(fmt.Sprintf("%020d", i)), 0)
			closeErr := file.Close()
			if err != nil {
				done <- err
				return
			}
			if closeErr != nil {
				done <- closeErr
				return
			}
			writes.Add(1)
			if i == 1 {
				close(started)
			}
			time.Sleep(time.Millisecond)
		}
	}()
	select {
	case <-started:
	case err := <-done:
		t.Fatal(err)
	}
	result, code := Begin(dir, []string{"."}, false, "")
	close(stop)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if writes.Load() < 2 {
		t.Fatal("記録中の変更を再現できませんでした")
	}
	requireFailure(t, result, code, "snapshot_changed")
	if _, err := os.Stat(filepath.Join(dir, stateName)); !os.IsNotExist(err) {
		t.Fatal("取得中に変わった状態を保存しました")
	}
}
