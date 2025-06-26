package serve

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/matryer/is"
)

func TestPackagingSuccess(t *testing.T) {
	is := is.New(t)
	failureEvent := `{"jobId":"test-job-id","url":"http://encore-example.osaas.io/"}`
	req, err := http.NewRequest("POST", "/packagingSuccess", bytes.NewBufferString(failureEvent))
	is.NoErr(err)
	rr := httptest.NewRecorder()
	api.HandlePackagingFailure(rr, req)
	is.Equal(rr.Code, http.StatusOK)
}
