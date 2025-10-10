package structure

import "slices"

type EncoreJob struct {
	Id                  string         `json:"id,omitempty"`
	ExternalId          string         `json:"externalId,omitempty"`
	Profile             string         `json:"profile"`
	OutputFolder        string         `json:"outputFolder"`
	BaseName            string         `json:"baseName"`
	Status              string         `json:"status,omitempty"`
	Inputs              []EncoreInput  `json:"inputs,omitempty"`
	Outputs             []EncoreOutput `json:"output,omitempty"`
	ProgressCallbackUri string         `json:"progressCallbackUri,omitempty"`
	Message             string         `json:"message,omitempty"`
}

func (ep *EncoreJob) GetFrameRates() []float64 {
	// In most cases, this is an overallocation, but it reduces
	// the amount of times we need to re-alloc the slice
	framerates := make([]float64, 0, len(ep.Outputs))
	for _, o := range ep.Outputs {
		for _, vs := range o.VideoStreams {
			if vs.FrameRate != "" {
				framerates = append(framerates, ParseFrameRate(vs.FrameRate))
			}
		}
	}
	// Sorting the slices allows us to use slices.Compact to remove duplicates
	slices.Sort(framerates)
	return slices.Compact(framerates)
}

type EncoreInput struct {
	Uri       string  `json:"uri"`
	SeekTo    float64 `json:"seekTo,omitempty"`
	CopyTs    bool    `json:"copyTs"`
	MediaType string  `json:"type"`
}

type EncoreOutput struct {
	MediaType      string              `json:"type"`
	Format         string              `json:"format"`
	File           string              `json:"file"`
	FileSize       int64               `json:"fileSize"`
	OverallBitrate int64               `json:"overallBitrate"`
	VideoStreams   []EncoreVideoStream `json:"videoStreams"`
	AudioStreams   []EncoreAudioStream `json:"audioStreams"`
}

type EncoreVideoStream struct {
	Codec     string `json:"codec"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	FrameRate string `json:"frameRate"`
}

type EncoreAudioStream struct {
	Codec        string `json:"codec"`
	Channels     int    `json:"channels"`
	SamplingRage int    `json:"samplingRate"`
	Profile      string `json:"profile"`
}
