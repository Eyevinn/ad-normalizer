package structure

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ManifestAsset struct {
	CreativeId        string
	MasterPlaylistUrl string
	Source            string
}

const DefaultTtl = 3600

// Used for HLS interstitials
type AssetDescription struct {
	Uri      string  `json:"URI"`
	Duration float64 `json:"DURATION"`
}

type TranscodeInfo struct {
	Url         string    `json:"url"`
	AspectRatio string    `json:"aspectRatio"`
	FrameRates  []float64 `json:"frameRates"`
	Status      string    `json:"status"`
	Source      string    `json:"source,omitempty"`
	LastUpdate  int64     `json:"lastUpdate,omitempty"`
	Error       string    `json:"error,omitempty"`
}

func TranscodeInfoFromEncoreJob(job *EncoreJob, jitPackaging bool, assetServerUrl url.URL) (TranscodeInfo, error) {
	jobStatus := job.GetTranscodeStatus(jitPackaging)
	if len(job.Outputs) == 0 {
		return TranscodeInfo{}, fmt.Errorf("no outputs found for job %s", job.Id)
	}
	firstVideoStream := job.Outputs[0].VideoStreams[0]
	width := 1920
	height := 1080
	if firstVideoStream.Width != 0 {
		width = firstVideoStream.Width
	}
	if firstVideoStream.Height != 0 {
		height = firstVideoStream.Height
	}
	aspectRatio := calculateAspectRatio(width, height)
	var vidUrl string
	if jitPackaging {
		packageUrl := CreatePackageUrl(
			assetServerUrl,
			job.OutputFolder,
			job.BaseName,
		)
		vidUrl = packageUrl.String()
	}
	tc := TranscodeInfo{
		Url:         vidUrl,
		AspectRatio: aspectRatio,
		FrameRates:  job.GetFrameRates(),
		Status:      jobStatus,
		Source:      job.Inputs[0].Uri,
		LastUpdate:  time.Now().Unix(),
	}
	if job.Message != "" {
		tc.Error = job.Message
	}
	return tc, nil
}

type EncoreJobProgress struct {
	JobId      string `json:"jobId"`
	ExternalId string `json:"externalId"`
	Progress   int    `json:"progress"`
	Status     string `json:"status"`
}

func (ep *EncoreJob) GetTranscodeStatus(jitPackage bool) string {
	var res string
	switch ep.Status {
	case "SUCCESSFUL":
		if jitPackage {
			res = "COMPLETED"
		} else {
			res = "PACKAGING"
		}
	case "FAILED", "CANCELLED":
		res = "FAILED"
	case "IN_PROGRESS", "QUEUED", "NEW":
		res = "IN_PROGRESS"
	default:
		res = "UNKNOWN"
	}
	return res
}

func calculateAspectRatio(width, height int) string {
	if width == 0 || height == 0 {
		return "0:0"
	}
	gcd := gcd(width, height)
	return strconv.Itoa(width/gcd) + ":" + strconv.Itoa(height/gcd)
}

// Greatest Common Divisor (GCD) using the Euclidean algorithm
func gcd(a, b int) int {
	for b != 0 {
		t := b
		b = a % b
		a = t
	}
	return a
}

// Parses a frame rate string in the format "numerator/denominator"
// and returns the frame rate as a rounded float64.
// Media applications very rarely need more than 2 decimal places
func ParseFrameRate(framerateStr string) float64 {
	parts := strings.Split(framerateStr, "/")
	numerator, parseErr := strconv.ParseFloat(parts[0], 64)
	denominator := 1.0
	if len(parts) == 2 {
		tDen, err2 := strconv.ParseFloat(parts[1], 64)
		if err2 == nil {
			denominator = tDen
		}
	}
	if parseErr == nil {
		frameRate := float64(numerator) / float64(denominator)
		return (math.Round(frameRate*100.0) / 100.0) // Round to 2 decimal places
	}
	return 0.0 // Default or error case
}

func CreatePackageUrl(
	assetServerUrl url.URL,
	outputFolder string,
	baseName string,
) url.URL {
	newUrl := assetServerUrl.JoinPath(
		outputFolder,
		baseName+".m3u8",
	)
	return *newUrl
}
