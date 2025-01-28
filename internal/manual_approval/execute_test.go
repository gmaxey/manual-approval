package manual_approval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	instructionsInput  = "***instruction***\n`instruction2`\n# instruction3\n## instruction4\n### instruction5\n\n> Blockquotes can contain multiple paragraphs\n>\n> Add a > on the blank lines between the paragraps.\n\n- Rirst item\n- Second Item\n- Third item \n  - Indented item\n  - Indented item\n- Fourth item"
	instructionsOutput = "<p><em><strong>instruction</strong></em>\n<code>instruction2</code></p>\n<h1>instruction3</h1>\n<h2>instruction4</h2>\n<h3>instruction5</h3>\n<blockquote>\n<p>Blockquotes can contain multiple paragraphs</p>\n<p>Add a &gt; on the blank lines between the paragraps.</p>\n</blockquote>\n<ul>\n<li>Rirst item</li>\n<li>Second Item</li>\n<li>Third item\n<ul>\n<li>Indented item</li>\n<li>Indented item</li>\n</ul>\n</li>\n<li>Fourth item</li>\n</ul>\n"
	approvalInputs     = "in1:\\n  type: string\\n  required: true\\n  description: One of the required approver inputs\\nin2:\\n  type: number\\n  description: a numeric input\\nin3:\\n  type: choice\\n  options:\\n    - op1\\n    - op2"
)

func init() {
	debug = true
}

type MockHttpClient struct {
	MockDo func(req *http.Request) (*http.Response, error)
}

func (c *MockHttpClient) Do(req *http.Request) (*http.Response, error) {
	return c.MockDo(req)
}

type MockStdOut struct {
	MockPrintf  func(format string, a ...any)
	MockPrintln func(a ...any)
}

func (c *MockStdOut) Printf(format string, a ...any) {
	c.MockPrintf(format, a...)
}

func (c *MockStdOut) Println(a ...any) {
	c.MockPrintln(a...)
}

func Test_defaultConfig(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		err  string
	}{
		{
			name: "success",
			env:  map[string]string{"URL": "http://test.com", "API_TOKEN": "test"},
			err:  "",
		},
		{
			name: "no API_TOKEN environment variable",
			env:  map[string]string{"URL": "http://test.com"},
			err:  "API_TOKEN environment variable missing",
		},
		{
			name: "no URL environment variable",
			env:  map[string]string{},
			err:  "URL environment variable missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare
			for k, v := range tt.env {
				os.Setenv(k, v)
				defer func(k string) {
					os.Unsetenv(k)
				}(k)
			}

			// Run
			c := Config{}
			apiUrl, apiToken, err := c.defaultConfig()

			// Verify
			if tt.err == "" {
				require.NoError(t, err)
				require.Equal(t, tt.env["URL"], apiUrl)
				require.Equal(t, tt.env["API_TOKEN"], apiToken)
			} else {
				require.Error(t, err)
				require.Equal(t, tt.err, err.Error())
			}
		})
	}
}

