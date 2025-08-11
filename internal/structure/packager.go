package structure

type PackagingSuccessBody struct {
	Url        string `json:"url"`
	JobId      string `json:"jobId"`
	OutputPath string `json:"outputPath"`
}

type PackagingFailureBody struct {
	Message FailMessage `json:"message"`
}

type FailMessage struct {
	JobId string `json:"jobId"`
}

type PackagingQueueMessage struct {
	JobId string `json:"jobId"`
	Url   string `json:"url"`
}
