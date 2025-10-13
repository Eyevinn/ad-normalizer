package serve

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Eyevinn/VMAP/vmap"
	"github.com/Eyevinn/ad-normalizer/internal/config"
	"github.com/Eyevinn/ad-normalizer/internal/logger"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/google/uuid"
	"github.com/matryer/is"
)

type StoreStub struct {
	mockStore map[string]structure.TranscodeInfo
	sets      int
	gets      int
	deletes   int
	blacklist []string
}

// Delete implements store.Store.
func (s *StoreStub) Delete(key string) error {
	delete(s.mockStore, key)
	s.deletes++
	return nil
}

func (s *StoreStub) Get(key string) (structure.TranscodeInfo, bool, error) {
	s.gets++
	if value, exists := s.mockStore[key]; exists {
		return value, true, nil
	}
	return structure.TranscodeInfo{}, false, nil
}

func (s *StoreStub) Set(key string, value structure.TranscodeInfo, ttl ...int64) error {
	s.sets++
	s.mockStore[key] = value
	return nil
}

func (s *StoreStub) List(page int, size int) ([]structure.TranscodeInfo, int64, error) {
	result := make([]structure.TranscodeInfo, 0, size)
	for i := range size {
		strVal := strconv.Itoa((page * size) + (size - 1 - i))
		tci := structure.TranscodeInfo{
			Url:        "http://example.com/video" + strVal + "/index.m3u8",
			Status:     "COMPLETED",
			Source:     "s3://fake-bucket/video" + strVal + ".mp4",
			LastUpdate: time.Now().Unix(),
		}
		result = append(result, tci)
	}
	return result, int64(size), nil
}

func (s *StoreStub) reset() {
	s.mockStore = make(map[string]structure.TranscodeInfo)
	s.sets = 0
	s.gets = 0
	s.deletes = 0
}

func (s *StoreStub) BlackList(key string) error {
	s.blacklist = append(s.blacklist, key)
	return nil
}

func (s *StoreStub) InBlackList(key string) (bool, error) {
	for _, blacklistedKey := range s.blacklist {
		if blacklistedKey == key {
			return true, nil
		}
	}
	return false, nil
}

func (s *StoreStub) RemoveFromBlackList(key string) error {
	for i, blacklistedKey := range s.blacklist {
		if blacklistedKey == key {
			s.blacklist = append(s.blacklist[:i], s.blacklist[i+1:]...)
			return nil
		}
	}
	return nil // Key not found in blacklist, nothing to remove
}

func (s *StoreStub) EnqueuePackagingJob(queueName string, message structure.PackagingQueueMessage) error {
	// This is a stub, in a real implementation this would enqueue the job to a queue
	return nil
}

type EncoreHandlerStub struct {
	calls int
}

// GetEncoreJob implements encore.EncoreHandler.
func (e *EncoreHandlerStub) GetEncoreJob(jobId string) (structure.EncoreJob, error) {
	return structure.EncoreJob{
		Id:         uuid.NewString(),
		ExternalId: jobId,
		Profile:    "test-profile",
		BaseName:   jobId,
		Status:     "COMPLETED",
		Outputs: []structure.EncoreOutput{
			{
				MediaType: "Video",
				VideoStreams: []structure.EncoreVideoStream{
					{
						Codec:     "AVC",
						Width:     1920,
						Height:    1080,
						FrameRate: "25",
					},
				},
			},
		},
		Inputs: []structure.EncoreInput{
			{
				Uri: "http://example.com/source/video.mp4",
			},
		},
	}, nil
}

func (e *EncoreHandlerStub) reset() {
	logger.Info("Resetting EncoreHandlerStub")
	e.calls = 0
}

func (e *EncoreHandlerStub) CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error) {
	logger.Info("EncoreHandlerStub.createJob called")
	newJob := structure.EncoreJob{}
	e.calls += 1
	return newJob, nil
}

var api *API
var testServer *httptest.Server
var encoreHandler *EncoreHandlerStub
var storeStub *StoreStub

