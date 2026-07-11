package mqttdump

import (
	"errors"
	"testing"
)

func TestClientStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		isConnected bool
		lastErr     error
	}{
		{
			name:        "connected without error",
			isConnected: true,
			lastErr:     nil,
		},
		{
			name:        "disconnected without error",
			isConnected: false,
			lastErr:     nil,
		},
		{
			name:        "disconnected with error",
			isConnected: false,
			lastErr:     errors.New("connection failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := &Client{
				isConnected: tt.isConnected,
				lastError:   tt.lastErr,
			}

			gotConnected, gotErr := c.Status()
			if gotConnected != tt.isConnected {
				t.Errorf("Status() gotConnected = %v, want %v", gotConnected, tt.isConnected)
			}
			if (gotErr == nil && tt.lastErr != nil) || (gotErr != nil && tt.lastErr == nil) || (gotErr != nil && tt.lastErr != nil && gotErr.Error() != tt.lastErr.Error()) {
				t.Errorf("Status() gotErr = %v, want %v", gotErr, tt.lastErr)
			}
		})
	}
}
