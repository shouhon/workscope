package baseline

// Progress は処理段階と、その回の取得で確認できたファイル数を表す。
// パスやファイル本文は含めない。
type Progress struct {
	Stage string
	Files int
	Pass  int
}

// Observer は実行中の状態を受け取る。呼び出しは処理と同じゴルーチンで行う。
type Observer func(Progress)

type progress struct {
	value  Progress
	report Observer
}

func newProgress(observers []Observer) *progress {
	if len(observers) == 0 || observers[0] == nil {
		return nil
	}
	return &progress{report: observers[0]}
}

func (p *progress) stage(stage string) {
	if p != nil {
		p.value.Stage = stage
		p.report(p.value)
	}
}

func (p *progress) pass(pass int) {
	if p != nil {
		p.value.Pass, p.value.Files = pass, 0
		p.stage("ファイル走査")
	}
}

func (p *progress) file(entry Entry) {
	if p != nil && entry.Kind != "directory" {
		p.value.Files++
		p.report(p.value)
	}
}

func (p *progress) git(args []string) func() {
	if p == nil {
		return func() {}
	}
	previous := p.value.Stage
	p.stage("Git待ち: " + gitOperation(args))
	return func() { p.stage(previous) }
}
