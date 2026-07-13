package udp

import "sync"

type SessionLimiter struct {
	mu     sync.Mutex
	limit  int
	active int
}

func NewSessionLimiter(limit int) *SessionLimiter {
	return &SessionLimiter{limit: limit}
}

func (l *SessionLimiter) TryAcquire() bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active >= l.limit {
		return false
	}
	l.active++
	return true
}

func (l *SessionLimiter) Release() {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.active > 0 {
		l.active--
	}
	l.mu.Unlock()
}

func (l *SessionLimiter) Active() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}
