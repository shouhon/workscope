package cli

import (
	"fmt"
	"io"
	"sync"
	"time"

	"workscope/internal/baseline"
)

func withProgress(stderr io.Writer, run func(baseline.Observer) (baseline.Result, int)) (baseline.Result, int) {
	var mu sync.Mutex
	value := baseline.Progress{Stage: "処理の準備"}
	started := time.Now()
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		reported := false
		write := func(finished bool) {
			mu.Lock()
			current := value
			mu.Unlock()
			if finished {
				current.Stage = "処理終了"
			}
			fmt.Fprintf(stderr, "進捗: %s | 確認済みファイル: %d件 | 取得: %d/2回目 | 経過: %.1f秒\n", current.Stage, current.Files, current.Pass, time.Since(started).Seconds())
		}
		for {
			select {
			case <-ticker.C:
				write(false)
				reported = true
			case <-stop:
				if reported {
					write(true)
				}
				return
			}
		}
	}()
	defer func() {
		close(stop)
		<-done
	}()
	return run(func(current baseline.Progress) {
		mu.Lock()
		value = current
		mu.Unlock()
	})
}
