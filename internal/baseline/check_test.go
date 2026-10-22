package baseline

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func beginForCheck(t *testing.T, dir string, allow ...string) Record {
	t.Helper()
	result, code := Begin(dir, allow, false)
	if code != 0 {
		t.Fatalf("開始記録失敗: %d %+v", code, result)
	}
	record, _ := readRecord(t, dir)
	return record
}

func TestCheckSeparatesPreexistingChangesAndReediting(t *testing.T) {
	dir := repo(t)
	write(t, dir, "tracked.txt", "開始前の編集")
	testGit(t, dir, "add", "tracked.txt")
	write(t, dir, "tracked.txt", "開始前の追加編集")
	record := beginForCheck(t, dir, "future")
	_, baselineBefore := readRecord(t, dir)
	result, code := Check(dir)
	if code != 0 || result.Status != "within_scope" || !reflect.DeepEqual(result.PreexistingChanges, record.PreexistingChanges) || len(result.ChangesSinceBegin) != 0 || len(result.IndexChanges) != 0 {
		t.Fatalf("開始前の変更を区別できません: %d %+v", code, result)
	}
	write(t, dir, "tracked.txt", "開始後の再編集")
	indexBefore, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
	configBefore, _ := os.ReadFile(filepath.Join(dir, ".git", "config"))
	result, code = Check(dir)
	if code != 1 || result.Status != "out_of_scope" || len(result.ChangesSinceBegin) != 1 || len(result.IndexChanges) != 0 || len(result.OutOfScopeChanges) != 1 {
		t.Fatalf("再編集を検出できません: %d %+v", code, result)
	}
	change := result.ChangesSinceBegin[0]
	if change.Path != "tracked.txt" || change.Kind != "modified" || change.WithinScope || change.BeforeSHA256 != record.Files[0].SHA256 || change.BeforeSHA256 == change.AfterSHA256 || len(change.AfterSHA256) != 64 {
		t.Fatalf("内容差分が不正: %+v", change)
	}
	again, againCode := Check(dir)
	firstJSON, _ := json.Marshal(result)
	secondJSON, _ := json.Marshal(again)
	if againCode != code || !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("同一状態の出力が安定しません")
	}
	_, baselineAfter := readRecord(t, dir)
	indexAfter, _ := os.ReadFile(filepath.Join(dir, ".git", "index"))
	configAfter, _ := os.ReadFile(filepath.Join(dir, ".git", "config"))
	content, _ := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	if !bytes.Equal(baselineBefore, baselineAfter) || !bytes.Equal(indexBefore, indexAfter) || !bytes.Equal(configBefore, configAfter) || string(content) != "開始後の再編集" {
		t.Fatal("照合が開始記録・インデックス・設定・対象を変更しました")
	}
}

func TestCheckChangeKindsAndDirectoryBoundaries(t *testing.T) {
	dir := repo(t)
	for _, name := range []string{"src/modify", "src/delete", "src/type", "src2/modify"} {
		write(t, dir, name, "開始時")
	}
	testGit(t, dir, "add", ".")
	commit(t, dir)
	beginForCheck(t, dir, "src", "新規 空白")
	write(t, dir, "src/modify", "更新")
	write(t, dir, "src/add", "追加")
	write(t, dir, "src2/modify", "範囲外")
	write(t, dir, "新規 空白/日本語.txt", "追加")
	if err := os.Remove(filepath.Join(dir, "src", "delete")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "src", "type")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "src/type/child", "種類変更後の配下")
	result, code := Check(dir)
	if code != 1 || len(result.OutOfScopeChanges) != 1 || len(result.Issues) != 0 {
		t.Fatalf("範囲判定失敗: %d %+v", code, result)
	}
	wantPaths := []string{"src/add", "src/delete", "src/modify", "src/type", "src/type/child", "src2/modify", "新規 空白/日本語.txt"}
	wantKinds := []string{"added", "deleted", "modified", "type_changed", "added", "modified", "added"}
	if len(result.ChangesSinceBegin) != len(wantPaths) {
		t.Fatalf("差分数が不正: %+v", result.ChangesSinceBegin)
	}
	for i, change := range result.ChangesSinceBegin {
		if change.Path != wantPaths[i] || change.Kind != wantKinds[i] || change.WithinScope != (i != 5) {
			t.Fatalf("差分が不正: %+v", change)
		}
	}
}

