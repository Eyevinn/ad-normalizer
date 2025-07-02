package structure

import (
	"net/url"
	"testing"

	"github.com/matryer/is"
)

func TestCreatePackagerUrl(t *testing.T) {
	is := is.New(t)
	assetServerUrl, err := url.Parse("http://cdn.osaas.io")
	is.NoErr(err)
	outputFolder := "assets/1234567890abcdef"
	basename := "test-asset"
	assetUrl := CreatePackageUrl(*assetServerUrl, outputFolder, basename)
	is.Equal(assetUrl.Host, "cdn.osaas.io")
	is.Equal(assetUrl.Path, "assets/1234567890abcdef/test-asset.m3u8")
}

func TestTranscodeInfoFromEncoreJob(t *testing.T) {
	is := is.New(t)
	testJob := EncoreJob{
		Status:       "SUCCESSFUL",
		BaseName:     "test-asset",
		OutputFolder: "assets/1234567890abcdef",
		Outputs: []EncoreOutput{
			{
				VideoStreams: []EncoreVideoStream{
					{
						Width:     1920,
						Height:    1080,
						Codec:     "AVC",
						FrameRate: "25",
					},
				},
			},
		},
	}
	assetServerUrl, err := url.Parse("http://cdn.osaas.io")
	is.NoErr(err)
	jitPackage := true
	res, err := TranscodeInfoFromEncoreJob(&testJob, jitPackage, *assetServerUrl)
	is.NoErr(err)
	is.Equal(res.AspectRatio, "16:9")
	is.Equal(res.FrameRates, []float64{25.0})
	is.Equal(res.Status, "COMPLETED")
	is.Equal(res.Url, "http://cdn.osaas.io/assets/1234567890abcdef/test-asset.m3u8")
}

func TestGetTranscodeStatus(t *testing.T) {
	is := is.New(t)
	cases := []struct {
		name       string
		job        EncoreJob
		jitPackage bool
		expected   string
	}{
		{
			name: "Successful Transcode no JIT package",
			job: EncoreJob{
				Status: "SUCCESSFUL",
			},
			jitPackage: false,
			expected:   "PACKAGING",
		},
		{
			name: "Successful Transcode with JIT package",
			job: EncoreJob{
				Status: "SUCCESSFUL",
			},
			jitPackage: true,
			expected:   "COMPLETED",
		},
		{
			name: "Failed Transcode",
			job: EncoreJob{
				Status: "FAILED",
			},
			jitPackage: false,
			expected:   "FAILED",
		},
		{
			name: "Cancelled Transcode",
			job: EncoreJob{
				Status: "FAILED",
			},
			jitPackage: false,
			expected:   "FAILED",
		},
		{
			name: "In progress Transcode",
			job: EncoreJob{
				Status: "IN_PROGRESS",
			},
			jitPackage: false,
			expected:   "IN_PROGRESS",
		},
		{
			name: "Queued Transcode",
			job: EncoreJob{
				Status: "QUEUED",
			},
			jitPackage: false,
			expected:   "IN_PROGRESS",
		},
		{
			name: "New Transcode",
			job: EncoreJob{
				Status: "NEW",
			},
			jitPackage: false,
			expected:   "IN_PROGRESS",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status := c.job.GetTranscodeStatus(c.jitPackage)
			is.Equal(status, c.expected)
		})
	}
}

func TestCalculateAspectRatio(t *testing.T) {
	is := is.New(t)
	cases := []struct {
		width, height int
		expected      string
	}{
		{width: 1920, height: 1080, expected: "16:9"},
		{width: 1280, height: 720, expected: "16:9"},
		{width: 640, height: 480, expected: "4:3"},
		{width: 0, height: 0, expected: "0:0"},
	}
	for _, c := range cases {
		t.Run(c.expected, func(t *testing.T) {
			ratio := calculateAspectRatio(c.width, c.height)
			is.Equal(ratio, c.expected)
		})
	}
}