func TestMain(m *testing.M) {
	storeStub = &StoreStub{
		mockStore: make(map[string]structure.TranscodeInfo),
	}

	testServer = setupTestServer()
	defer testServer.Close()
	encoreHandler = &EncoreHandlerStub{}
	adserverUrl, _ := url.Parse(testServer.URL)
	assetServerUrl, _ := url.Parse("https://asset-server.example.com")
	apiConf := config.AdNormalizerConfig{
		AdServerUrl:    *adserverUrl,
		AssetServerUrl: *assetServerUrl,
		KeyField:       "url",
		KeyRegex:       "[^a-zA-Z0-9]",
	}
	// Initialize the API with the mock store
	api = NewAPI(
		storeStub,
		apiConf,
		encoreHandler,
		&http.Client{}, // Use nil for the client in tests, or you can create a mock client
	)

	// Run the tests
	exitCode := m.Run()

	// Clean up if necessary
	os.Exit(exitCode)
}

func TestReplaceVast(t *testing.T) {
	is := is.New(t)
	// Populate the store with one ad
	re := regexp.MustCompile("[^a-zA-Z0-9]")
	adKey := re.ReplaceAllString("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4", "")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vastReq, err := http.NewRequest(
		"GET",
		testServer.URL,
		nil,
	)
	is.NoErr(err)
	vastReq.Header.Set("User-Agent", "TestUserAgent")
	vastReq.Header.Set("X-Forwarded-For", "123.123.123")
	vastReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vastReq.Header.Set("accept", "application/xml")
	// make sure we request a VAST response
	qps := vastReq.URL.Query()
	newUrl := strings.Replace(testServer.URL, "127", "128", 1)
	parsedUrl, err := url.Parse(newUrl)
	is.NoErr(err)
	api.adServerUrl = *parsedUrl
	qps.Set("requestType", "vast")
	qps.Set("subDomain", "127")
	vastReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVast(recorder, vastReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/xml")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vastRes, err := vmap.DecodeVast(responseBody)
	is.NoErr(err)
	is.Equal(len(vastRes.Ad), 1)
	mediaFile := vastRes.Ad[0].InLine.Creatives[0].Linear.MediaFiles[0]
	is.Equal(mediaFile.MediaType, "application/x-mpegURL")
	is.Equal(mediaFile.Text, "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8")
	is.Equal(mediaFile.Width, 718)
	is.Equal(mediaFile.Height, 404)

	realUrl, _ := url.Parse(testServer.URL)
	api.adServerUrl = *realUrl // Reset to original URL

	encoreHandler.reset()
	storeStub.reset()
}

func TestReplaceVastWithBlacklisted(t *testing.T) {
	is := is.New(t)
	// Populate the store with one ad
	re := regexp.MustCompile("[^a-zA-Z0-9]")
	adKey := re.ReplaceAllString("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4", "")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vastReq, err := http.NewRequest(
		"GET",
		testServer.URL,
		nil,
	)
	is.NoErr(err)
	_ = storeStub.BlackList("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4")
	vastReq.Header.Set("User-Agent", "TestUserAgent")
	vastReq.Header.Set("X-Forwarded-For", "123.123.123")
	vastReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vastReq.Header.Set("accept", "application/xml")
	// make sure we request a VAST response
	qps := vastReq.URL.Query()
	is.NoErr(err)
	qps.Set("requestType", "vast")
	vastReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVast(recorder, vastReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/xml")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vastRes, err := vmap.DecodeVast(responseBody)
	is.NoErr(err)
	is.Equal(len(vastRes.Ad), 0) // since the ad is blacklisted, we should not get any ads back

	encoreHandler.reset()
	storeStub.reset()
	storeStub.blacklist = []string{} // Reset the blacklist
}

func TestReplaceVastWithFiller(t *testing.T) {
	is := is.New(t)
	re := regexp.MustCompile("[^a-zA-Z0-9]")
	adKey := re.ReplaceAllString("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4", "")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	// add a filler
	fillerInfo := structure.TranscodeInfo{
		Url:         "http://example.com/video.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	fillerKey := re.ReplaceAllString("http://example.com/video.mp4", "")
	_ = storeStub.Set(fillerKey, fillerInfo)

	vastReq, err := http.NewRequest(
		"GET",
		testServer.URL,
		nil,
	)
	is.NoErr(err)
	vastReq.Header.Set("User-Agent", "TestUserAgent")
	vastReq.Header.Set("X-Forwarded-For", "123.123.123")
	vastReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vastReq.Header.Set("accept", "application/xml")
	qps := vastReq.URL.Query()
	qps.Set("requestType", "vast")
	qps.Set("filler", "http://example.com/video.mp4")
	vastReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVast(recorder, vastReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/xml")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vastRes, err := vmap.DecodeVast(responseBody)
	is.NoErr(err)
	is.Equal(len(vastRes.Ad), 2)
	mediaFile := vastRes.Ad[0].InLine.Creatives[0].Linear.MediaFiles[0]
	is.Equal(mediaFile.MediaType, "application/x-mpegURL")
	is.Equal(mediaFile.Text, "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8")
	is.Equal(mediaFile.Width, 718)
	is.Equal(mediaFile.Height, 404)

	filler := vastRes.Ad[1]
	is.Equal(filler.Id, "NORMALIZER_FILLER")

	encoreHandler.reset()
	storeStub.reset()
}

func TestGetAssetList(t *testing.T) {
	is := is.New(t)
	// Populate the store with one ad
	re := regexp.MustCompile("[^a-zA-Z0-9]")
	adKey := re.ReplaceAllString("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4", "")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vastReq, err := http.NewRequest(
		"GET",
		testServer.URL,
		nil,
	)
	is.NoErr(err)
	vastReq.Header.Set("User-Agent", "TestUserAgent")
	vastReq.Header.Set("X-Forwarded-For", "123.123.123")
	vastReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vastReq.Header.Set("Accept", "application/json")
	// make sure we request a VAST response
	qps := vastReq.URL.Query()
	qps.Set("requestType", "vast")
	vastReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVast(recorder, vastReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/json")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	var assetList []structure.AssetDescription
	err = json.Unmarshal(responseBody, &assetList)
	is.NoErr(err)
	is.Equal(len(assetList), 1)
	is.Equal(assetList[0].Uri, transcodeInfo.Url)
	is.Equal(assetList[0].Duration, 10.25)

	encoreHandler.reset()
	storeStub.reset()
}

func TestEmptyVmap(t *testing.T) {
	is := is.New(t)
	vmapReq, err := http.NewRequest(
		"GET",
		testServer.URL+"/vmap",
		nil,
	)
	is.NoErr(err)
	vmapReq.Header.Set("User-Agent", "TestUserAgent")
	vmapReq.Header.Set("X-Forwarded-For", "123.123.123")
	vmapReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vmapReq.Header.Set("accept", "application/xml")
	qps := vmapReq.URL.Query()
	qps.Set("requestType", "vmap")
	qps.Set("empty", "true") // tell test server to return empty vmap
	vmapReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVmap(recorder, vmapReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/xml")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vmapRes, err := vmap.DecodeVmap(responseBody)
	is.NoErr(err)
	is.Equal(len(vmapRes.AdBreaks), 1)
	firstBreak := vmapRes.AdBreaks[0]
	is.Equal(firstBreak.TimeOffset.Position, vmap.OffsetStart)
	is.Equal(firstBreak.BreakType, "linear")
	is.Equal(len(firstBreak.AdSource.VASTData.VAST.Ad), 0)

	encoreHandler.reset()
	storeStub.reset()
}

func TestReplaceVmap(t *testing.T) {
	is := is.New(t)
	f, err := os.Open("../test_data/testVmap.xml")
	defer func() {
		_ = f.Close()
	}()
	is.NoErr(err)

	// Populate the store with one ad
	re := regexp.MustCompile("[^a-zA-Z0-9]")
	adKey := re.ReplaceAllString("https://testcontent.eyevinn.technology/ads/alvedon-10s.mp4", "")
	transcodeInfo := structure.TranscodeInfo{
		Url:         "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8",
		AspectRatio: "16:9",
		FrameRates:  []float64{25.0},
		Status:      "COMPLETED",
	}
	_ = storeStub.Set(adKey, transcodeInfo)
	vmapReq, err := http.NewRequest(
		"GET",
		testServer.URL+"/vmap",
		nil,
	)
	is.NoErr(err)
	vmapReq.Header.Set("User-Agent", "TestUserAgent")
	vmapReq.Header.Set("X-Forwarded-For", "123.123.123")
	vmapReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vmapReq.Header.Set("accept", "application/xml")
	qps := vmapReq.URL.Query()
	qps.Set("requestType", "vmap")
	vmapReq.URL.RawQuery = qps.Encode()
	recorder := httptest.NewRecorder()
	api.HandleVmap(recorder, vmapReq)
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/xml")
	defer recorder.Result().Body.Close()

	responseBody, err := io.ReadAll(recorder.Result().Body)
	is.NoErr(err)
	vmapRes, err := vmap.DecodeVmap(responseBody)
	is.NoErr(err)
	is.Equal(len(vmapRes.AdBreaks), 1)

	firstBreak := vmapRes.AdBreaks[0]
	is.Equal(firstBreak.TimeOffset.Position, vmap.OffsetStart)
	is.Equal(firstBreak.BreakType, "linear")

	firstVast := firstBreak.AdSource.VASTData.VAST
	is.Equal(len(firstVast.Ad), 1)
	firstAd := firstVast.Ad[0]
	is.Equal(firstAd.Id, "POD_AD-ID_001")
	is.Equal(firstAd.Sequence, 1)
	is.Equal(len(firstAd.InLine.Creatives), 1)
	firstCreative := firstAd.InLine.Creatives[0]
	is.Equal(len(firstCreative.Linear.TrackingEvents), 5)
	is.Equal(len(firstCreative.Linear.ClickTracking), 1)
	is.Equal(firstCreative.Linear.Duration.Duration, 10*time.Second)
	mediaFile := firstCreative.Linear.MediaFiles[0]
	is.Equal(mediaFile.MediaType, "application/x-mpegURL")
	is.Equal(mediaFile.Text, "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8")
	is.Equal(mediaFile.Width, 718)
	is.Equal(mediaFile.Height, 404)
	encoreHandler.reset()
	storeStub.reset()
}

func TestBlacklist(t *testing.T) {
	is := is.New(t)
	blacklistUrl := "https://adserver-assets.io/badfile.mp4"
	reqBody := blacklistRequest{
		MediaUrl: blacklistUrl,
	}
	serializedBody, err := json.Marshal(reqBody)
	is.NoErr(err)
	blacklistReq, err := http.NewRequest(
		"POST",
		testServer.URL+"/blacklist/",
		bytes.NewBuffer(serializedBody),
	)
	is.NoErr(err)
	recorder := httptest.NewRecorder()
	api.HandleBlackList(recorder, blacklistReq)
	is.Equal(recorder.Result().StatusCode, http.StatusNoContent)
	is.Equal(len(storeStub.blacklist), 1)

	//remove from blacklist
	unblacklistReq, err := http.NewRequest(
		"DELETE",
		testServer.URL+"/blacklist/",
		bytes.NewBuffer(serializedBody),
	)
	is.NoErr(err)
	recorder = httptest.NewRecorder()
	api.HandleBlackList(recorder, unblacklistReq)
	is.Equal(recorder.Result().StatusCode, http.StatusNoContent)
	is.Equal(len(storeStub.blacklist), 0)
}

// TODO: Add test for status endpoint

func TestHandleJobList(t *testing.T) {
	is := is.New(t)

	// Create test request
	req, err := http.NewRequest(http.MethodGet, "/status", nil)
	is.NoErr(err)

	// Create response recorder
	recorder := httptest.NewRecorder()

	// Call the handler
	api.HandleJobList(recorder, req)

	// Check the response
	is.Equal(recorder.Result().StatusCode, http.StatusOK)
	is.Equal(recorder.Result().Header.Get("Content-Type"), "application/json")

	// Parse the response body
	var response statusResponse
	err = json.NewDecoder(recorder.Body).Decode(&response)
	is.NoErr(err)

	// Verify response contents
	is.Equal(response.Page, 0)
	is.Equal(len(response.Jobs), 10)
	is.Equal(response.Next, "/jobs?page=1&size=10")
	is.Equal(response.Prev, "")
	for i, job := range response.Jobs {
		expectedIndex := 9 - i // Since jobs are in descending order
		expectedUrl := "http://example.com/video" + strconv.Itoa(expectedIndex) + "/index.m3u8"
		expectedSource := "s3://fake-bucket/video" + strconv.Itoa(expectedIndex) + ".mp4"
		is.Equal(job.Url, expectedUrl)
		is.Equal(job.Status, "COMPLETED")
		is.Equal(job.Source, expectedSource)
		is.True(job.LastUpdate > 0)
	}
}

func TestHandleJobListInvalidPageParameter(t *testing.T) {
	is := is.New(t)

	// Test with invalid page parameter
	req, err := http.NewRequest(http.MethodGet, "/status?page=invalid", nil)
	is.NoErr(err)

	recorder := httptest.NewRecorder()
	api.HandleJobList(recorder, req)

	is.Equal(recorder.Result().StatusCode, http.StatusBadRequest)

	body, err := io.ReadAll(recorder.Body)
	is.NoErr(err)
	is.True(strings.Contains(string(body), "Invalid page parameter"))
}

func TestHandleJobListInvalidSizeParameter(t *testing.T) {
	is := is.New(t)

	// Test with invalid size parameter
	req, err := http.NewRequest(http.MethodGet, "/status?size=invalid", nil)
	is.NoErr(err)

	recorder := httptest.NewRecorder()
	api.HandleJobList(recorder, req)

	is.Equal(recorder.Result().StatusCode, http.StatusBadRequest)

	body, err := io.ReadAll(recorder.Body)
	is.NoErr(err)
	is.True(strings.Contains(string(body), "Invalid size parameter"))
}

func TestHandleJobListNegativePageParameter(t *testing.T) {
	is := is.New(t)

	// Test with negative page parameter
	req, err := http.NewRequest(http.MethodGet, "/status?page=-1", nil)
	is.NoErr(err)

	recorder := httptest.NewRecorder()
	api.HandleJobList(recorder, req)

	is.Equal(recorder.Result().StatusCode, http.StatusBadRequest)

	body, err := io.ReadAll(recorder.Body)
	is.NoErr(err)
	is.True(strings.Contains(string(body), "Invalid page parameter"))
}

func TestHandleJobListInvalidSizeParameterZero(t *testing.T) {
	is := is.New(t)

	// Test with size parameter as zero
	req, err := http.NewRequest(http.MethodGet, "/status?size=0", nil)
	is.NoErr(err)

	recorder := httptest.NewRecorder()
	api.HandleJobList(recorder, req)

	is.Equal(recorder.Result().StatusCode, http.StatusBadRequest)

	body, err := io.ReadAll(recorder.Body)
	is.NoErr(err)
	is.True(strings.Contains(string(body), "Invalid size parameter"))
}

func setupTestServer() *httptest.Server {
	vastData, _ := os.ReadFile("../test_data/testVast.xml")
	vmapData, _ := os.ReadFile("../test_data/testVmap.xml")
	emptyVmapData, _ := os.ReadFile("../test_data/emptyVmap.xml")
	return httptest.NewServer(http.HandlerFunc(
		func(res http.ResponseWriter, req *http.Request) {
			switch req.URL.Query().Get("requestType") {
			case "vast":
				time.Sleep(time.Millisecond * 10)
				res.Header().Set("Content-Type", "application/xml")
				if strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") {
					res.Header().Set("Content-Encoding", "gzip")
					res.WriteHeader(http.StatusOK)
					writer := gzip.NewWriter(res)
					defer func() {
						_ = writer.Close()
					}()
					_, _ = writer.Write(vastData)
				} else {
					res.WriteHeader(http.StatusOK)
					_, _ = res.Write(vastData)
				}
			case "vmap":
				time.Sleep(time.Millisecond * 10)
				res.Header().Set("Content-Type", "application/xml")
				res.WriteHeader(200)
				if req.URL.Query().Get("empty") == "true" {
					_, _ = res.Write(emptyVmapData)
				} else {
					_, _ = res.Write(vmapData)
				}
			default:
				res.WriteHeader(404)
			}
		}))
}
