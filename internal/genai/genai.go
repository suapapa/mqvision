package genai

import (
	"context"
	"io"
	"time"
)

// VisionClient analyzes a JPEG gas-meter image and returns structured read/date.
type VisionClient interface {
	ReadGasGaugePic(ctx context.Context, jpgReader io.Reader) (*GasMeterReadResult, error)
	// ReadGasGaugePicFromURL runs the same analysis using an image reachable at imageURL (e.g. https).
	ReadGasGaugePicFromURL(ctx context.Context, imageURL string) (*GasMeterReadResult, error)
}

type GasMeterReadResult struct {
	Read    string    `json:"read" bson:"read"`
	Date    string    `json:"date" bson:"date"`
	ReadAt  time.Time `json:"read_at,omitempty" bson:"read_at,omitempty"`
	ItTakes string    `json:"it_takes,omitempty" bson:"it_takes,omitempty"`
}
