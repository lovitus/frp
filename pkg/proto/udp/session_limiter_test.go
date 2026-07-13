package udp

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionLimiterConcurrentLimit(t *testing.T) {
	limiter := NewSessionLimiter(8)
	var wg sync.WaitGroup
	results := make(chan bool, 64)
	for range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- limiter.TryAcquire()
		}()
	}
	wg.Wait()
	close(results)

	acquired := 0
	for ok := range results {
		if ok {
			acquired++
		}
	}
	require.Equal(t, 8, acquired)
	require.Equal(t, 8, limiter.Active())
	for range acquired {
		limiter.Release()
	}
	require.Zero(t, limiter.Active())
}
