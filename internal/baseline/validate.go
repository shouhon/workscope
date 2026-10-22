package baseline

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"time"
)

func hexValue(value string, sizes ...int) bool {
	for _, size := range sizes {
		if len(value) == size {
			_, err := hex.DecodeString(value)
			return err == nil && value == strings.ToLower(value)
		}
	}
	return false
}

// 保存データの欠落や不正なパスを、変更なしという判定へ流さない。
func (r *repository) validRecord(record Record) bool {
	if record.Tool != "workscope" || record.SchemaVersion != SchemaVersion || !hexValue(record.ID, 32) || record.Root != r.root || len(record.Allow) == 0 || record.Files == nil || record.Index == nil || record.PreexistingChanges == nil || !reflect.DeepEqual(record.Coverage, DefaultCoverage()) {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, record.CreatedAt); err != nil {
		return false
	}
	if record.Head != "" && !hexValue(record.Head, 40, 64) {
		return false
	}
	validPath := func(name string) bool {
		return validGitPath(name) && !strings.ContainsRune(name, 0) && !r.protected(name)
	}
	seen := map[string]bool{}
	for _, allow := range record.Allow {
		if (!validPath(allow.Path) && !(allow.Path == "." && allow.Kind == "directory")) || seen[allow.Path] || (allow.Kind != "file" && allow.Kind != "directory" && allow.Kind != "future") {
			return false
		}
		seen[allow.Path] = true
	}
	files := map[string]bool{}
	for i, entry := range record.Files {
		if !validPath(entry.Path) || (i > 0 && record.Files[i-1].Path >= entry.Path) {
			return false
		}
		files[entry.Path] = true
		switch entry.Kind {
		case "file":
			if !hexValue(entry.SHA256, 64) || entry.Target != "" {
				return false
			}
		case "symlink":
			hash := sha256.Sum256([]byte(entry.Target))
			if entry.SHA256 != hex.EncodeToString(hash[:]) {
				return false
			}
		case "directory", "missing":
			if entry.SHA256 != "" || entry.Target != "" {
				return false
			}
		default:
			return false
		}
	}
	for i, entry := range record.Index {
		if !validPath(entry.Path) || !files[entry.Path] || !hexValue(entry.Object, 40, 64) || entry.Stage < 0 || entry.Stage > 3 || (entry.Mode != "100644" && entry.Mode != "100755" && entry.Mode != "120000") {
			return false
		}
		if i > 0 {
			previous := record.Index[i-1]
			if previous.Path > entry.Path || (previous.Path == entry.Path && previous.Stage >= entry.Stage) {
				return false
			}
		}
	}
	for i, change := range record.PreexistingChanges {
		if !validPath(change.Path) || (i > 0 && record.PreexistingChanges[i-1].Path >= change.Path) {
			return false
		}
		if change.Status != "??" && (len(change.Status) != 2 || change.Status == "  " || strings.Trim(change.Status, " MADUT") != "") {
			return false
		}
	}
	return true
}
