package serve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Eyevinn/ad-normalizer/internal/config"
	"github.com/Eyevinn/ad-normalizer/internal/normalizerMetrics"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/matryer/is"
)

func TestEncoreCallback(t *testing.T) {
	is := is.New(t)
	cases := []struct {
		name           string
		progressUpdate structure.EncoreJobProgress
		expectSets     int
		expectDeletes  int
		expectGets     int
	}{
		{
			name: "Successful Transcode",
			progressUpdate: structure.EncoreJobProgress{
				Status: "SUCCESSFUL",
			},
			expectSets:    1,
			expectDeletes: 0,
			expectGets:    0,
		},
		{
			name: "Failed Transcode",
			progressUpdate: structure.EncoreJobProgress{
				Status: "FAILED",
			},
			expectSets:    0,
			expectDeletes: 1,
			expectGets:    0,
		},
		{
			name: "In Progress Transcode",
			progressUpdate: structure.EncoreJobProgress{
				Status: "IN_PROGRESS",
			},
			expectSets:    0,
			expectDeletes: 0,
			expectGets:    0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			ss := &StoreStub{
				mockStore: make(map[string]structure.TranscodeInfo),
				kpis:      normalizerMetrics.NormalizerMetrics{},
			}

			ts = setupTestServer()
			defer ts.Close()
			eh := &EncoreHandlerStub{}
			adserverUrl, _ := url.Parse(ts.URL)
			assetServerUrl, _ := url.Parse("https://asset-server.example.com")
			ac := config.AdNormalizerConfig{
				AdServerUrl:    *adserverUrl,
				AssetServerUrl: *assetServerUrl,
				KeyField:       "url",
				KeyRegex:       "[^a-zA-Z0-9]",
				KpiPostUrl:     "http://kpi-post.example.com/metrics",
			}
			// Initialize the API with the mock store
			runApiInstance := NewAPI(
				ss,
				ac,
				eh,
				&http.Client{}, // Use nil for the client in tests, or you can create a mock client
				ss.kpiReport,
			)
			reqBody, err := json.Marshal(c.progressUpdate)
			is.NoErr(err)
			req, err := http.NewRequest("POST", "/encore/callback", bytes.NewBuffer(reqBody))
			is.NoErr(err)
			rr := httptest.NewRecorder()
			runApiInstance.HandleEncoreCallback(rr, req)
			is.Equal(rr.Code, http.StatusOK)
			is.Equal(ss.sets, c.expectSets)
			is.Equal(ss.deletes, c.expectDeletes)
			is.Equal(ss.gets, c.expectGets)
			ss.reset()
		})
	}
}
