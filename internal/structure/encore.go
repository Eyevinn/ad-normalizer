package structure

type EncoreJob struct {
	Id                  string         `json:"id"`
	ExternalId          string         `json:"externalId"`
	Profile             string         `json:"profile"`
	OutputFolder        string         `json:"outputFolder"`
	BaseName            string         `json:"baseName"`
	Status              string         `json:"status"`
	Inputs              []EncoreInput  `json:"inputs"`
	Outputs             []EncoreOutput `json:"outputs"`
	ProgressCallbackUri string         `json:"progressCallbackUri"`
}

type EncoreInput struct {
	Uri       string `json:"uri"`
	SeekTo    int    `json:"seekTo"`
	CopyTs    bool   `json:"copyTs"`
	MediaType string `json:"type"`
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
