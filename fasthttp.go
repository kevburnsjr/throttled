package throttled

import (
	"errors"
	"math"
	"strconv"

	"github.com/valyala/fasthttp"
)

var (
	// DefaultDeniedHandler is the default DeniedHandler for an
	// HTTPRateLimiter. It returns a 429 status code with a generic
	// message.
	FastDefaultDeniedHandler = fasthttp.RequestHandler(func(ctx *fasthttp.RequestCtx) {
		ctx.Response.SetStatusCode(429)
		ctx.Response.SetBody([]byte("limit exceeded"))
	})

	// DefaultError is the default Error function for an HTTPRateLimiter.
	// It returns a 500 status code with a generic message.
	FastDefaultError = func(ctx *fasthttp.RequestCtx, err error) {
		ctx.Response.SetStatusCode(500)
		ctx.Response.SetBody([]byte("internal error"))
	}
)

// FastHTTPRateLimiter faciliates using a Limiter to limit HTTP requests.
type FastHTTPRateLimiter struct {
	// DeniedHandler is called if the request is disallowed. If it is
	// nil, the DefaultDeniedHandler variable is used.
	DeniedHandler fasthttp.RequestHandler

	// Error is called if the RateLimiter returns an error. If it is
	// nil, the DefaultErrorFunc is used.
	Error func(ctx *fasthttp.RequestCtx, err error)

	// Limiter is call for each request to determine whether the
	// request is permitted and update internal state. It must be set.
	RateLimiter RateLimiter

	// VaryBy is called for each request to generate a key for the
	// limiter. If it is nil, all requests use an empty string key.
	VaryBy interface {
		Key(*fasthttp.RequestCtx) string
	}
}

// RateLimit wraps a fasthttp.RequestHandler to limit incoming requests.
// Requests that are not limited will be passed to the handler
// unchanged.  Limited requests will be passed to the DeniedHandler.
// X-RateLimit-Limit, X-RateLimit-Remaining, X-RateLimit-Reset and
// Retry-After headers will be written to the response based on the
// values in the RateLimitResult.
func (t *FastHTTPRateLimiter) RateLimit(h fasthttp.RequestHandler) fasthttp.RequestHandler {
	return func(ctx *fasthttp.RequestCtx) {
		if t.RateLimiter == nil {
			t.error(ctx, errors.New("You must set a RateLimiter on HTTPRateLimiter"))
		}

		var k string
		if t.VaryBy != nil {
			k = t.VaryBy.Key(ctx)
		}

		limited, context, err := t.RateLimiter.RateLimit(k, 1)

		if err != nil {
			t.error(ctx, err)
			return
		}

		setFastHTTPRateLimitHeaders(ctx, context)

		if !limited {
			h(ctx)
		} else {
			dh := t.DeniedHandler
			if dh == nil {
				dh = FastDefaultDeniedHandler
			}
			dh(ctx)
		}
	}
}

func (t *FastHTTPRateLimiter) error(ctx *fasthttp.RequestCtx, err error) {
	e := t.Error
	if e == nil {
		e = FastDefaultError
	}
	e(ctx, err)
}

func setFastHTTPRateLimitHeaders(ctx *fasthttp.RequestCtx, context RateLimitResult) {
	if v := context.Limit; v >= 0 {
		ctx.Response.Header.Set("X-RateLimit-Limit", strconv.Itoa(v))
	}

	if v := context.Remaining; v >= 0 {
		ctx.Response.Header.Set("X-RateLimit-Remaining", strconv.Itoa(v))
	}

	if v := context.ResetAfter; v >= 0 {
		vi := int(math.Ceil(v.Seconds()))
		ctx.Response.Header.Set("X-RateLimit-Reset", strconv.Itoa(vi))
	}

	if v := context.RetryAfter; v >= 0 {
		vi := int(math.Ceil(v.Seconds()))
		ctx.Response.Header.Set("Retry-After", strconv.Itoa(vi))
	}
}
