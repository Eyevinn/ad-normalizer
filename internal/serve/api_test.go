package serve

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/Eyevinn/VMAP/vmap"
	"github.com/Eyevinn/ad-normalizer/internal/config"
	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/matryer/is"
)

type StoreStub struct {
	mockStore map[string]string
}

func (s *StoreStub) Get(key string) (string, bool, error) {
	if value, exists := s.mockStore[key]; exists {
		return value, true, nil
	}
	return "", false, nil
}

func (s *StoreStub) Set(key string, value string, ttl ...int) error {
	s.mockStore[key] = value
	return nil
}

type EncoreHandlerStub struct {
}

// TODO: refactor from AI slop
func (e *EncoreHandlerStub) CreateJob(creative *structure.ManifestAsset) (structure.EncoreJob, error) {
	newJob := structure.EncoreJob{}
	return newJob, nil
}

var api *API
var testServer *httptest.Server
var encoreHandler *EncoreHandlerStub // TODO: Convert encorehandler to interface
var storeStub *StoreStub

func TestMain(m *testing.M) {
	storeStub = &StoreStub{
		mockStore: make(map[string]string),
	}

	testServer = setupTestServer()
	defer testServer.Close()
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
	_ = storeStub.Set(adKey, "https://testcontent.eyevinn.technology/ads/alvedon-10s.m3u8")
	vastReq, err := http.NewRequest(
		"GET",
		testServer.URL+"/vast",
		nil,
	)
	is.NoErr(err)
	vastReq.Header.Set("User-Agent", "TestUserAgent")
	vastReq.Header.Set("X-Forwarded-For", "123.123.123")
	vastReq.Header.Set("X-Device-User-Agent", "TestDeviceUserAgent")
	vastReq.Header.Set("accept", "application/xml")
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
}

func TestReplaceVmap(t *testing.T) {
	is := is.New(t)
	f, err := os.Open("../test_data/testVmap.xml")
	defer func() {
		_ = f.Close()
	}()

	is.NoErr(err)
	// This function should contain the test logic for the ReplaceVmap function
	// It should set up the necessary mocks and expectations, call the function,
	// and then assert that the results are as expected.

}

func setupTestServer() *httptest.Server {
	vastData, _ := os.ReadFile("../test_data/testVast.xml")
	vmapData, _ := os.ReadFile("../test_data/testVmap.xml")
	return httptest.NewServer(http.HandlerFunc(
		func(res http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/vast" {
				time.Sleep(time.Millisecond * 10)
				res.Header().Set("Content-Type", "application/xml")
				res.WriteHeader(200)
				_, _ = res.Write(vastData)
			} else if req.URL.Path == "/vmap" {
				time.Sleep(time.Millisecond * 10)
				res.Header().Set("Content-Type", "application/xml")
				res.WriteHeader(200)
				_, _ = res.Write(vmapData)
			} else {
				res.WriteHeader(404)
			}
		}))
}
