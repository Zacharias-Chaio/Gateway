package logx

import (
	"testing"
	"time"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

func TestDailyRotateStopReturnsPromptly(t *testing.T) {
	stop := startDailyRotate(&lumberjack.Logger{})
	done := make(chan struct{})
	go func() {
		stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("daily rotation worker did not stop")
	}
}

func TestInitPreservesSinkAcrossReload(t *testing.T) {
	Init(Options{BufferSize: 10})
	mu.RLock()
	first := runtime.sink
	mu.RUnlock()

	Init(Options{BufferSize: 20})
	mu.RLock()
	second := runtime.sink
	mu.RUnlock()

	if first != second {
		t.Fatal("log sink was replaced during reload")
	}
	if second.size != 20 {
		t.Fatalf("sink size = %d, want 20", second.size)
	}
}
