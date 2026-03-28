// Copyright 2017 fatedier, fatedier@gmail.com
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package net

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatedier/frp/pkg/util/util"
)

type HTTPAuthMiddleware struct {
	user          string
	passwd        string
	authFailDelay time.Duration

	mu             sync.Mutex
	failedAttempts []time.Time
	lockUntil      time.Time
	maxFailures    int
	failureWindow  time.Duration
	lockDuration   time.Duration
}

func NewHTTPAuthMiddleware(user, passwd string) *HTTPAuthMiddleware {
	return &HTTPAuthMiddleware{
		user:          user,
		passwd:        passwd,
		maxFailures:   10,
		failureWindow: time.Minute,
		lockDuration:  10 * time.Second,
	}
}

func (authMid *HTTPAuthMiddleware) SetAuthFailDelay(delay time.Duration) *HTTPAuthMiddleware {
	authMid.authFailDelay = delay
	return authMid
}

func (authMid *HTTPAuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if locked, retryAfter := authMid.isLocked(); locked {
			authMid.writeLockedResponse(w, r, retryAfter)
			return
		}

		reqUser, reqPasswd, hasAuth := r.BasicAuth()
		if (authMid.user == "" && authMid.passwd == "") ||
			(hasAuth && util.ConstantTimeEqString(reqUser, authMid.user) &&
				util.ConstantTimeEqString(reqPasswd, authMid.passwd)) {
			authMid.resetFailures()
			next.ServeHTTP(w, r)
		} else {
			if locked, retryAfter := authMid.recordFailureAndMaybeLock(); locked {
				authMid.writeLockedResponse(w, r, retryAfter)
				return
			}
			if authMid.authFailDelay > 0 {
				time.Sleep(authMid.authFailDelay)
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		}
	})
}

func (authMid *HTTPAuthMiddleware) resetFailures() {
	authMid.mu.Lock()
	defer authMid.mu.Unlock()

	authMid.failedAttempts = nil
	authMid.lockUntil = time.Time{}
}

func (authMid *HTTPAuthMiddleware) isLocked() (bool, time.Duration) {
	authMid.mu.Lock()
	defer authMid.mu.Unlock()

	now := time.Now()
	if authMid.lockUntil.IsZero() || now.After(authMid.lockUntil) {
		if !authMid.lockUntil.IsZero() {
			authMid.failedAttempts = nil
			authMid.lockUntil = time.Time{}
		}
		return false, 0
	}
	return true, time.Until(authMid.lockUntil)
}

func (authMid *HTTPAuthMiddleware) recordFailureAndMaybeLock() (bool, time.Duration) {
	authMid.mu.Lock()
	defer authMid.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-authMid.failureWindow)
	filtered := authMid.failedAttempts[:0]
	for _, item := range authMid.failedAttempts {
		if item.After(cutoff) {
			filtered = append(filtered, item)
		}
	}
	filtered = append(filtered, now)
	authMid.failedAttempts = filtered

	if len(authMid.failedAttempts) < authMid.maxFailures {
		return false, 0
	}

	authMid.lockUntil = now.Add(authMid.lockDuration)
	authMid.failedAttempts = nil
	return true, authMid.lockDuration
}

func (authMid *HTTPAuthMiddleware) writeLockedResponse(w http.ResponseWriter, r *http.Request, retryAfter time.Duration) {
	seconds := int(retryAfter.Round(time.Second) / time.Second)
	seconds = max(seconds, 1)

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Retry-After", strconv.Itoa(seconds))

	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":       http.StatusTooManyRequests,
			"msg":        "dashboard auth temporarily locked",
			"retryAfter": seconds,
		})
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = io.WriteString(w, buildDashboardLockHTML(seconds))
}

func buildDashboardLockHTML(seconds int) string {
	const htmlTemplate = `<!doctype html><html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Dashboard Locked</title><style>
body{font-family:ui-sans-serif,system-ui,sans-serif;background:#0f172a;color:#e2e8f0;
display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}
.card{max-width:420px;padding:28px 24px;border-radius:16px;background:#111827;
box-shadow:0 20px 45px rgba(0,0,0,.35)}
h1{margin:0 0 12px;font-size:24px}
p{margin:0;color:#cbd5e1;line-height:1.6}
.count{display:inline-block;min-width:2ch;font-weight:700;color:#f59e0b}
</style></head><body>
<div class="card"><h1>Dashboard Temporarily Locked</h1>
<p>Too many failed login attempts. Authentication is paused for
<span id="count" class="count">%d</span> seconds.</p></div>
<script>
let left=%d;
const el=document.getElementById('count');
const timer=setInterval(()=>{left-=1;if(left<=0){clearInterval(timer);location.reload();return;}el.textContent=String(left);},1000);
</script>
</body></html>`
	return fmt.Sprintf(htmlTemplate, seconds, seconds)
}

type HTTPGzipWrapper struct {
	h http.Handler
}

func (gw *HTTPGzipWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		gw.h.ServeHTTP(w, r)
		return
	}
	w.Header().Set("Content-Encoding", "gzip")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	gzr := gzipResponseWriter{Writer: gz, ResponseWriter: w}
	gw.h.ServeHTTP(gzr, r)
}

func MakeHTTPGzipHandler(h http.Handler) http.Handler {
	return &HTTPGzipWrapper{
		h: h,
	}
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}
