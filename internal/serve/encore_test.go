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

// successfulEncoreHandlerStub returns an EncoreJob whose Status is "SUCCESSFUL"
// (the Encore-side status), so that GetTranscodeStatus correctly resolves to
// "PACKAGING" (non-JIT) or "COMPLETED" (JIT). The shared EncoreHandlerStub
// returns "COMPLETED" which maps to "UNKNOWN" — wrong for these tests.
type successfulEncoreHandlerStub struct{}

func (s *successfulEncoreHandlerStub) GetEncoreJob(jobId string) (structure.EncoreJob, error) {
	return structure.EncoreJob{
		Id:         jobId,
		ExternalId: jobId,
		Profile:    "test-profile",
		BaseName:   jobId,
		Status:     "SUCCESSFUL",
		Outputs: []structure.EncoreOutput{
			{
				MediaType: "Video",
				VideoStreams: []structure.EncoreVideoStream{
					{Codec: "AVC", Width: 1920, Height: 1080, FrameRate: "25"},
				},
			},
			{
				MediaType:    "Audio",
				AudioStreams: []structure.EncoreAudioStream{{Codec: "AAC", Channels: 2}},
			},
		},
		Inputs: []structure.EncoreInput{{Uri: "http://example.com/source/video.mp4"}},
	}, nil
}

func (s *successfulEncoreHandlerStub) CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error) {
	return structure.EncoreJob{}, nil
}

// setupApiWithJit constructs an API identical to setupApi but with jitPackage
// set to the given value and lastTtls initialised, so TTL assertions work.
func setupApiWithJit(jit bool) (*API, *StoreStub) {
	storeStub := &StoreStub{
		mockStore: make(map[string]structure.TranscodeInfo),
		lastTtls:  make(map[string]int64),
		kpis:      normalizerMetrics.NormalizerMetrics{},
	}
	adserverUrl, _ := url.Parse("http://localhost:9999")
	assetServerUrl, _ := url.Parse("https://asset-server.example.com")
	const wantTtl = int64(3600)
	apiConf := config.AdNormalizerConfig{
		AdServerUrl:    *adserverUrl,
		AssetServerUrl: *assetServerUrl,
		KeyField:       "url",
		KeyRegex:       "[^a-zA-Z0-9]",
		InFlightTtl:    int(wantTtl),
		JitPackage:     jit,
	}
	api := NewAPI(
		storeStub,
		apiConf,
		&successfulEncoreHandlerStub{},
		&http.Client{},
		storeStub.kpiReport,
	)
	return api, storeStub
}

// TestHandleTranscodeCompletedNonJitWritesPackagingMarkerWithTtl verifies that
// when JIT packaging is disabled (the normal path), the PACKAGING in-flight
// marker written by handleTranscodeCompleted carries the configured inFlightTtl.
// Without this TTL a callback-reject from the packager (issue #98/99) leaves
// the creative wedged at PACKAGING forever — recoverable only by manual Valkey
// deletion. With the TTL the marker self-expires and the next VAST request
// re-dispatches an Encore job automatically.
func TestHandleTranscodeCompletedNonJitWritesPackagingMarkerWithTtl(t *testing.T) {
	is := is.New(t)

	const wantTtl = int64(3600)
	api, storeStub := setupApiWithJit(false)

	progress := structure.EncoreJobProgress{
		JobId:      "test-job-id",
		ExternalId: "non-jit-creative-id",
		Status:     "SUCCESSFUL",
	}
	reqBody, err := json.Marshal(progress)
	is.NoErr(err)
	req, err := http.NewRequest("POST", "/encoreCallback", bytes.NewBuffer(reqBody))
	is.NoErr(err)
	rr := httptest.NewRecorder()
	api.HandleEncoreCallback(rr, req)
	is.Equal(rr.Code, http.StatusOK)

	// Exactly one Set call must have happened.
	is.Equal(storeStub.sets, 1)

	// The stored entry must be a PACKAGING (in-flight) marker.
	stored, ok, err := storeStub.Get(progress.ExternalId)
	is.NoErr(err)
	is.True(ok)
	is.Equal(stored.Status, "PACKAGING")

	// Most importantly: it must carry the in-flight TTL so it can self-expire.
	gotTtl, exists := storeStub.lastTtls[progress.ExternalId]
	is.True(exists)
	is.Equal(gotTtl, wantTtl)
}

// TestHandleTranscodeCompletedJitWritesCompletedMarkerWithoutTtl verifies that
// when JIT packaging is enabled, the COMPLETED terminal marker written by
// handleTranscodeCompleted persists forever (no TTL). Applying inFlightTtl to
// this path would cause JIT-packaged assets to disappear from the cache after
// the TTL expires — a regression.
func TestHandleTranscodeCompletedJitWritesCompletedMarkerWithoutTtl(t *testing.T) {
	is := is.New(t)

	api, storeStub := setupApiWithJit(true)

	progress := structure.EncoreJobProgress{
		JobId:      "test-job-id",
		ExternalId: "jit-creative-id",
		Status:     "SUCCESSFUL",
	}
	reqBody, err := json.Marshal(progress)
	is.NoErr(err)
	req, err := http.NewRequest("POST", "/encoreCallback", bytes.NewBuffer(reqBody))
	is.NoErr(err)
	rr := httptest.NewRecorder()
	api.HandleEncoreCallback(rr, req)
	is.Equal(rr.Code, http.StatusOK)

	// Exactly one Set call must have happened.
	is.Equal(storeStub.sets, 1)

	// The stored entry must be a COMPLETED (terminal) marker.
	stored, ok, err := storeStub.Get(progress.ExternalId)
	is.NoErr(err)
	is.True(ok)
	is.Equal(stored.Status, "COMPLETED")

	// Terminal markers must NOT carry a TTL — they should persist forever.
	gotTtl, exists := storeStub.lastTtls[progress.ExternalId]
	is.True(exists)
	is.Equal(gotTtl, int64(-1)) // -1 means no TTL was passed to Set
}

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

			api, ts, ss, _ := setupApi()
			defer ts.Close()
			reqBody, err := json.Marshal(c.progressUpdate)
			is.NoErr(err)
			req, err := http.NewRequest("POST", "/encore/callback", bytes.NewBuffer(reqBody))
			is.NoErr(err)
			rr := httptest.NewRecorder()
			api.HandleEncoreCallback(rr, req)
			is.Equal(rr.Code, http.StatusOK)
			is.Equal(ss.sets, c.expectSets)
			is.Equal(ss.deletes, c.expectDeletes)
			is.Equal(ss.gets, c.expectGets)
			ss.reset()
		})
	}
}
