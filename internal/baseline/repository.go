package baseline

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

type repository struct {
	root     string
	fs       *os.Root
	excluded []string
}

func git(dir string, input []byte, args ...string) ([]byte, error) {
	base := []string{"--no-optional-locks", "-C", dir, "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false"}
	cmd := exec.Command("git", append(base, args...)...)
	// 呼出元のGIT_DIRや代替インデックスで、別の作業領域を記録しない。
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(strings.ToUpper(key), "GIT_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, &gitError{cause: err, operation: gitOperation(args), stderr: safeGitStderr(stderr.String())}
	}
	return out, err
}

func gitLine(dir string, args ...string) (string, error) {
	out, err := git(dir, nil, args...)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(out) || !bytes.HasSuffix(out, []byte{'\n'}) {
		return "", fmt.Errorf("Git出力の形式が不正です。失敗したGit操作: %s", gitOperation(args))
	}
	return strings.TrimSuffix(strings.TrimSuffix(string(out), "\n"), "\r"), nil
}

func openRepository(dir string) (*repository, error) {
	root, err := gitLine(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, failGit("git_unavailable", "Gitの作業ディレクトリを特定できません。", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fail("root_unreadable", "Gitルートの実体を確認できません。", "")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	fs, err := os.OpenRoot(root)
	if err != nil {
		return nil, fail("root_unreadable", "Gitルートを開けません。", "")
	}
	r := &repository{root: root, fs: fs, excluded: []string{".workscope"}}
	for _, arg := range []string{"--absolute-git-dir", "--git-common-dir"} {
		name, err := gitLine(root, "rev-parse", "--path-format=absolute", arg)
		if err != nil {
			fs.Close()
			return nil, failGit("git_unavailable", "Gitの管理領域を特定できません。", err)
		}
		name, err = filepath.EvalSymlinks(name)
		if err != nil {
			fs.Close()
			return nil, fail("git_unavailable", "Gitの管理領域の実体を確認できません。", "")
		}
		if rel, err := filepath.Rel(root, name); err == nil && filepath.IsLocal(rel) {
			r.excluded = append(r.excluded, filepath.ToSlash(rel))
		}
	}
	return r, nil
}

func (r *repository) head() (string, error) {
	head, err := gitLine(r.root, "rev-parse", "--verify", "HEAD")
	if err == nil {
		return head, nil
	}
	// コミットがまだないブランチだけは空のHEADとして扱う。
	ref, symErr := gitLine(r.root, "symbolic-ref", "-q", "HEAD")
	if symErr != nil {
		return "", failGit("head_unreadable", "HEADを読み取れません。", errors.Join(err, symErr))
	}
	_, refErr := git(r.root, nil, "show-ref", "--verify", "--quiet", ref)
	var e *exec.ExitError
	if errors.As(refErr, &e) && e.ExitCode() == 1 {
		return "", nil
	}
	if refErr != nil {
		return "", failGit("head_unreadable", "HEADを読み取れません。", errors.Join(err, refErr))
	}
	return "", failGit("head_unreadable", "HEADを読み取れません。", err)
}

func splitNUL(data []byte) ([]string, error) {
	if len(data) == 0 {
		return []string{}, nil
	}
	if data[len(data)-1] != 0 || !utf8.Valid(data) {
		return nil, fmt.Errorf("NUL区切りのGit出力が不正です")
	}
	return strings.Split(string(data[:len(data)-1]), "\x00"), nil
}

func (r *repository) index() ([]IndexEntry, error) {
	data, err := git(r.root, nil, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, failGit("index_unreadable", "インデックスを読み取れません。", err)
	}
	return parseIndex(data)
}

func parseIndex(data []byte) ([]IndexEntry, error) {
	rows, err := splitNUL(data)
	if err != nil {
		return nil, err
	}
	entries := []IndexEntry{}
	for _, row := range rows {
		meta, name, ok := strings.Cut(row, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || !validGitPath(name) {
			return nil, fmt.Errorf("インデックスの形式が不正です")
		}
		stage, err := strconv.Atoi(fields[2])
		if err != nil || stage < 0 || stage > 3 {
			return nil, fmt.Errorf("インデックスの段階が不正です")
		}
		entries = append(entries, IndexEntry{name, fields[0], fields[1], stage})
	}
	return entries, nil
}

func parseStatus(data []byte) ([]GitChange, error) {
	rows, err := splitNUL(data)
	if err != nil {
		return nil, err
	}
	changes := []GitChange{}
	for _, row := range rows {
		if len(row) < 4 || row[2] != ' ' || !validGitPath(row[3:]) || strings.ContainsAny(row[:2], "RC") {
			return nil, fmt.Errorf("Git状態の形式が不正です")
		}
		changes = append(changes, GitChange{row[3:], row[:2]})
	}
	return changes, nil
}

func validGitPath(name string) bool {
	return name != "." && filepath.IsLocal(filepath.FromSlash(name)) && filepath.ToSlash(filepath.Clean(filepath.FromSlash(name))) == name && !strings.Contains(name, "\\")
}

func (r *repository) ignored(names []string) (map[string]bool, error) {
	ignored := map[string]bool{}
	if len(names) == 0 {
		return ignored, nil
	}
	data, err := git(r.root, []byte(strings.Join(names, "\x00")+"\x00"), "check-ignore", "--no-index", "-z", "--stdin")
	var e *exec.ExitError
	if errors.As(err, &e) && e.ExitCode() == 1 {
		err = nil
	}
	if err != nil {
		return nil, failGit("ignore_unreadable", "Gitの無視設定を確認できません。", err)
	}
	rows, err := splitNUL(data)
	if err != nil {
		return nil, err
	}
	for _, name := range rows {
		ignored[name] = true
	}
	return ignored, nil
}
