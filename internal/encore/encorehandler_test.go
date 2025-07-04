package encore

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Eyevinn/ad-normalizer/internal/structure"
	"github.com/google/uuid"
	"github.com/matryer/is"
)

var encoreHandler EncoreHandler

func TestMain(m *testing.M) {
	testServer := setupTestServer()
	defer testServer.Close()
	client := &http.Client{}
	testUrl, _ := url.Parse(testServer.URL)
	bucketUrl, _ := url.Parse("s3://example.com/transcoding-output/")
	rootUrl, _ := url.Parse("https://ad-normalizer.osaas.io")
	encoreHandler = NewHttpEncoreHandler(
		client,
		*testUrl,
		"test-profile",
		nil,
		*bucketUrl,
		*rootUrl,
	)

	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestCreateJob(t *testing.T) {
	is := is.New(t)
	asset := &structure.ManifestAsset{
		CreativeId:        "test-creative-id",
		MasterPlaylistUrl: "http://example.com/test.mp4",
	}
	created, err := encoreHandler.CreateJob(asset)
	is.NoErr(err)
	is.Equal(created.ExternalId, asset.CreativeId)
	is.Equal(created.Profile, "test-profile")
	is.Equal(created.BaseName, "test-creative-id")
	is.True(strings.HasPrefix(created.OutputFolder, "s3://example.com/transcoding-output/test-creative-id/"))
	is.Equal(created.ProgressCallbackUri, "https://ad-normalizer.osaas.io/encoreCallback")
	is.Equal(len(created.Inputs), 1)
}

func TestGetJob(t *testing.T) {
	is := is.New(t)
	jobId := uuid.New().String()
	_, err := encoreHandler.GetEncoreJob(jobId)
	is.NoErr(err)
}

func setupTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			validRequest := validateRequest(r)
			if !validRequest {
				http.Error(w, "Invalid request", http.StatusBadRequest)
				return
			}
			defer r.Body.Close()
			jsonDecoder := json.NewDecoder(r.Body)
			postedJob := structure.EncoreJob{}
			err := jsonDecoder.Decode(&postedJob)
			if postedJob.ExternalId == "" {
				http.Error(w, "Missing required field: externalId", http.StatusBadRequest)
				return
			}
			if err != nil {
				if err == io.EOF {
					http.Error(w, "Request body is empty", http.StatusBadRequest)
					return
				}
				http.Error(w, "Failed to decode request body", http.StatusBadRequest)
				return
			}
			time.Sleep(time.Millisecond * 10) // Simulate round-trip delay
			postedJob.Id = uuid.New().String()

			resbod, err := json.Marshal(postedJob)
			if err != nil {
				http.Error(w, "Failed to encode response", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			w.Header().Set("Content-Type", "application/hal+json")
			_, _ = w.Write(resbod)
		case http.MethodGet:
			defer r.Body.Close()
			jobId := strings.TrimPrefix(r.URL.Path, "/encoreJobs/")
			if jobId == "" {
				http.Error(w, "Job ID is required", http.StatusBadRequest)
				return
			}
			job := encoreJob("test-job", "http://example.com/test.mp4", "s3://example.com/transcoding-output/test-job/")
			time.Sleep(time.Millisecond * 10) // Simulate round-trip delay
			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/hal+json")
			_, _ = w.Write(job)
		}
	}))

}

func validateRequest(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	if r.Header.Get("Content-Type") != "application/json" {
		return false
	}
	if r.Header.Get("Accept") != "application/hal+json" {
		return false
	}

	return true
}

// might not be needed
func encoreJob(externalId string, inputFile, outputFolder string) []byte {
	return []byte(`{
		"id": "` + uuid.New().String() + `",
		"externalId": "` + externalId + `",
		"profile": "test-profile",
		"outputFolder": "` + outputFolder + `",
		"baseName": "test-creative-id",
		"progressCallbackUri": "http://example.com/encoreCallback",
		"inputs": [
			{
				"uri": "` + inputFile + `",
				"seekTo": 0.0,
				"copyTs": true,
				"mediaType": "AudioVideo"
			}
		]
	}`)
}
