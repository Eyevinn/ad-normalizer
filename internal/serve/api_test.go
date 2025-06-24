package serve

import (
	"os"
	"testing"

	"github.com/matryer/is"
)

type StoreStub struct {
	mockStore map[string]string
}

func (s *StoreStub) Get(key string) (string, bool, error) {
	if value, exists := s.mockStore[key]; exists {
		return value, true, nil
	}
	return "", false, nil
}

func (s *StoreStub) Set(key string, value string, ttl ...int) error {
	s.mockStore[key] = value
	return nil
}

var api *API

func TestMain(m *testing.M) {
	store := &StoreStub{
		mockStore: make(map[string]string),
	}
	// Initialize the API with the mock store
	api = NewAPI(store, "http://example.com/adserver", "http://example.com/encore", "http://example.com/assets")

	// Run the tests
	exitCode := m.Run()

	// Clean up if necessary
	os.Exit(exitCode)
}

func TestReplaceVast(t *testing.T) {
	// This function should contain the test logic for the ReplaceVast function
	// It should set up the necessary mocks and expectations, call the function,
	// and then assert that the results are as expected.
	t.Skip("Test not implemented yet")
}

func TestReplaceVmap(t *testing.T) {
	is := is.New(t)
	f, err := os.Open("../test_data/testVmap.xml")
	defer func() {
		_ = f.Close()
	}()

	is.NoErr(err)
	// This function should contain the test logic for the ReplaceVmap function
	// It should set up the necessary mocks and expectations, call the function,
	// and then assert that the results are as expected.

}
