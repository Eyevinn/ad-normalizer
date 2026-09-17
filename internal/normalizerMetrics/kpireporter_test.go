package normalizerMetrics

import (
	"os"
	"testing"
	"time"

	"github.com/matryer/is"
)

func TestMain(m *testing.M) {
	exitVal := m.Run()
	os.Exit(exitVal)
}

func TestKpiCollector(t *testing.T) {
	is := is.New(t)

	// Use a long export interval so the ticker never fires during the test;
	// we drive flushing explicitly via waitForIdle and cancel.
	c, cancel := NewNormalizerKpiCollector(time.Hour, "")

	collector, ok := c.(*normalizerKpiCollector)

	if !ok {
		t.Fatal("Could not cast to normalizerKpiCollector")
	}
	// Test that metrics are recorded correctly
	args := AdsHandledEventArguments{
		Subdomain:   "test-subdomain",
		BrokenAds:   5,
		IngestedAds: 100,
		ServedAds:   95,
	}

	c.AdsHandled(args)

	// Wait until the collector goroutine has processed the event before reading
	// kpiMap — eliminates the data race that caused flaky test results.
	collector.waitForIdle()

	// Check that the metric was recorded
	metrics, exists := collector.kpiMap["test-subdomain"]
	is.True(exists)
	is.Equal(metrics.Service, "test-subdomain")
	is.Equal(metrics.BrokenAds, 5)
	is.Equal(metrics.IngestedAds, 100)
	is.Equal(metrics.ServedAds, 95)

	// Add more metrics for the same subdomain
	args2 := AdsHandledEventArguments{
		Subdomain:   "test-subdomain",
		BrokenAds:   2,
		IngestedAds: 50,
		ServedAds:   48,
	}

	c.AdsHandled(args2)
	collector.waitForIdle()

	// Check that metrics are accumulated
	metrics, exists = collector.kpiMap["test-subdomain"]
	is.True(exists)
	is.Equal(metrics.BrokenAds, 7)
	is.Equal(metrics.IngestedAds, 150)
	is.Equal(metrics.ServedAds, 143)
	is.Equal(metrics.Service, "test-subdomain")

	// Cancelling the context triggers a final exportMetrics call, which resets
	// the map.  Wait for the goroutine to exit before reading kpiMap — this
	// guarantees no concurrent access and eliminates the data race.
	cancel()
	<-collector.doneCh

	// Check that map is cleared after export
	is.Equal(len(collector.kpiMap), 0)
}
