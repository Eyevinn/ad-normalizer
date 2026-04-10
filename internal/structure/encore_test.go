package structure

import (
	"testing"

	"github.com/matryer/is"
)


func TestHasAudioOutput(t *testing.T) {
	is := is.New(t)

	t.Run("no outputs", func(t *testing.T) {
		is := is.New(t)
		ej := EncoreJob{}
		is.Equal(ej.HasAudioOutput(), false)
	})

	t.Run("outputs without audio streams", func(t *testing.T) {
		is := is.New(t)
		ej := EncoreJob{
			Outputs: []EncoreOutput{
				{VideoStreams: []EncoreVideoStream{{Codec: "h264"}}},
			},
		}
		is.Equal(ej.HasAudioOutput(), false)
	})

	t.Run("output with audio stream", func(t *testing.T) {
		is := is.New(t)
		ej := EncoreJob{
			Outputs: []EncoreOutput{
				{AudioStreams: []EncoreAudioStream{{Codec: "aac"}}},
			},
		}
		is.Equal(ej.HasAudioOutput(), true)
	})

	t.Run("mixed outputs, one has audio", func(t *testing.T) {
		is := is.New(t)
		ej := EncoreJob{
			Outputs: []EncoreOutput{
				{VideoStreams: []EncoreVideoStream{{Codec: "h264"}}},
				{AudioStreams: []EncoreAudioStream{{Codec: "aac"}}},
			},
		}
		is.Equal(ej.HasAudioOutput(), true)
	})
}

func TestGetFrameRates(t *testing.T) {
	is := is.New(t)
	ej := EncoreJob{
		Outputs: []EncoreOutput{{
			VideoStreams: []EncoreVideoStream{
				{FrameRate: "25"},
			},
		}, {
			VideoStreams: []EncoreVideoStream{
				{FrameRate: "25"},
			},
		}, {
			VideoStreams: []EncoreVideoStream{
				{FrameRate: "50"},
			},
		},
			{
				VideoStreams: []EncoreVideoStream{
					{FrameRate: "50/1"},
				},
			},
			{
				VideoStreams: []EncoreVideoStream{
					{FrameRate: "30/1.001"},
				},
			},
		},
	}
	frameRates := ej.GetFrameRates()
	is.Equal(len(frameRates), 3)
	is.Equal(frameRates[0], 25.0)
	is.Equal(frameRates[1], 29.97)
	is.Equal(frameRates[2], 50.0)
}
