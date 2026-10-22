package baseline

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"reflect"
	"time"
)

const stateName = ".workscope"
const recordName = "baseline.json"

func token() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (r *repository) existing() ([]byte, error) {
	info, err := r.fs.Lstat(stateName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fail("foreign_state", ".workscope が通常の専用ディレクトリではありません。", stateName)
	}
	dir, err := r.fs.OpenRoot(stateName)
	if err != nil {
		return nil, fail("state_unreadable", "状態保存先を開けません。", stateName)
	}
	defer dir.Close()
	file, err := dir.Open(".")
	if err != nil {
		return nil, err
	}
	children, err := file.ReadDir(-1)
	file.Close()
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if child.Name() != recordName {
			return nil, fail("foreign_state", "状態保存先に開始記録以外のファイルがあります。既存の内容を確認してください。", stateName)
		}
	}
	info, err = dir.Lstat(recordName)
	if err != nil || !info.Mode().IsRegular() {
		return nil, fail("foreign_state", "workscopeの正常な開始記録を確認できません。", stateName+"/"+recordName)
	}
	data, err := dir.ReadFile(recordName)
	if err != nil {
		return nil, fail("state_unreadable", "開始記録を読み取れません。", stateName+"/"+recordName)
	}
	var record Record
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&record)
	var extra any
	if err != nil || decoder.Decode(&extra) != io.EOF || !r.validRecord(record) {
		return nil, fail("foreign_state", "開始記録の識別情報・保存形式・ルートが一致しません。既存の記録を保護して停止します。", stateName+"/"+recordName)
	}
	return data, nil
}

func (r *repository) save(record Record, previous []byte) error {
	created := false
	if previous == nil {
		if err := r.fs.Mkdir(stateName, 0700); err != nil {
			return fail("save_failed", "専用の状態保存先を作成できません。既存の内容は変更しません。", stateName)
		}
		created = true
	}
	// 初回保存の失敗時だけ、自分が作った空ディレクトリを除去する。
	defer func() {
		if created {
			r.fs.Remove(stateName)
		}
	}()
	info, err := r.fs.Lstat(stateName)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fail("save_failed", "状態保存先が変化しました。", stateName)
	}
	dir, err := r.fs.OpenRoot(stateName)
	if err != nil {
		return fail("save_failed", "状態保存先を開けません。", stateName)
	}
	defer dir.Close()
	lock, err := dir.OpenFile("record.lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fail("save_failed", "別の記録処理が実行中か、状態保存先へ書き込めません。", stateName)
	}
	lock.Close()
	defer dir.Remove("record.lock")
	if previous != nil {
		currentInfo, statErr := dir.Lstat(recordName)
		if statErr != nil || !currentInfo.Mode().IsRegular() {
			return fail("state_changed", "保存前に開始記録の種類が変化しました。", stateName+"/"+recordName)
		}
		current, readErr := dir.ReadFile(recordName)
		if readErr != nil || !bytes.Equal(current, previous) {
			return fail("state_changed", "保存前に開始記録が変化しました。", stateName+"/"+recordName)
		}
	}
	temp := ".baseline-" + token() + ".tmp"
	file, err := dir.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fail("save_failed", "開始記録の一時ファイルを作成できません。", stateName)
	}
	defer dir.Remove(temp)
	err = json.NewEncoder(file).Encode(record)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return fail("save_failed", "開始記録の書き込みに失敗しました。以前の記録を保持します。", stateName)
	}
	// 既存ファイルを先に削除しない。置換に失敗しても以前の記録を保持する。
	if err := dir.Rename(temp, recordName); err != nil {
		return fail("save_failed", "開始記録の置換に失敗しました。以前の記録を保持します。", stateName)
	}
	return nil
}

// Begin は現在の作業ディレクトリのGitルートに、本文を含まない開始記録を保存する。
func Begin(dir string, names []string, replace bool) (Result, int) {
	result := NewResult()
	if len(names) == 0 {
		result.Status = "invalid_arguments"
		result.Coverage.Complete = false
		result.Issues = append(result.Issues, Issue{Code: "missing_allow", Message: "--allow を1つ以上指定してください。"})
		return result, 64
	}
	err := func() error {
		r, err := openRepository(dir)
		if err != nil {
			return err
		}
		defer r.fs.Close()
		result.Root = r.root
		allow, err := r.normalizeAllow(names)
		if err != nil {
			return err
		}
		previous, err := r.existing()
		if err != nil {
			return err
		}
		if previous != nil && !replace {
			return fail("baseline_exists", "開始記録が既にあります。取り直す場合は --replace を指定してください。", stateName+"/"+recordName)
		}
		snapshot, err := r.stableSnapshot(nil)
		if snapshot.Submodules != nil {
			result.Coverage.Submodules = snapshot.Submodules
		}
		if err != nil {
			return err
		}
		if len(snapshot.Issues) != 0 {
			return &failure{issue: snapshot.Issues[0]}
		}
		confirmed, err := r.normalizeAllow(names)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(allow, confirmed) {
			return fail("snapshot_changed", "記録中に許容対象の種類や実体が変化しました。", "")
		}
		record := Record{SchemaVersion: SchemaVersion, Tool: "workscope", ID: token(), Root: r.root, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Head: snapshot.Head, Allow: allow, Coverage: result.Coverage, PreexistingChanges: snapshot.Changes, Files: snapshot.Files, Index: snapshot.Index}
		if err := r.save(record, previous); err != nil {
			return err
		}
		result.Status = "recorded"
		result.BaselineID = record.ID
		result.PreexistingChanges = record.PreexistingChanges
		return nil
	}()
	if err == nil {
		return result, 0
	}
	result.Coverage.Complete = false
	result.Issues = append(result.Issues, describeError(err))
	var f *failure
	if errors.As(err, &f) && f.invalid {
		result.Status = "invalid_arguments"
		return result, 64
	}
	return result, 2
}