func Test_init(t *testing.T) {
	tests := []struct {
		name         string
		reqCheckFunc func(req map[string]interface{})
		respGenFunc  func() (*http.Response, error)
		env          map[string]string
		client       *MockHttpClient
		output       []string
		err          string
	}{
		{
			name: "success",
			reqCheckFunc: func(req map[string]interface{}) {
				require.NotNil(t, req["approvers"])
				require.Equal(t, []interface{}{"123", "user@mail.com"}, req["approvers"])
				require.NotNil(t, req["instructions"])
				require.Equal(t, instructionsInput, req["instructions"].(string))
				require.Equal(t, false, req["disallowLaunchedByUser"].(bool))
				require.Equal(t, false, req["notifyEligibleUsers"].(bool))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{"approvers":[{"userName": "testUserName", "userId": "123", "email": "user@mail.com"}]}`)),
				}, nil
			},
			env: map[string]string{
				"URL":              "http://test.com",
				"API_TOKEN":        "test",
				"CLOUDBEES_STATUS": "/tmp/test-status-out",
				"APPROVERS":        "123,user@mail.com",
				"INSTRUCTIONS":     instructionsInput,
			},
			output: []string{
				"Waiting for approval from one of the following: testUserName\n",
				"Instructions:\n<p><em><strong>instruction</strong></em>\n<code>instruction2</code></p>\n<h1>instruction3</h1>\n<h2>instruction4</h2>\n<h3>instruction5</h3>\n<blockquote>\n<p>Blockquotes can contain multiple paragraphs</p>\n<p>Add a &gt; on the blank lines between the paragraps.</p>\n</blockquote>\n<ul>\n<li>Rirst item</li>\n<li>Second Item</li>\n<li>Third item\n<ul>\n<li>Indented item</li>\n<li>Indented item</li>\n</ul>\n</li>\n<li>Fourth item</li>\n</ul>\n\n",
			},
			err: "",
		},
		{
			name: "success with inputs",
			reqCheckFunc: func(req map[string]interface{}) {
				require.NotNil(t, req["approvers"])
				require.Equal(t, []interface{}{"123", "user@mail.com"}, req["approvers"])
				require.NotNil(t, req["instructions"])
				require.Equal(t, instructionsInput, req["instructions"].(string))
				require.NotNil(t, req["approvalInputs"])
				require.Equal(t, approvalInputs, req["approvalInputs"].(string))
				require.Equal(t, false, req["disallowLaunchedByUser"].(bool))
				require.Equal(t, false, req["notifyEligibleUsers"].(bool))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{"approvers":[{"userName": "testUserName", "userId": "123", "email": "user@mail.com"}]}`)),
				}, nil
			},
			env: map[string]string{
				"URL":              "http://test.com",
				"API_TOKEN":        "test",
				"CLOUDBEES_STATUS": "/tmp/test-status-out",
				"APPROVERS":        "123,user@mail.com",
				"INSTRUCTIONS":     instructionsInput,
				"INPUTS":           approvalInputs,
			},
			output: []string{
				"Waiting for approval from one of the following: testUserName\n",
				"Instructions:\n<p><em><strong>instruction</strong></em>\n<code>instruction2</code></p>\n<h1>instruction3</h1>\n<h2>instruction4</h2>\n<h3>instruction5</h3>\n<blockquote>\n<p>Blockquotes can contain multiple paragraphs</p>\n<p>Add a &gt; on the blank lines between the paragraps.</p>\n</blockquote>\n<ul>\n<li>Rirst item</li>\n<li>Second Item</li>\n<li>Third item\n<ul>\n<li>Indented item</li>\n<li>Indented item</li>\n</ul>\n</li>\n<li>Fourth item</li>\n</ul>\n\n",
			},
			err: "",
		},
		{
			name: "success with disallowLaunchedByUser",
			reqCheckFunc: func(req map[string]interface{}) {
				require.NotNil(t, req["approvers"])
				require.Equal(t, []interface{}{"123", "user@mail.com"}, req["approvers"])
				require.NotNil(t, req["instructions"])
				require.Equal(t, instructionsInput, req["instructions"].(string))
				require.Equal(t, true, req["disallowLaunchedByUser"].(bool))
				require.Equal(t, false, req["notifyEligibleUsers"].(bool))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{"approvers":[{"userName": "testUserName", "userId": "123", "email": "user@mail.com"}]}`)),
				}, nil
			},
			env: map[string]string{
				"URL":                       "http://test.com",
				"API_TOKEN":                 "test",
				"CLOUDBEES_STATUS":          "/tmp/test-status-out",
				"APPROVERS":                 "123,user@mail.com",
				"INSTRUCTIONS":              instructionsInput,
				"DISALLOW_LAUNCHED_BY_USER": "true",
			},
			output: []string{
				"Waiting for approval from one of the following: testUserName\n",
				"Instructions:\n<p><em><strong>instruction</strong></em>\n<code>instruction2</code></p>\n<h1>instruction3</h1>\n<h2>instruction4</h2>\n<h3>instruction5</h3>\n<blockquote>\n<p>Blockquotes can contain multiple paragraphs</p>\n<p>Add a &gt; on the blank lines between the paragraps.</p>\n</blockquote>\n<ul>\n<li>Rirst item</li>\n<li>Second Item</li>\n<li>Third item\n<ul>\n<li>Indented item</li>\n<li>Indented item</li>\n</ul>\n</li>\n<li>Fourth item</li>\n</ul>\n\n",
			},
			err: "",
		},
		{
			name: "failure with invalid disallowLaunchedByUser",
			reqCheckFunc: func(req map[string]interface{}) {
				require.NotNil(t, req["approvers"])
				require.Equal(t, []interface{}{"123", "user@mail.com"}, req["approvers"])
				require.NotNil(t, req["instructions"])
				require.Equal(t, instructionsInput, req["instructions"].(string))
				require.Equal(t, true, req["disallowLaunchedByUser"].(bool))
				require.Equal(t, false, req["notifyEligibleUsers"].(bool))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{"approvers":[{"userName": "testUserName", "userId": "123", "email": "user@mail.com"}]}`)),
				}, nil
			},
			env: map[string]string{
				"URL":                       "http://test.com",
				"API_TOKEN":                 "test",
				"CLOUDBEES_STATUS":          "/tmp/test-status-out",
				"APPROVERS":                 "123,user@mail.com",
				"INSTRUCTIONS":              instructionsInput,
				"DISALLOW_LAUNCHED_BY_USER": "invalid boolean",
			},
			output: nil,
			err:    "strconv.ParseBool: parsing \"invalid boolean\": invalid syntax",
		},
		{
			name: "success with notifyEligibleUsers",
			reqCheckFunc: func(req map[string]interface{}) {
				require.NotNil(t, req["approvers"])
				require.Equal(t, []interface{}{"123", "user@mail.com"}, req["approvers"])
				require.NotNil(t, req["instructions"])
				require.Equal(t, instructionsInput, req["instructions"].(string))
				require.Equal(t, false, req["disallowLaunchedByUser"].(bool))
				require.Equal(t, true, req["notifyEligibleUsers"].(bool))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{"approvers":[{"userName": "testUserName", "userId": "123", "email": "user@mail.com"}]}`)),
				}, nil
			},
			env: map[string]string{
				"URL":                       "http://test.com",
				"API_TOKEN":                 "test",
				"CLOUDBEES_STATUS":          "/tmp/test-status-out",
				"APPROVERS":                 "123,user@mail.com",
				"INSTRUCTIONS":              instructionsInput,
				"NOTIFY_ALL_ELIGIBLE_USERS": "true",
			},
			output: []string{
				"Waiting for approval from one of the following: testUserName\n",
				"Instructions:\n<p><em><strong>instruction</strong></em>\n<code>instruction2</code></p>\n<h1>instruction3</h1>\n<h2>instruction4</h2>\n<h3>instruction5</h3>\n<blockquote>\n<p>Blockquotes can contain multiple paragraphs</p>\n<p>Add a &gt; on the blank lines between the paragraps.</p>\n</blockquote>\n<ul>\n<li>Rirst item</li>\n<li>Second Item</li>\n<li>Third item\n<ul>\n<li>Indented item</li>\n<li>Indented item</li>\n</ul>\n</li>\n<li>Fourth item</li>\n</ul>\n\n",
			},
			err: "",
		},
		{
			name: "success with invalid notifyEligibleUsers",
			reqCheckFunc: func(req map[string]interface{}) {
				require.NotNil(t, req["approvers"])
				require.Equal(t, []interface{}{"123", "user@mail.com"}, req["approvers"])
				require.NotNil(t, req["instructions"])
				require.Equal(t, instructionsInput, req["instructions"].(string))
				require.Equal(t, false, req["disallowLaunchedByUser"].(bool))
				require.Equal(t, true, req["notifyEligibleUsers"].(bool))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{"approvers":[{"userName": "testUserName", "userId": "123", "email": "user@mail.com"}]}`)),
				}, nil
			},
			env: map[string]string{
				"URL":                       "http://test.com",
				"API_TOKEN":                 "test",
				"CLOUDBEES_STATUS":          "/tmp/test-status-out",
				"APPROVERS":                 "123,user@mail.com",
				"INSTRUCTIONS":              instructionsInput,
				"NOTIFY_ALL_ELIGIBLE_USERS": "invalid boolean",
			},
			output: nil,
			err:    "strconv.ParseBool: parsing \"invalid boolean\": invalid syntax",
		},
		{
			name: "failure",
			reqCheckFunc: func(req map[string]interface{}) {
				require.NotNil(t, req["approvers"])
				require.Equal(t, []interface{}{"123", "user@mail.com"}, req["approvers"])
				require.NotNil(t, req["instructions"])
				require.Equal(t, instructionsInput, req["instructions"].(string))
				require.Equal(t, false, req["disallowLaunchedByUser"].(bool))
				require.Equal(t, false, req["notifyEligibleUsers"].(bool))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 500,
					Status:     "500 Internal Server Error",
					Body:       io.NopCloser(bytes.NewBufferString(`wrong parameter`)),
				}, nil
			},
			env: map[string]string{
				"URL":              "http://test.com",
				"API_TOKEN":        "test",
				"CLOUDBEES_STATUS": "/tmp/test-status-out",
				"APPROVERS":        "123,user@mail.com",
				"INSTRUCTIONS":     instructionsInput,
			},
			output: []string{
				"ERROR: API call failed with error: 'failed to send event: \nPOST http://test.com/v1/workflows/approval\nHTTP/500 500 Internal Server Error\n'\n",
				"ERROR: API response: 'wrong parameter'\n",
			},
			err: "failed to send event: \nPOST http://test.com/v1/workflows/approval\nHTTP/500 500 Internal Server Error\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare
			for k, v := range tt.env {
				os.Setenv(k, v)
				defer func(k string) {
					os.Unsetenv(k)
				}(k)
			}

			var testOutput []string

			// Run
			c := Config{
				Client: &MockHttpClient{
					MockDo: func(req *http.Request) (*http.Response, error) {
						require.NotNil(t, req)
						require.Equal(t, "POST", req.Method)
						require.Equal(t, "http://test.com/v1/workflows/approval", req.URL.String())
						require.Equal(t, "application/json", req.Header.Get("Content-Type"))
						require.Equal(t, "application/json", req.Header.Get("Accept"))
						require.Contains(t, req.Header.Get("Authorization"), "Bearer ")

						reqBody := make(map[string]interface{})
						bodyReader, err := req.GetBody()
						require.NoError(t, err)
						body, err := io.ReadAll(bodyReader)
						require.NoError(t, err)
						err = json.Unmarshal(body, &reqBody)
						require.NoError(t, err)

						// Check parsed request body
						tt.reqCheckFunc(reqBody)

						// Generate response
						return tt.respGenFunc()
					},
				},
				Output: &MockStdOut{
					MockPrintf: func(format string, a ...any) {
						testOutput = append(testOutput, fmt.Sprintf(format, a...))
						fmt.Printf(format, a...)
					},
					MockPrintln: func(a ...any) {
						testOutput = append(testOutput, fmt.Sprintln(a...))
						fmt.Println(a...)
					},
				},
			}
			err := c.init()

			// Verify
			if tt.err == "" {
				require.NoError(t, err)
				out, ferr := os.ReadFile(tt.env["CLOUDBEES_STATUS"])
				require.NoError(t, ferr)
				require.Equal(t, "{\"message\":\"Waiting for approval from approvers\",\"status\":\"PENDING_APPROVAL\"}", string(out))
			} else {
				require.Error(t, err)
				require.Equal(t, tt.err, err.Error())
			}

			require.True(t, slices.Equal(tt.output, testOutput))
		})
	}
}

