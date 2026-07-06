package logx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Entry 是提供给前端出口的一条结构化日志。
type Entry struct {
	Time  time.Time `json:"time"`
	Level string    `json:"level"`
	Mod   string    `json:"mod"`
	Msg   string    `json:"msg"`
}

// ringSink 是一个有界环形缓冲，保留最近 N 条日志，并向 SSE 订阅者广播。
type ringSink struct {
	mu   sync.RWMutex
	buf  []Entry
	size int
	subs map[chan Entry]struct{}
}

func newRingSink(size int) *ringSink {
	if size <= 0 {
		size = 500
	}
	return &ringSink{size: size, subs: make(map[chan Entry]struct{})}
}

// push 追加一条日志：超出容量丢弃最旧，并非阻塞地广播给订阅者。
func (s *ringSink) push(e Entry) {
	s.mu.Lock()
	s.buf = append(s.buf, e)
	if len(s.buf) > s.size {
		s.buf = s.buf[len(s.buf)-s.size:]
	}
	for ch := range s.subs {
		select {
		case ch <- e:
		default: // 订阅者消费不过来则丢弃该条，绝不阻塞日志主流程
		}
	}
	s.mu.Unlock()
}

// recent 返回当前缓冲快照。
func (s *ringSink) recent() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, len(s.buf))
	copy(out, s.buf)
	return out
}

func (s *ringSink) subscribe() chan Entry {
	ch := make(chan Entry, 64)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *ringSink) unsubscribe(ch chan Entry) {
	s.mu.Lock()
	delete(s.subs, ch)
	s.mu.Unlock()
}

func (s *ringSink) handler(level slog.Level) slog.Handler {
	return &sinkHandler{sink: s, level: level}
}

// sinkHandler 把 slog 记录转成 Entry 压入环形缓冲。
type sinkHandler struct {
	sink  *ringSink
	level slog.Level
	mod   string
	group string
}

func (h *sinkHandler) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }

func (h *sinkHandler) Handle(_ context.Context, r slog.Record) error {
	e := Entry{Time: r.Time, Level: r.Level.String(), Msg: r.Message, Mod: h.mod}
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "mod" {
			e.Mod = a.Value.String()
			return false
		}
		return true
	})
	h.sink.push(e)
	return nil
}

func (h *sinkHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	for _, a := range attrs {
		if a.Key == "mod" {
			next.mod = a.Value.String()
		}
	}
	return &next
}

func (h *sinkHandler) WithGroup(name string) slog.Handler {
	next := *h
	next.group = name
	return &next
}

// SyslogHandler 返回 GET /api/syslog 处理器：一次性返回最近的日志快照。
func SyslogHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		s := sink
		mu.RUnlock()
		var data []Entry
		if s != nil {
			data = s.recent()
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}
}

// SyslogStreamHandler 返回 GET /api/syslog/stream 处理器：以 SSE 实时推送新日志。
func SyslogStreamHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		s := sink
		mu.RUnlock()
		fl, ok := w.(http.Flusher)
		if s == nil || !ok {
			http.Error(w, "SSE 不可用", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		ch := s.subscribe()
		defer s.unsubscribe(ch)

		// 先补发当前快照，让新连接立即看到最近日志。
		for _, e := range s.recent() {
			writeSSE(w, e)
		}
		fl.Flush()

		ping := time.NewTicker(25 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case e := <-ch:
				writeSSE(w, e)
				fl.Flush()
			case <-ping.C:
				_, _ = w.Write([]byte(": ping\n\n")) // 心跳，维持连接
				fl.Flush()
			}
		}
	}
}

func writeSSE(w http.ResponseWriter, e Entry) {
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n\n"))
}
