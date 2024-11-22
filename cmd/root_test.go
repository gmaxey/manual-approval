package cmd

import (
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_arguments(t *testing.T) {
	prevArgs := os.Args
	defer func() {
		os.Args = prevArgs
	}()

	tests := []struct {
		name string
		args []string
		env  map[string]string
		err  string
	}{
		{
			name: "wrong argument",
			args: []string{"manual-approval", "wrong"},
			err:  "unknown arguments: [wrong]",
		},
		{
			name: "wrong flag",
			args: []string{"manual-approval", "--wrong", "something wrong"},
			err:  "unknown flag: --wrong",
		},
		{
			name: "wrong handler argument",
			args: []string{"manual-approval", "--handler", "something wrong"},
			err:  "unsupported handler type: something wrong",
		},
		{
			name: "missed handler argument",
			args: []string{"manual-approval"},
			err:  "unsupported handler type: something wrong",
		},
		{
			name: "init - no URL environment variable",
			args: []string{"manual-approval", "--handler", "init"},
			env:  map[string]string{"CLOUDBEES_STATUS": "/tmp/fake-status" + strconv.Itoa(time.Now().Nanosecond())},
			err:  "URL environment variable missing",
		},
		{
			name: "init - wrong DISALLOW_LAUNCHED_BY_USER environment variable",
			args: []string{"manual-approval", "--handler", "init"},
			env:  map[string]string{"DISALLOW_LAUNCHED_BY_USER": "not a boolean"},
			err:  "strconv.ParseBool: parsing \"not a boolean\": invalid syntax",
		},
		{
			name: "init - wrong NOTIFY_ALL_ELIGIBLE_USERS environment variable",
			args: []string{"manual-approval", "--handler", "init"},
			env:  map[string]string{"NOTIFY_ALL_ELIGIBLE_USERS": "not a boolean"},
			err:  "strconv.ParseBool: parsing \"not a boolean\": invalid syntax",
		},
		{
			name: "init - no API_TOKEN environment variable",
			args: []string{"manual-approval", "--handler", "init"},
			env:  map[string]string{"URL": "http://test.com", "CLOUDBEES_STATUS": "/tmp/fake-status.out" + strconv.Itoa(time.Now().Nanosecond())},
			err:  "API_TOKEN environment variable missing",
		},
		{
			name: "init - no CLOUDBEES_STATUS environment variable",
			args: []string{"manual-approval", "--handler", "init"},
			env:  map[string]string{"URL": "http://test.com", "API_TOKEN": "12345"},
			err:  "CLOUDBEES_STATUS environment variable missing",
		},
		{
			name: "callback - no PAYLOAD environment variable",
			args: []string{"manual-approval", "--handler", "callback"},
			env:  map[string]string{},
			err:  "PAYLOAD environment variable missing",
		},
		{
			name: "callback - no URL environment variable",
			args: []string{"manual-approval", "--handler", "callback"},
			env: map[string]string{"PAYLOAD": "{\"status\": \"UPDATE_MANUAL_APPROVAL_STATUS_APPROVED\", \"comments\": \"lgtm\", \"respondedOn\": \"some-time\", \"userName\": \"Some One\"}",
				"CLOUDBEES_STATUS": "/tmp/fake-status.out" + strconv.Itoa(time.Now().Nanosecond())},
			err: "URL environment variable missing",
		},
		{
			name: "callback - no API_TOKEN environment variable",
			args: []string{"manual-approval", "--handler", "callback"},
			env: map[string]string{"PAYLOAD": "{\"status\": \"UPDATE_MANUAL_APPROVAL_STATUS_APPROVED\", \"comments\": \"lgtm\", \"respondedOn\": \"some-time\", \"userName\": \"Some One\"}",
				"URL": "http://test.com", "CLOUDBEES_STATUS": "/tmp/fake-status.out" + strconv.Itoa(time.Now().Nanosecond())},
			err: "API_TOKEN environment variable missing",
		},
		{
			name: "cancel - no CANCELLATION_REASON environment variable",
			args: []string{"manual-approval", "--handler", "cancel"},
			env:  map[string]string{},
			err:  "CANCELLATION_REASON environment variable missing",
		},
		{
			name: "cancel - no URL environment variable",
			args: []string{"manual-approval", "--handler", "cancel"},
			env:  map[string]string{"CANCELLATION_REASON": "test reason"},
			err:  "URL environment variable missing",
		},
		{
			name: "cancel - no API_TOKEN environment variable",
			args: []string{"manual-approval", "--handler", "cancel"},
			env:  map[string]string{"CANCELLATION_REASON": "test reason", "URL": "http://test.com"},
			err:  "API_TOKEN environment variable missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare
			os.Args = tt.args
			for k, v := range tt.env {
				os.Setenv(k, v)
				defer func(k string) {
					os.Unsetenv(k)
				}(k)
			}

			// Run
			err := cmd.Execute()

			// Verify
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Equal(t, tt.err, err.Error())
			}
		})
	}
}