func TestCheckFileAllowDoesNotExpandAfterTypeChange(t *testing.T) {
	dir := repo(t)
	beginForCheck(t, dir, "tracked.txt")
	if err := os.Remove(filepath.Join(dir, "tracked.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "tracked.txt/child", "配下は許容していない")
	result, code := Check(dir)
	if code != 1 || len(result.ChangesSinceBegin) != 2 || !result.ChangesSinceBegin[0].WithinScope || result.ChangesSinceBegin[1].WithinScope {
		t.Fatalf("ファイルの許容範囲が広がりました: %d %+v", code, result)
	}
}

func TestCheckReportsIndexOnlyChanges(t *testing.T) {
	dir := repo(t)
	original, _ := os.ReadFile(filepath.Join(dir, "tracked.txt"))
	beginForCheck(t, dir, "future")
	write(t, dir, "tracked.txt", "ステージだけに残す本文")
	testGit(t, dir, "add", "tracked.txt")
	write(t, dir, "tracked.txt", string(original))
	result, code := Check(dir)
	if code != 1 || len(result.ChangesSinceBegin) != 0 || len(result.IndexChanges) != 1 || len(result.OutOfScopeChanges) != 1 {
		t.Fatalf("インデックスだけの変化を検出できません: %d %+v", code, result)
	}
	change := result.IndexChanges[0]
	if change.Kind != "modified" || change.WithinScope || len(change.Before) != 1 || len(change.After) != 1 || change.Before[0].Object == change.After[0].Object || change.Source != "index" {
		t.Fatalf("インデックス差分が不正: %+v", change)
	}
}

func TestCheckIndexDeletionAdditionAndConflictStages(t *testing.T) {
	dir := repo(t)
	record := beginForCheck(t, dir, ".")
	testGit(t, dir, "rm", "--cached", "tracked.txt")
	result, code := Check(dir)
	if code != 0 || len(result.ChangesSinceBegin) != 0 || len(result.IndexChanges) != 1 || result.IndexChanges[0].Kind != "deleted" || len(result.IndexChanges[0].After) != 0 {
		t.Fatalf("追跡解除を誤判定: %d %+v", code, result)
	}
	write(t, dir, "added.txt", "新規")
	testGit(t, dir, "add", "added.txt")
	if _, err := git(dir, []byte("100644 "+record.Index[0].Object+" 1\ttracked.txt\n100644 "+record.Index[0].Object+" 2\ttracked.txt\n"), "update-index", "--index-info"); err != nil {
		t.Fatal(err)
	}
	result, code = Check(dir)
	if code != 0 || len(result.IndexChanges) != 2 || result.IndexChanges[0].Kind != "added" || len(result.IndexChanges[1].After) != 2 || result.IndexChanges[1].After[1].Stage != 2 {
		t.Fatalf("追加・競合段階を検出できません: %d %+v", code, result)
	}
}

func TestCheckUnknownAndOutOfScopeCoexist(t *testing.T) {
	for _, kind := range []string{"head", "submodule"} {
		t.Run(kind, func(t *testing.T) {
			dir := repo(t)
			record := beginForCheck(t, dir, "future")
			wantIssue := "head_changed"
			if kind == "head" {
				testGit(t, dir, "-c", "user.name=workscope-test", "-c", "user.email=test@example.invalid", "-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "HEAD変更")
			} else {
				wantIssue = "unsupported_submodule"
				testGit(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+record.Head+",module")
				write(t, dir, "module/hidden.txt", "対象外")
			}
			write(t, dir, "tracked.txt", "範囲外の編集")
			result, code := Check(dir)
			requireFailure(t, result, code, wantIssue)
			if code != 2 || result.Coverage.Complete || len(result.ChangesSinceBegin) != 1 || len(result.OutOfScopeChanges) == 0 || result.ChangesSinceBegin[0].Path != "tracked.txt" {
				t.Fatalf("判定不能と範囲外を保持できません: %d %+v", code, result)
			}
			if kind == "submodule" && !reflect.DeepEqual(result.Coverage.Submodules, []string{"module"}) {
				t.Fatalf("対象外のサブモジュールが不明: %+v", result)
			}
		})
	}
}

func TestCheckRejectsMissingAndInvalidBaseline(t *testing.T) {
	dir := repo(t)
	result, code := Check(dir)
	requireFailure(t, result, code, "baseline_missing")
	if _, err := os.Stat(filepath.Join(dir, stateName)); !os.IsNotExist(err) {
		t.Fatal("照合が開始記録を作成しました")
	}
	beginForCheck(t, dir, ".")
	record, _ := readRecord(t, dir)
	for _, kind := range []string{"broken", "schema", "root", "files", "index", "allow", "hash", "path", "date"} {
		t.Run(kind, func(t *testing.T) {
			data, _ := json.Marshal(record)
			var changed Record
			json.Unmarshal(data, &changed)
			switch kind {
			case "schema":
				changed.SchemaVersion++
			case "root":
				changed.Root = t.TempDir()
			case "files":
				changed.Files = nil
			case "index":
				changed.Index = nil
			case "allow":
				changed.Allow[0].Kind = "unknown"
			case "hash":
				changed.Files[0].SHA256 = "invalid"
			case "path":
				changed.Files[0].Path = "../outside"
			case "date":
				changed.CreatedAt = "不正"
			}
			data, _ = json.Marshal(changed)
			if kind == "broken" {
				data = []byte("{")
			}
			write(t, dir, stateName+"/"+recordName, string(data))
			result, code := Check(dir)
			requireFailure(t, result, code, "foreign_state")
			after, err := os.ReadFile(filepath.Join(dir, stateName, recordName))
			if err != nil || !bytes.Equal(data, after) {
				t.Fatal("不正な開始記録が変更されました")
			}
		})
	}
}

func TestCheckFromWorktreeSubdirectory(t *testing.T) {
	dir := repo(t)
	worktree := filepath.Join(t.TempDir(), "追加 作業ツリー")
	testGit(t, dir, "worktree", "add", "-qb", "check-worktree", worktree)
	write(t, worktree, "sub/日本語.txt", "開始時")
	beginForCheck(t, worktree, "sub")
	write(t, worktree, "sub/日本語.txt", "更新")
	result, code := Check(filepath.Join(worktree, "sub"))
	if code != 0 || len(result.ChangesSinceBegin) != 1 || result.ChangesSinceBegin[0].Path != "sub/日本語.txt" || !result.ChangesSinceBegin[0].WithinScope {
		t.Fatalf("作業ツリーの照合失敗: %d %+v", code, result)
	}
}

func TestCheckLinkTargetChangesWithoutFollowingContents(t *testing.T) {
	dir := repo(t)
	out1, out2 := t.TempDir(), t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink(out1, link); err != nil {
		t.Skipf("この環境ではリンクを作成できません: %v", err)
	}
	beginForCheck(t, dir, "link")
	write(t, out1, "秘密.txt", "リンク先の変化は対象外")
	result, code := Check(dir)
	if code != 0 || len(result.ChangesSinceBegin) != 0 {
		t.Fatalf("リンク先の内容を照合しました: %d %+v", code, result)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out2, link); err != nil {
		t.Fatal(err)
	}
	result, code = Check(dir)
	if code != 0 || len(result.ChangesSinceBegin) != 1 || result.ChangesSinceBegin[0].Kind != "modified" || result.ChangesSinceBegin[0].BeforeSHA256 == result.ChangesSinceBegin[0].AfterSHA256 {
		t.Fatalf("リンク先の文字列変更を検出できません: %d %+v", code, result)
	}
}

func TestCheckReportsNewParentLinkAlongsideUnreadableDescendant(t *testing.T) {
	dir := repo(t)
	outside := t.TempDir()
	// 作成権限を先に確認し、未確認の環境で実行済みと扱わない。
	probe := filepath.Join(dir, "probe")
	if err := os.Symlink(outside, probe); err != nil {
		t.Skipf("この環境ではリンクを作成できません: %v", err)
	}
	if err := os.Remove(probe); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "dir/child", "開始時")
	beginForCheck(t, dir, "future")
	if err := os.Remove(filepath.Join(dir, "dir", "child")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "dir")); err != nil {
		t.Fatal(err)
	}
	result, code := Check(dir)
	requireFailure(t, result, code, "unsafe_parent")
	if code != 2 || len(result.ChangesSinceBegin) != 1 || result.ChangesSinceBegin[0].Path != "dir" || result.ChangesSinceBegin[0].AfterKind != "symlink" || len(result.OutOfScopeChanges) != 1 {
		t.Fatalf("取得できた親リンクの変化が失われました: %d %+v", code, result)
	}
}

func TestCheckDoesNotTreatNewlyIgnoredBaselinePathAsDeleted(t *testing.T) {
	dir := repo(t)
	write(t, dir, "untracked.txt", "開始時")
	beginForCheck(t, dir, ".gitignore")
	write(t, dir, ".gitignore", "untracked.txt\n")
	write(t, dir, "untracked.txt", "無視対象になった本文")
	result, code := Check(dir)
	requireFailure(t, result, code, "coverage_changed")
	if code != 2 || len(result.ChangesSinceBegin) != 1 || result.ChangesSinceBegin[0].Path != ".gitignore" {
		t.Fatalf("無視対象への変化を削除や変更なしと扱いました: %d %+v", code, result)
	}
}