func Test_callback(t *testing.T) {
	tests := []struct {
		name              string
		reqCheckFunc      func(req map[string]interface{})
		respGenFunc       func() (*http.Response, error)
		env               map[string]string
		client            *MockHttpClient
		statusInFile      string
		commentsInOutput  string
		inputValsInOutput string
		output            []string
		err               string
	}{
		{
			name: "success APPROVED",
			reqCheckFunc: func(req map[string]interface{}) {
				require.Equal(t, "UPDATE_MANUAL_APPROVAL_STATUS_APPROVED", req["status"].(string))
				require.Equal(t, "test comments1", req["comments"].(string))
				require.Equal(t, "123", req["userId"].(string))
				require.Equal(t, "testUserName", req["userName"].(string))
				require.Equal(t, "2009-11-10T23:00:00Z", req["respondedOn"].(string))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
				}, nil
			},
			env: map[string]string{
				"URL":               "http://test.com",
				"API_TOKEN":         "test",
				"CLOUDBEES_STATUS":  "/tmp/test-status-out",
				"CLOUDBEES_OUTPUTS": "/tmp/test-outputs",
				"PAYLOAD":           "{\"status\":\"UPDATE_MANUAL_APPROVAL_STATUS_APPROVED\",\"comments\":\"test comments1\",\"userId\":\"123\",\"userName\":\"testUserName\",\"respondedOn\":\"2009-11-10T23:00:00Z\",\"inputs\": [{\"name\":\"reqBoolInput\",\"value\":true,\"is_default\":true},{\"name\":\"reqStrInput\",\"value\":\"Streamline Workflows, Speed Up Software Delivery, and Enable Continuous Improvement.\\nCloudBees empowers developers by reducing time spent on non-coding tasks with self-service automation pipelines, speeding up software delivery with advanced CI/CD capabilities, and fostering innovation through feature management and real-time feedback loops.\",\"is_default\":true},{\"name\":\"reqNumInput\",\"value\":99.33,\"is_default\":false}]}",
			},
			statusInFile:      "{\"message\":\"Successfully changed workflow manual approval status\",\"status\":\"APPROVED\"}",
			commentsInOutput:  "test comments1",
			inputValsInOutput: "{\"reqBoolInput\":true,\"reqNumInput\":99.33,\"reqStrInput\":\"Streamline Workflows, Speed Up Software Delivery, and Enable Continuous Improvement.\\nCloudBees empowers developers by reducing time spent on non-coding tasks with self-service automation pipelines, speeding up software delivery with advanced CI/CD capabilities, and fostering innovation through feature management and real-time feedback loops.\"}",
			output: []string{
				"Approved by testUserName on 2009-11-10T23:00:00Z with comments:\ntest comments1\n",
				"\nInput Parameters:\n",
				"------------------\n",
				" reqBoolInput: true (default) \n",
				" reqStrInput: Streamline Workflows, Speed Up Software Delivery, and Enable Continuous Improvement.<br/>CloudBees empowers developers by reducing time spent on non-coding tasks with self-service automation pipelines, speeding up software delivery with advanced CI/CD capabilities, and fostering innovation through feature management and real-time feedback loops. (default) \n",
				" reqNumInput: 99.33 \n",
			},
			err: "",
		},
		{
			name: "success APPROVED - empty input values",
			reqCheckFunc: func(req map[string]interface{}) {
				require.Equal(t, "UPDATE_MANUAL_APPROVAL_STATUS_APPROVED", req["status"].(string))
				require.Equal(t, "test comments1", req["comments"].(string))
				require.Equal(t, "123", req["userId"].(string))
				require.Equal(t, "testUserName", req["userName"].(string))
				require.Equal(t, "2009-11-10T23:00:00Z", req["respondedOn"].(string))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
				}, nil
			},
			env: map[string]string{
				"URL":               "http://test.com",
				"API_TOKEN":         "test",
				"CLOUDBEES_STATUS":  "/tmp/test-status-out",
				"CLOUDBEES_OUTPUTS": "/tmp/test-outputs",
				"PAYLOAD":           "{\"status\":\"UPDATE_MANUAL_APPROVAL_STATUS_APPROVED\",\"comments\":\"test comments1\",\"userId\":\"123\",\"userName\":\"testUserName\",\"respondedOn\":\"2009-11-10T23:00:00Z\", \"inputs\":[]}",
			},
			statusInFile:      "{\"message\":\"Successfully changed workflow manual approval status\",\"status\":\"APPROVED\"}",
			commentsInOutput:  "test comments1",
			inputValsInOutput: "{}",
			output: []string{
				"Approved by testUserName on 2009-11-10T23:00:00Z with comments:\ntest comments1\n",
			},
			err: "",
		},
		{
			name: "success REJECTED",
			reqCheckFunc: func(req map[string]interface{}) {
				require.Equal(t, "UPDATE_MANUAL_APPROVAL_STATUS_REJECTED", req["status"].(string))
				require.Equal(t, "test comments2", req["comments"].(string))
				require.Equal(t, "123", req["userId"].(string))
				require.Equal(t, "testUserName", req["userName"].(string))
				require.Equal(t, "2009-11-10T23:00:00Z", req["respondedOn"].(string))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
				}, nil
			},
			env: map[string]string{
				"URL":               "http://test.com",
				"API_TOKEN":         "test",
				"CLOUDBEES_STATUS":  "/tmp/test-status-out",
				"CLOUDBEES_OUTPUTS": "/tmp/test-outputs",
				"PAYLOAD":           "{\"status\":\"UPDATE_MANUAL_APPROVAL_STATUS_REJECTED\",\"comments\":\"test comments2\",\"userId\":\"123\",\"userName\":\"testUserName\",\"respondedOn\":\"2009-11-10T23:00:00Z\"}",
			},
			statusInFile:      "{\"message\":\"Successfully changed workflow manual approval status\",\"status\":\"REJECTED\"}",
			commentsInOutput:  "test comments2",
			inputValsInOutput: "{}",
			output: []string{
				"Rejected by testUserName on 2009-11-10T23:00:00Z with comments:\ntest comments2\n",
			},
			err: "",
		},
		{
			name: "failure UNSPECIFIED",
			reqCheckFunc: func(req map[string]interface{}) {
				require.Equal(t, "UPDATE_MANUAL_APPROVAL_STATUS_UNSPECIFIED", req["status"].(string))
				require.Equal(t, "test comments", req["comments"].(string))
				require.Equal(t, "123", req["userId"].(string))
				require.Equal(t, "testUserName", req["userName"].(string))
				require.Equal(t, "2009-11-10T23:00:00Z", req["respondedOn"].(string))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
				}, nil
			},
			env: map[string]string{
				"URL":              "http://test.com",
				"API_TOKEN":        "test",
				"CLOUDBEES_STATUS": "/tmp/test-status-out",
				"PAYLOAD":          "{\"status\":\"UPDATE_MANUAL_APPROVAL_STATUS_UNSPECIFIED\",\"comments\":\"test comments\",\"userId\":\"123\",\"userName\":\"testUserName\",\"respondedOn\":\"2009-11-10T23:00:00Z\",\"inputs\":null}",
			},
			statusInFile: "{\"message\":\"Unexpected approval status 'UPDATE_MANUAL_APPROVAL_STATUS_UNSPECIFIED'\",\"status\":\"FAILED\"}",
			output: []string{
				"ERROR: Unexpected approval status 'UPDATE_MANUAL_APPROVAL_STATUS_UNSPECIFIED'\n",
			},
			err: "Unexpected approval status 'UPDATE_MANUAL_APPROVAL_STATUS_UNSPECIFIED'",
		},
		{
			name: "failure",
			reqCheckFunc: func(req map[string]interface{}) {
				require.Equal(t, "UPDATE_MANUAL_APPROVAL_STATUS_APPROVED", req["status"].(string))
				require.Equal(t, "test comments", req["comments"].(string))
				require.Equal(t, "123", req["userId"].(string))
				require.Equal(t, "testUserName", req["userName"].(string))
				require.Equal(t, "2009-11-10T23:00:00Z", req["respondedOn"].(string))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 500,
					Status:     "500 Internal Server Error",
					Body:       io.NopCloser(bytes.NewBufferString(`wrong parameter`)),
				}, nil
			},
			env: map[string]string{
				"URL":              "http://test.com",
				"API_TOKEN":        "test",
				"CLOUDBEES_STATUS": "/tmp/test-status-out",
				"PAYLOAD":          "{\"status\":\"UPDATE_MANUAL_APPROVAL_STATUS_APPROVED\",\"comments\":\"test comments\",\"userId\":\"123\",\"userName\":\"testUserName\",\"respondedOn\":\"2009-11-10T23:00:00Z\"}",
			},
			statusInFile: "{\"message\":\"Failed to change workflow manual approval status: 'failed to send event: \\nPOST http://test.com/v1/workflows/approval/status\\nHTTP/500 500 Internal Server Error\\n'\",\"status\":\"FAILED\"}",
			output: []string{
				"ERROR: API call failed with error: 'failed to send event: \nPOST http://test.com/v1/workflows/approval/status\nHTTP/500 500 Internal Server Error\n'\n",
				"ERROR: API response: 'wrong parameter'\n",
			},
			err: "failed to send event: \nPOST http://test.com/v1/workflows/approval/status\nHTTP/500 500 Internal Server Error\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare
			for k, v := range tt.env {
				os.Setenv(k, v)
				defer func(k string) {
					os.Unsetenv(k)
				}(k)
			}
			outputs_dir, exists := tt.env["CLOUDBEES_OUTPUTS"]
			if exists {
				os.Mkdir(outputs_dir, 0755)
				defer func(dir string) {
					os.RemoveAll(dir)
				}(outputs_dir)
			}

			var testOutput []string

			// Run
			c := Config{
				Client: &MockHttpClient{
					MockDo: func(req *http.Request) (*http.Response, error) {
						require.NotNil(t, req)
						require.Equal(t, "POST", req.Method)
						require.Equal(t, "http://test.com/v1/workflows/approval/status", req.URL.String())
						require.Equal(t, "application/json", req.Header.Get("Content-Type"))
						require.Equal(t, "application/json", req.Header.Get("Accept"))
						require.Contains(t, req.Header.Get("Authorization"), "Bearer ")

						reqBody := make(map[string]interface{})
						bodyReader, err := req.GetBody()
						require.NoError(t, err)
						body, err := io.ReadAll(bodyReader)
						require.NoError(t, err)
						err = json.Unmarshal(body, &reqBody)
						require.NoError(t, err)

						// Check parsed request body
						tt.reqCheckFunc(reqBody)

						// Generate response
						return tt.respGenFunc()
					},
				},
				Output: &MockStdOut{
					MockPrintf: func(format string, a ...any) {
						testOutput = append(testOutput, fmt.Sprintf(format, a...))
						fmt.Printf(format, a...)
					},
					MockPrintln: func(a ...any) {
						testOutput = append(testOutput, fmt.Sprintln(a...))
						fmt.Println(a...)
					},
				},
			}
			err := c.callback()

			// Verify
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Equal(t, tt.err, err.Error())
			}

			if tt.inputValsInOutput != "" {
				out, ferr := os.ReadFile(tt.env["CLOUDBEES_OUTPUTS"] + "/approvalInputValues")
				require.NoError(t, ferr)
				require.Equal(t, tt.inputValsInOutput, string(out))
			}

			if tt.commentsInOutput != "" {
				out, ferr := os.ReadFile(tt.env["CLOUDBEES_OUTPUTS"] + "/comments")
				require.NoError(t, ferr)
				require.Equal(t, tt.commentsInOutput, string(out))
			}

			out, ferr := os.ReadFile(tt.env["CLOUDBEES_STATUS"])
			require.NoError(t, ferr)
			require.Equal(t, tt.statusInFile, string(out))

			require.True(t, slices.Equal(tt.output, testOutput))
		})
	}
}

