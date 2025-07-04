package serve

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestPackagingFailure(t *testing.T) {
	is := is.New(t)
	failureEvent := `{"message": {"jobId":"test-job-id","url":"http://encore-example.osaas.io/"}}`
	req, err := http.NewRequest("POST", "/failure", bytes.NewBufferString(failureEvent))
	is.NoErr(err)
	rr := httptest.NewRecorder()
	api.HandlePackagingFailure(rr, req)
	is.Equal(rr.Code, http.StatusOK)
	is.Equal(storeStub.deletes, 1)

	storeStub.reset()
}

func TestPackagingSuccess(t *testing.T) {
	is := is.New(t)
	successEvent := `{
		"jobId": "test-job-id",
    	"url": "https://encore-instance",
    	"outputPath": "/output-folder/assetId/jobId/"
	}`
	req, err := http.NewRequest("POST", "/success", bytes.NewBufferString(successEvent))
	is.NoErr(err)
	rr := httptest.NewRecorder()
	api.HandlePackagingSuccess(rr, req)
	is.Equal(rr.Code, http.StatusOK)
	is.Equal(storeStub.sets, 1)
	tci, ok, err := storeStub.Get("test-job-id")
	is.NoErr(err)
	is.True(ok)
	is.Equal(tci.Status, "COMPLETED")
	is.True(strings.HasSuffix(tci.Url, "index.m3u8"))
}
