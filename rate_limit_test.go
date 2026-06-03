package ekodb

import (
	"net/http"
	"strconv"
	"sync"
	"testing"
)

// TestExtractRateLimitInfoNoDataRace exercises concurrent writes (extractRateLimitInfo)
// and reads (GetRateLimitInfo / IsNearRateLimit) of rateLimitInfo. Run under `go test -race`
// it fails if the field is accessed without synchronization. Regression for #33.
func TestExtractRateLimitInfoNoDataRace(t *testing.T) {
	c := &Client{}

	var wg sync.WaitGroup
	const iterations = 100
	for i := 0; i < iterations; i++ {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			resp := &http.Response{Header: http.Header{}}
			resp.Header.Set("X-RateLimit-Limit", "1000")
			resp.Header.Set("X-RateLimit-Remaining", strconv.Itoa(1000-n))
			resp.Header.Set("X-RateLimit-Reset", "1700000000")
			c.extractRateLimitInfo(resp)
		}(i)
		go func() {
			defer wg.Done()
			_ = c.GetRateLimitInfo()
			_ = c.IsNearRateLimit()
		}()
	}
	wg.Wait()

	info := c.GetRateLimitInfo()
	if info == nil {
		t.Fatal("expected rate limit info to be set")
	}
	if info.Limit != 1000 {
		t.Fatalf("expected Limit=1000, got %d", info.Limit)
	}
}