func Test_cancel(t *testing.T) {
	tests := []struct {
		name         string
		reqCheckFunc func(req map[string]interface{})
		respGenFunc  func() (*http.Response, error)
		env          map[string]string
		client       *MockHttpClient
		output       []string
		err          string
	}{
		{
			name: "success CANCELLED",
			reqCheckFunc: func(req map[string]interface{}) {
				require.NotNil(t, req["status"])
				require.Equal(t, "UPDATE_MANUAL_APPROVAL_STATUS_ABORTED", req["status"].(string))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
				}, nil
			},
			env: map[string]string{
				"URL":                 "http://test.com",
				"API_TOKEN":           "test",
				"CANCELLATION_REASON": "CANCELLED",
			},
			output: []string{
				"Workflow aborted by user\n",
				"Cancelling the manual approval request\n",
			},
			err: "",
		},
		{
			name: "success TIMED_OUT",
			reqCheckFunc: func(req map[string]interface{}) {
				require.NotNil(t, req["status"])
				require.Equal(t, "UPDATE_MANUAL_APPROVAL_STATUS_TIMED_OUT", req["status"].(string))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 200,
					Status:     "200 OK",
					Body:       io.NopCloser(bytes.NewBufferString(`{}`)),
				}, nil
			},
			env: map[string]string{
				"URL":                 "http://test.com",
				"API_TOKEN":           "test",
				"CANCELLATION_REASON": "TIMED_OUT",
			},
			output: []string{
				"Workflow timed out\n",
				"Workflow approval response was not received within allotted time.\n",
			},
			err: "",
		},
		{
			name: "failure",
			reqCheckFunc: func(req map[string]interface{}) {
				require.NotNil(t, req["status"])
				require.Equal(t, "UPDATE_MANUAL_APPROVAL_STATUS_TIMED_OUT", req["status"].(string))
			},
			respGenFunc: func() (*http.Response, error) {
				return &http.Response{
					StatusCode: 500,
					Status:     "500 Internal Server Error",
					Body:       io.NopCloser(bytes.NewBufferString(`wrong parameter`)),
				}, nil
			},
			env: map[string]string{
				"URL":                 "http://test.com",
				"API_TOKEN":           "test",
				"CANCELLATION_REASON": "TIMED_OUT",
			},
			output: []string{
				"Workflow timed out\n",
				"Workflow approval response was not received within allotted time.\n",
				"ERROR: API call failed with error: 'failed to send event: \nPOST http://test.com/v1/workflows/approval/status\nHTTP/500 500 Internal Server Error\n'\n",
				"ERROR: API response: 'wrong parameter'\n",
			},
			err: "failed to send event: \nPOST http://test.com/v1/workflows/approval/status\nHTTP/500 500 Internal Server Error\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Prepare
			for k, v := range tt.env {
				os.Setenv(k, v)
				defer func(k string) {
					os.Unsetenv(k)
				}(k)
			}

			var testOutput []string

			// Run
			c := Config{
				Client: &MockHttpClient{
					MockDo: func(req *http.Request) (*http.Response, error) {
						require.NotNil(t, req)
						require.Equal(t, "POST", req.Method)
						require.Equal(t, "http://test.com/v1/workflows/approval/status", req.URL.String())
						require.Equal(t, "application/json", req.Header.Get("Content-Type"))
						require.Equal(t, "application/json", req.Header.Get("Accept"))
						require.Contains(t, req.Header.Get("Authorization"), "Bearer ")

						reqBody := make(map[string]interface{})
						bodyReader, err := req.GetBody()
						require.NoError(t, err)
						body, err := io.ReadAll(bodyReader)
						require.NoError(t, err)
						err = json.Unmarshal(body, &reqBody)
						require.NoError(t, err)

						// Check parsed request body
						tt.reqCheckFunc(reqBody)

						// Generate response
						return tt.respGenFunc()
					},
				},
				Output: &MockStdOut{
					MockPrintf: func(format string, a ...any) {
						testOutput = append(testOutput, fmt.Sprintf(format, a...))
						fmt.Printf(format, a...)
					},
					MockPrintln: func(a ...any) {
						testOutput = append(testOutput, fmt.Sprintln(a...))
						fmt.Println(a...)
					},
				},
			}
			err := c.cancel()

			// Verify
			if tt.err == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Equal(t, tt.err, err.Error())
			}

			require.True(t, slices.Equal(tt.output, testOutput))
		})
	}
}

func Test_markdown(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name:   "Markdown",
			input:  instructionsInput,
			output: instructionsOutput,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run
			result := markdown(tt.input)

			// Verify
			require.Equal(t, tt.output, result)
		})
	}
}
