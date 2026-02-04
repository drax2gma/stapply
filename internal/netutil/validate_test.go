package netutil

import "testing"

func TestNormalizeNATSURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "fqdn only",
			input:    "example.com",
			expected: "nats://example.com:4222",
		},
		{
			name:     "ip only",
			input:    "127.0.0.1",
			expected: "nats://127.0.0.1:4222",
		},
		{
			name:     "fqdn with port",
			input:    "example.com:1234",
			expected: "nats://example.com:1234",
		},
		{
			name:     "scheme with fqdn",
			input:    "nats://example.com",
			expected: "nats://example.com:4222",
		},
		{
			name:     "full url",
			input:    "nats://example.com:1234",
			expected: "nats://example.com:1234",
		},
		{
			name:     "tls scheme",
			input:    "tls://example.com",
			expected: "tls://example.com:4222",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeNATSURL(tt.input)
			if got != tt.expected {
				t.Errorf("NormalizeNATSURL(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
