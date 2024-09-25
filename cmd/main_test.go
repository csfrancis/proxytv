package main

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestSafeUrlHookFire(t *testing.T) {
	hook := &safeURLHook{}

	testCases := []struct {
		name     string
		inputURL string
		expected string
	}{
		{
			name:     "URL with username and password",
			inputURL: "http://example.com/path?username=user&password=pass",
			expected: "http://example.com/path?username=REDACTED&password=REDACTED",
		},
		{
			name:     "URL with token",
			inputURL: "http://example.com/path?token=secrettoken",
			expected: "http://example.com/path?token=REDACTED",
		},
		{
			name:     "URL with multiple path segments",
			inputURL: "http://example.com/path1/path2/path3?param=value",
			expected: "http://example.com/XXXX/XXXX/path3?param=value",
		},
		{
			name:     "URL with no sensitive information",
			inputURL: "http://example.com/path?param=value",
			expected: "http://example.com/path?param=value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			entry := &log.Entry{
				Data: log.Fields{
					"url": tc.inputURL,
				},
			}

			err := hook.Fire(entry)
			assert.NoError(t, err)

			assert.Equal(t, tc.expected, entry.Data["url"])
		})
	}
}
