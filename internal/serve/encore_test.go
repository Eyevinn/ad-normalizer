package serve

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
			reqBody, err := json.Marshal(c.progressUpdate)
			is.NoErr(err)
			req, err := http.NewRequest("POST", "/encore/callback", bytes.NewBuffer(reqBody))
			is.NoErr(err)
			rr := httptest.NewRecorder()
			api.HandleEncoreCallback(rr, req)
			is.Equal(rr.Code, http.StatusOK)
			is.Equal(storeStub.sets, c.expectSets)
			is.Equal(storeStub.deletes, c.expectDeletes)
			is.Equal(storeStub.gets, c.expectGets)
			storeStub.reset()
		})
	}
}
