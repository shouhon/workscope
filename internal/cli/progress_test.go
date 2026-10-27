package cli

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"workscope/internal/baseline"
)

type progressWriter struct {
	bytes.Buffer
	once sync.Once
	seen chan struct{}
}

func (w *progressWriter) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	w.once.Do(func() { close(w.seen) })
	return n, err
}

func TestProgressRemainsVisibleWhileFileReadWaits(t *testing.T) {
	writer := &progressWriter{seen: make(chan struct{})}
	_, code := withProgress(writer, func(observer baseline.Observer) (baseline.Result, int) {
		observer(baseline.Progress{Stage: "ファイル走査", Files: 37, Pass: 2})
		// ファイルの読み取りが進まない間も、表示側が現在の状態を通知する。
		select {
		case <-writer.seen:
		case <-time.After(5 * time.Second):
			t.Fatal("待機中に進捗が表示されませんでした")
		}
		return baseline.NewResult(), 2
	})
	if code != 2 || !strings.Contains(writer.String(), "進捗: ファイル走査 | 確認済みファイル: 37件 | 取得: 2/2回目 | 経過:") || !strings.Contains(writer.String(), "進捗: 処理終了") {
		t.Fatalf("走査中または失敗終了時の進捗が不正: %d %s", code, writer.String())
	}
}
