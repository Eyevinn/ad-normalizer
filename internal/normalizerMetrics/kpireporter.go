package normalizerMetrics

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/Eyevinn/ad-normalizer/internal/logger"
)

type NormalizerKpiReporter interface {
	AdsHandled(AdsHandledEventArguments)
}

type AdsHandledEventArguments struct {
	Subdomain   string
	BrokenAds   int
	IngestedAds int
	ServedAds   int
}

type NormalizerMetrics struct {
	Service     string `json:"service"`
	BrokenAds   int    `json:"broken_ads"`
	IngestedAds int    `json:"ingested_ads"`
	ServedAds   int    `json:"served_ads"`
}

type NormalizerMetricsRequest = map[string]NormalizerMetrics // Key is same as Service == subdomain

type normalizerKpiCollector struct {
	kpiMap  map[string]*NormalizerMetrics
	ch      chan AdsHandledEventArguments
	ctx     context.Context
	postUrl string
}

func NewNormalizerKpiCollector(exportInterval time.Duration, postUrl string) (NormalizerKpiReporter, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	collector := &normalizerKpiCollector{}
	collector.kpiMap = make(map[string]*NormalizerMetrics)
	collector.ch = make(chan AdsHandledEventArguments, 1000)
	collector.ctx = ctx

	_, err := url.Parse(postUrl)
	if err == nil {
		collector.postUrl = postUrl
	} else {
		logger.Error(
			"Could not parse KPI POST url, no KPIs will be exported via POST",
			slog.String("err", err.Error()),
		)
	}

	go collector.runCollector(exportInterval)

	return collector, cancel
}

func (c *normalizerKpiCollector) runCollector(exportInterval time.Duration) {
	ticker := time.NewTicker(exportInterval)
	defer ticker.Stop()
	for {
		select {
		case args := <-c.ch:
			c.recordMetrics(args)
		case <-ticker.C:
			c.exportMetrics()
		case <-c.ctx.Done():
			logger.Info("Stopping KPI exporter", slog.String("err", c.ctx.Err().Error()))
			close(c.ch)
			c.exportMetrics()
			return
		}
	}
}

func (c *normalizerKpiCollector) exportMetrics() {
	j, err := json.Marshal(c.kpiMap)
	if err != nil {
		logger.Error("Could not marshal KPI metrics", slog.String("err", err.Error()))
		return
	}
	logger.Debug("Reporting KPIs", slog.String("json", string(j)))
	go c.postMetrics(j)
	c.kpiMap = make(map[string]*NormalizerMetrics)
}

func (c *normalizerKpiCollector) postMetrics(data []byte) {
	if c.postUrl == "" {
		logger.Error("No KPI POST url configured, not posting KPIs")
		return
	}

	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	r, err := http.NewRequestWithContext(ctx, http.MethodPost, c.postUrl, bytes.NewBuffer(data))
	if err != nil {
		logger.Error("Failed to create KPI report request", slog.String("err", err.Error()))
	}
	r.Close = true
	r.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	res, err := client.Do(r)
	if err != nil {
		logger.Error("Could not report KPIs", slog.String("err", err.Error()))
		return
	}

	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		logger.Error("Could not read KPI report response", slog.String("err", err.Error()))
		return
	}

	if res.StatusCode >= 400 {
		logger.Error("Bad HTTP status code when reporting KPIs",
			slog.Int("statusCode", res.StatusCode),
			slog.String("body", string(bodyBytes)),
		)
		return
	}
}

func (c *normalizerKpiCollector) recordMetrics(args AdsHandledEventArguments) {
	key := args.Subdomain
	metrics, exists := c.kpiMap[key]
	if !exists {
		metrics = &NormalizerMetrics{
			Service:     args.Subdomain,
			BrokenAds:   0,
			IngestedAds: 0,
			ServedAds:   0,
		}
		c.kpiMap[key] = metrics
	}

	if args.BrokenAds > 0 {
		metrics.BrokenAds += args.BrokenAds
	}
	if args.IngestedAds > 0 {
		metrics.IngestedAds += args.IngestedAds
	}
	if args.ServedAds > 0 {
		metrics.ServedAds += args.ServedAds
	}
	logger.Debug(
		"added metrics, new state:",
		slog.String("key", key),
		slog.Int("broken", metrics.BrokenAds),
		slog.Int("ingested", metrics.IngestedAds),
		slog.Int("served", metrics.ServedAds),
	)

}

func (c *normalizerKpiCollector) AdsHandled(args AdsHandledEventArguments) {
	c.ch <- args
}
