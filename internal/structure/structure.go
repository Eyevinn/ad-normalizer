package structure

type ManifestAsset struct {
	CreativeId        string
	MasterPlaylistUrl string
}

// Used for HLS interstitials
type AssetDescription struct {
	Uri      string `json:"URI"`
	Duration string `json:"DURATION"`
}

type EncoreJob struct {
	Id         string `json:"id"`
	ExternalId string `json:"ExternalId"`
}

type TranscodeStatus int

const (
	Unknown TranscodeStatus = iota
	Failed
	InProgress
	Transcoding
	Completed
	Packaging
)

type TranscodeInfo struct {
	Url         string `json:"url"`
	AspectRatio string `json:"aspectRatio"`
	FrameRates  []int  `json:"frameRates"`
	Status      string `json:"status"`
}

type JobProgress struct {
	JobId      string `json:"jobId"`
	ExternalId string `json:"externalId"`
	Progress   int    `json:"progress"`
	Status     string `json:"status"`
}

// How can a langugage not have enum support?
func (ts *TranscodeStatus) String() string {
	var res string
	switch *ts {
	case Unknown:
		res = "UNKNOWN"
	case Failed:
		res = "FAILED"
	case InProgress:
		res = "IN_PROGRESS"
	case Completed:
		res = "COMPLETED"
	case Packaging:
		res = "PACKAGING"
	default:
		res = "TRANSCODING"
	}
	return res
}

func (jp *JobProgress) ToTranscodeStatus() TranscodeStatus {
	var res TranscodeStatus
	switch jp.Status {
	case "SUCCESSFUL":
		res = Completed
	case "FAILED":
		res = Failed
	case "IN_PROGRESS":
		res = InProgress
	default:
		res = Transcoding
	}
	return res
}
