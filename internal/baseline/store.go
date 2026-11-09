package baseline

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const stateName = ".workscope"
const recordName = "baseline.json"

// RecordName はタスクの開始記録名を返す。タスク名を直接パスとして使わない。
func RecordName(task string) string {
	if task == "" {
		return recordName
	}
	hash := sha256.Sum256([]byte(task))
	return "task-" + hex.EncodeToString(hash[:]) + ".json"
}

func validTask(task string) bool {
	return utf8.ValidString(task) && strings.TrimSpace(task) == task && !strings.ContainsAny(task, `/\`) && !strings.ContainsFunc(task, unicode.IsControl)
}

const invalidTaskMessage = "タスク名には前後の空白、パス区切り、制御文字を使えません。"

func token() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func (r *repository) existing(task string) ([]byte, error) {
	selected, _, err := r.existingRecords(task)
	return selected, err
}

func (r *repository) existingRecords(task string) ([]byte, []RecordSummary, error) {
	state, err := r.loadState(task, false)
	if state.handle != nil {
		defer state.handle.Close()
	}
	return state.data, state.records, err
}

type stateSnapshot struct {
	data      []byte
	records   []RecordSummary
	directory os.FileInfo
	handle    *os.File
}

func directoryIdentity(dir *os.Root) (os.FileInfo, error) {
	file, err := dir.Open(".")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Windowsでも実体情報を遅延参照せず、開いたハンドルから取得する。
	return file.Stat()
}

func (r *repository) sameStateDirectory(expected os.FileInfo) bool {
	info, err := r.fs.Lstat(stateName)
	if err != nil || info.Mode().Type() != os.ModeDir {
		return false
	}
	file, err := r.fs.Open(stateName)
	if err != nil {
		return false
	}
	defer file.Close()
	current, err := file.Stat()
	return err == nil && os.SameFile(expected, current)
}

func (r *repository) loadState(task string, allowEmpty bool) (stateSnapshot, error) {
	state := stateSnapshot{records: []RecordSummary{}}
	info, err := r.fs.Lstat(stateName)
	if errors.Is(err, fs.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fail("state_unreadable", "状態保存先を参照できません。", stateName)
	}
	if info.Mode().Type() != os.ModeDir {
		return state, fail("foreign_state", ".workscope が通常の専用ディレクトリではありません。", stateName)
	}
	dir, err := r.fs.OpenRoot(stateName)
	if err != nil {
		return state, fail("state_unreadable", "状態保存先を開けません。", stateName)
	}
	defer dir.Close()
	state.handle, err = dir.Open(".")
	if err != nil {
		return state, fail("state_unreadable", "状態保存先の実体を開けません。", stateName)
	}
	// 保存までハンドルを保持し、削除後に同じ識別子が再利用されることを防ぐ。
	state.directory, err = state.handle.Stat()
	if err != nil {
		return state, fail("state_unreadable", "状態保存先の実体を確認できません。", stateName)
	}
	if !r.sameStateDirectory(state.directory) {
		return state, fail("state_changed", "確認中に状態保存先が変化しました。", stateName)
	}
	state.data, state.records, err = r.readState(dir, task, false, allowEmpty, "")
	return state, err
}

// 保存時だけ、自分が作成したロックと一時ファイルを検査対象から除く。
func (r *repository) readState(dir *os.Root, task string, locked, allowEmpty bool, temporary string) ([]byte, []RecordSummary, error) {
	file, err := dir.Open(".")
	if err != nil {
		return nil, nil, fail("state_unreadable", "状態保存先を読み取れません。", stateName)
	}
	children, err := file.ReadDir(-1)
	file.Close()
	if err != nil {
		return nil, nil, fail("state_unreadable", "状態保存先の一覧を読み取れません。", stateName)
	}
	var selected []byte
	records := []RecordSummary{}
	if len(children) == 0 && !allowEmpty {
		return nil, nil, fail("foreign_state", "既存の .workscope は空で、workscope用の開始記録を確認できません。保存先は変更していません。通常の空ディレクトリは begin --allow <対象> で利用できます。次の編集前に begin を実行し、成功を確認してください。編集後に作った記録で過去の変更を検証済みにしないでください。", stateName)
	}
	for _, child := range children {
		name := child.Name()
		if locked && (name == "record.lock" || name == temporary) {
			continue
		}
		if name != recordName && !(strings.HasPrefix(name, "task-") && strings.HasSuffix(name, ".json") && hexValue(strings.TrimSuffix(strings.TrimPrefix(name, "task-"), ".json"), 64)) {
			return nil, nil, fail("foreign_state", "状態保存先に開始記録以外のファイルがあります。既存の内容を確認してください。", stateName)
		}
		info, err := dir.Lstat(name)
		if err != nil {
			return nil, nil, fail("state_unreadable", "開始記録を参照できません。", stateName+"/"+name)
		}
		if !info.Mode().IsRegular() {
			return nil, nil, fail("foreign_state", "workscopeの正常な開始記録を確認できません。", stateName+"/"+name)
		}
		data, err := dir.ReadFile(name)
		if err != nil {
			return nil, nil, fail("state_unreadable", "開始記録を読み取れません。", stateName+"/"+name)
		}
		var record Record
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		err = decoder.Decode(&record)
		var extra any
		if err != nil || decoder.Decode(&extra) != io.EOF || !r.validRecord(record) || !validTask(record.Task) || RecordName(record.Task) != name {
			return nil, nil, fail("foreign_state", "開始記録の識別情報・保存形式・ルート・タスク名が一致しません。既存の記録を保護して停止します。", stateName+"/"+name)
		}
		if name == RecordName(task) {
			selected = data
		}
		records = append(records, RecordSummary{Task: record.Task, Default: record.Task == "", BaselineID: record.ID, CreatedAt: record.CreatedAt, AllowCount: len(record.Allow)})
	}
	if len(records) == 0 && !allowEmpty {
		return nil, nil, fail("foreign_state", "workscopeの正常な開始記録を確認できません。", stateName)
	}
	return selected, records, nil
}

func (r *repository) save(record Record, previous stateSnapshot) error {
	created := false
	if previous.directory == nil {
		if err := r.fs.Mkdir(stateName, 0700); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return fail("state_changed", "保存前に状態保存先が作成されました。既存の内容は変更しません。", stateName)
			}
			return fail("save_failed", "専用の状態保存先を作成できません。既存の内容は変更しません。", stateName)
		}
		created = true
	}
	info, err := r.fs.Lstat(stateName)
	if err != nil || info.Mode().Type() != os.ModeDir {
		return fail("state_changed", "状態保存先が変化しました。", stateName)
	}
	dir, err := r.fs.OpenRoot(stateName)
	if err != nil {
		return fail("save_failed", "状態保存先を開けません。", stateName)
	}
	defer dir.Close()
	identity, err := directoryIdentity(dir)
	if err != nil {
		return fail("save_failed", "状態保存先の実体を確認できません。", stateName)
	}
	if previous.directory != nil && !os.SameFile(previous.directory, identity) || !r.sameStateDirectory(identity) {
		return fail("state_changed", "状態保存先の実体が変化しました。", stateName)
	}
	// 初回保存の失敗時だけ、自分が作った同じ空ディレクトリを除去する。
	defer func() {
		if created && r.sameStateDirectory(identity) {
			r.fs.Remove(stateName)
		}
	}()
	lock, err := dir.OpenFile("record.lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fail("save_failed", "別の記録処理が実行中か、状態保存先へ書き込めません。", stateName)
	}
	lock.Close()
	defer dir.Remove("record.lock")
	name := RecordName(record.Task)
	confirm := func(temporary string) error {
		current, records, err := r.readState(dir, record.Task, true, len(previous.records) == 0, temporary)
		if err != nil || !bytes.Equal(current, previous.data) || len(previous.records) == 0 && len(records) != 0 || !r.sameStateDirectory(identity) {
			return fail("state_changed", "保存前に開始記録か状態保存先が変化しました。", stateName+"/"+name)
		}
		return nil
	}
	if err := confirm(""); err != nil {
		return err
	}
	temp := ".baseline-" + token() + ".tmp"
	file, err := dir.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fail("save_failed", "開始記録の一時ファイルを作成できません。", stateName)
	}
	defer dir.Remove(temp)
	defer file.Close()
	err = json.NewEncoder(file).Encode(record)
	if err == nil {
		err = file.Sync()
	}
	if err != nil {
		return fail("save_failed", "開始記録の書き込みに失敗しました。以前の記録を保持します。", stateName)
	}
	if err := confirm(temp); err != nil {
		return err
	}
	if previous.data == nil {
		// 新規記録は、最終確認の後に同名ファイルが作られても上書きしない。
		if err := publishNewRecord(dir, file, temp, name); err != nil {
			return fail("save_failed", "新しい開始記録を確定できません。既存の内容は変更しません。", stateName+"/"+name)
		}
		if err := file.Close(); err != nil {
			return fail("save_failed", "確定した開始記録を閉じられません。保存先を確認してください。", stateName+"/"+name)
		}
		if !r.sameStateDirectory(identity) {
			return fail("state_changed", "確定中に状態保存先の実体が変化しました。", stateName)
		}
		return nil
	}
	// 既存ファイルを先に削除しない。置換に失敗しても以前の記録を保持する。
	if err := dir.Rename(temp, name); err != nil {
		return fail("save_failed", "開始記録の置換に失敗しました。以前の記録を保持します。", stateName)
	}
	if err := file.Close(); err != nil {
		return fail("save_failed", "確定した開始記録を閉じられません。保存先を確認してください。", stateName+"/"+name)
	}
	if !r.sameStateDirectory(identity) {
		return fail("state_changed", "確定中に状態保存先の実体が変化しました。", stateName)
	}
	return nil
}

// Begin は現在の作業ディレクトリのGitルートに、本文を含まない開始記録を保存する。
func Begin(dir string, names []string, replace bool, task string, observers ...Observer) (Result, int) {
	return BeginWithOptions(dir, names, replace, task, Options{}, observers...)
}

// BeginWithOptions は今回だけのGit設定で開始記録を保存する。
func BeginWithOptions(dir string, names []string, replace bool, task string, options Options, observers ...Observer) (Result, int) {
	result := NewResult()
	result.Task = task
	if !validTask(task) {
		result.Status = "invalid_arguments"
		result.Coverage.Complete = false
		result.Issues = append(result.Issues, Issue{Code: "invalid_task", Message: invalidTaskMessage})
		return result, 64
	}
	if len(names) == 0 {
		result.Status = "invalid_arguments"
		result.Coverage.Complete = false
		result.Issues = append(result.Issues, Issue{Code: "missing_allow", Message: "--allow を1つ以上指定してください。"})
		return result, 64
	}
	progress := newProgress(observers)
	err := func() error {
		progress.stage("Git作業場所の確認")
		r, err := openRepositoryWithOptions(dir, options, progress)
		if err != nil {
			return err
		}
		defer r.fs.Close()
		result.Root = r.root
		progress.stage("許容範囲の確認")
		allow, err := r.normalizeAllow(names)
		if err != nil {
			return err
		}
		progress.stage("開始記録の確認")
		previous, err := r.loadState(task, true)
		if previous.handle != nil {
			defer previous.handle.Close()
		}
		if err != nil {
			return err
		}
		if previous.data != nil && !replace {
			return fail("baseline_exists", "開始記録が既にあります。取り直す場合は --replace を指定してください。", stateName+"/"+RecordName(task))
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
		progress.stage("許容範囲の再確認")
		confirmed, err := r.normalizeAllow(names)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(allow, confirmed) {
			return fail("snapshot_changed", "記録中に許容対象の種類や実体が変化しました。", "")
		}
		record := Record{SchemaVersion: SchemaVersion, Tool: "workscope", ID: token(), Task: task, Root: r.root, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Head: snapshot.Head, Allow: allow, Coverage: result.Coverage, PreexistingChanges: snapshot.Changes, Files: snapshot.Files, Index: snapshot.Index}
		progress.stage("開始記録の保存")
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
