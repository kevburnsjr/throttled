package throttled_test

import (
"time"
"errors"
	"testing"

	"github.com/throttled/throttled"
	"github.com/valyala/fasthttp"
)

type fastStubLimiter struct {
}

func (sl *fastStubLimiter) RateLimit(key string, quantity int) (bool, throttled.RateLimitResult, error) {
	switch key {
	case "/limit":
		result := throttled.RateLimitResult{
			Limit:      -1,
			Remaining:  -1,
			ResetAfter: -1,
			RetryAfter: time.Minute,
		}
		return true, result, nil
	case "/error":
		result := throttled.RateLimitResult{}
		return false, result, errors.New("stubLimiter error")
	default:
		result := throttled.RateLimitResult{
			Limit:      1,
			Remaining:  2,
			ResetAfter: time.Minute,
			RetryAfter: -1,
		}
		return false, result, nil
	}
}

type fastPathGetter struct{}

func (*fastPathGetter) Key(ctx *fasthttp.RequestCtx) string {
	return string(ctx.Request.URI().Path())
}

func TestFastHTTPRateLimiter(t *testing.T) {
	limiter := throttled.FastHTTPRateLimiter{
		RateLimiter: &fastStubLimiter{},
		VaryBy:      &fastPathGetter{},
	}

	handler := limiter.RateLimit(func(ctx *fasthttp.RequestCtx){})

	runFastHTTPTestCases(t, handler, []httpTestCase{
		{"ok", 200, map[string]string{"X-Ratelimit-Limit": "1", "X-Ratelimit-Remaining": "2", "X-Ratelimit-Reset": "60"}},
		{"/error", 500, map[string]string{}},
		{"/limit", 429, map[string]string{"Retry-After": "60"}},
	})
}

func TestCustomFastHTTPRateLimiterHandlers(t *testing.T) {
	limiter := throttled.FastHTTPRateLimiter{
		RateLimiter: &fastStubLimiter{},
		VaryBy:      &fastPathGetter{},
		DeniedHandler: func(ctx *fasthttp.RequestCtx) {
			ctx.Response.SetStatusCode(400)
			ctx.Response.SetBody([]byte("custom limit exceeded"))
		},
		Error: func(ctx *fasthttp.RequestCtx, err error) {
			ctx.Response.SetStatusCode(501)
			ctx.Response.SetBody([]byte("custom internal error"))
		},
	}

	handler := limiter.RateLimit(func(ctx *fasthttp.RequestCtx){})

	runFastHTTPTestCases(t, handler, []httpTestCase{
		{"/limit", 400, map[string]string{}},
		{"/error", 501, map[string]string{}},
	})
}

func runFastHTTPTestCases(t *testing.T, h fasthttp.RequestHandler, cs []httpTestCase) {
	var ctx fasthttp.RequestCtx
	for i, c := range cs {
		ctx.Request.Reset()
		ctx.Request.Header.SetRequestURI(c.path)
		ctx.Request.Header.SetMethod("GET")
		h(&ctx)
		if have, want := ctx.Response.StatusCode(), c.code; have != want {
			t.Errorf("Expected request %d at %s to return %d but got %d",
				i, c.path, want, have)
		}

		for name, want := range c.headers {
			if have := string(ctx.Response.Header.Peek(name)); have != want {
				t.Errorf("Expected request %d at %s to have header '%s: %s' but got '%s'",
					i, c.path, name, want, have)
			}
		}
	}
}

func BenchmarkFastHTTPRateLimiter(b *testing.B) {
	limiter := throttled.FastHTTPRateLimiter{
		RateLimiter: &stubLimiter{},
		VaryBy:      &fastPathGetter{},
	}
	h := limiter.RateLimit(func(ctx *fasthttp.RequestCtx) {})
	var ctx fasthttp.RequestCtx
	ctx.Request.Reset()
	ctx.Request.Header.SetRequestURI("/")
	ctx.Request.Header.SetMethod("GET")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h(&ctx)
	}
	_ = ctx.Response.Body()
}
