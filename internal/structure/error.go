package structure

import "strconv"

type AdServerError struct {
	StatusCode int
	Message    string
}

func (e AdServerError) Error() string {
	return "error " + strconv.Itoa(e.StatusCode) + ": " + e.Message
}
