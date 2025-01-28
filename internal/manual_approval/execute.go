package manual_approval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark"
)

var debug bool

type RealHttpClient struct{}

func (c *RealHttpClient) Do(req *http.Request) (*http.Response, error) {
	http.DefaultClient.Timeout = 150 * time.Second
	return http.DefaultClient.Do(req)
}

type RealStdOut struct{}

func (c *RealStdOut) Printf(format string, a ...any) {
	fmt.Printf(format, a...)
}

func (c *RealStdOut) Println(a ...any) {
	fmt.Println(a...)
}

func init() {
	debug = os.Getenv("DEBUG") == "true"
}

func (k *Config) Run(ctx context.Context) error {
	k.Context = ctx

	// Use default std out if it is not already provided in the configuration
	if k.Output == nil {
		k.Output = &RealStdOut{}
	}

	switch k.Handler {
	case "init":
		return k.init()
	case "callback":
		return k.callback()
	case "cancel":
		return k.cancel()
	default:
		return fmt.Errorf("unsupported handler type: %s", k.Handler)
	}
}

func (k *Config) defaultConfig() (string, string, error) {
	debugf("Read default configuration from the environment variables\n")

	apiUrl := os.Getenv("URL")
	if apiUrl == "" {
		return "", "", fmt.Errorf("URL environment variable missing")
	}

	apiToken := os.Getenv("API_TOKEN")
	if apiToken == "" {
		return "", "nil", fmt.Errorf("API_TOKEN environment variable missing")
	}

	return apiUrl, apiToken, nil
}

func (k *Config) init() error {
	debugf("Inside init handler\n")

	// approvers are optional
	approvers := os.Getenv("APPROVERS")

	// instructions are optional
	instructions := os.Getenv("INSTRUCTIONS")

	// by default disallowLaunchedByUser is false
	disallowLaunchedByUserStr := os.Getenv("DISALLOW_LAUNCHED_BY_USER")
	if disallowLaunchedByUserStr == "" {
		disallowLaunchedByUserStr = "false"
	}
	disallowLaunchedByUser, err := strconv.ParseBool(disallowLaunchedByUserStr)
	if err != nil {
		return err
	}

	// by default notifyAllEligibleUsers is false
	notifyStr := os.Getenv("NOTIFY_ALL_ELIGIBLE_USERS")
	if notifyStr == "" {
		notifyStr = "false"
	}
	notify, err := strconv.ParseBool(notifyStr)
	if err != nil {
		return err
	}

	// get approvalInputs if configured for the manual approval job
	inputs := os.Getenv("INPUTS")

	// Construct request body
	body := map[string]interface{}{
		"disallowLaunchedByUser": disallowLaunchedByUser,
		"notifyEligibleUsers":    notify,
	}

	if approvers != "" {
		body["approvers"] = strings.Split(approvers, ",")
	}

	if instructions != "" {
		body["instructions"] = instructions
	}

	if inputs != "" {
		body["approvalInputs"] = inputs
	}

	resp, err := k.post("/v1/workflows/approval", body)
	if err != nil {
		k.Output.Printf("ERROR: API call failed with error: '%s'\n", err)
		k.Output.Printf("ERROR: API response: '%s'\n", resp)
		ferr := writeStatus("FAILED", fmt.Sprintf("Failed to initialize workflow manual approval request: '%s'", err))
		if ferr != nil {
			return ferr
		}
		return err
	}
	debugf("Response: '%s'\n", resp)

	//get the names of potential approvers from the response
	parsedResp := CreateManualApprovalResponse{}
	err = json.Unmarshal([]byte(resp), &parsedResp)
	if err != nil {
		return err
	}

	users := make([]string, len(parsedResp.Approvers))
	for i, approver := range parsedResp.Approvers {
		users[i] = approver.UserName
	}

	k.Output.Printf("Waiting for approval from one of the following: %s\n", strings.Join(users, ","))
	if instructions != "" {
		k.Output.Printf("Instructions:\n%s\n", markdown(instructions))
	}

	return writeStatus("PENDING_APPROVAL", "Waiting for approval from approvers")
}

func (k *Config) callback() error {
	debugf("Inside callback handler\n")

	payload := os.Getenv("PAYLOAD")
	if payload == "" {
		return fmt.Errorf("PAYLOAD environment variable missing")
	}

	debugf("Incoming payload: '%s'\n", payload)

	parsedPayload := map[string]interface{}{}
	err := json.Unmarshal([]byte(payload), &parsedPayload)
	if err != nil {
		return err
	}

	approvalStatus := parsedPayload["status"].(string)
	debugf("Approval status: '%s'\n", approvalStatus)

	comments := parsedPayload["comments"].(string)
	debugf("Comments: '%s'\n", comments)

	respondedOn := parsedPayload["respondedOn"].(string)
	debugf("Responded on: '%s'\n", respondedOn)

	approverUserName := parsedPayload["userName"].(string)
	debugf("Approver user name: '%s'\n", approverUserName)

	// POST request expects input param values to be strings, so converting values to string
	// Also, creating a map with input values in original type to be made available in outputs
	modifiedInputsParamForPost, outputsMap, err4 := formatInputsForPost(parsedPayload)
	if err4 != nil {
		return err4
	}

	resp, err := k.post("/v1/workflows/approval/status", parsedPayload)
	if err != nil {
		k.Output.Printf("ERROR: API call failed with error: '%s'\n", err)
		k.Output.Printf("ERROR: API response: '%s'\n", resp)
		ferr := writeStatus("FAILED", fmt.Sprintf("Failed to change workflow manual approval status: '%s'", err))
		if ferr != nil {
			return ferr
		}
		return err
	}
	debugf("Response: '%s'\n", resp)

	jobStatus, err2 := k.processApprovalStatus(approvalStatus, approverUserName, respondedOn, comments)
	if err2 != nil {
		return err2
	}

	// Add suffix for default vals and write to log
	k.formatInputsValsAndWriteToLog(modifiedInputsParamForPost)

	//
	err3 := k.writeToOutputs(outputsMap, comments)
	if err3 != nil {
		return err3
	}

	return writeStatus(jobStatus, "Successfully changed workflow manual approval status")
}

/*
* POST request expects input param values to be strings, so converting values
* to string Also, creating a map with input values in original type to be made
* available in outputs
 */
func formatInputsForPost(parsedPayload map[string]interface{}) ([]interface{}, map[string]interface{}, error) {
	var modifiedInputsParamForPost []interface{}
	outputsMap := make(map[string]interface{})

	if parsedPayload["inputs"] != nil && len(parsedPayload["inputs"].([]interface{})) > 0 {

		modifiedInputsParamForPost = parsedPayload["inputs"].([]interface{})

		for _, input := range modifiedInputsParamForPost {
			ip := input.(map[string]interface{})
			// To print input param values in original type to outputs
			outputsMap[ip["name"].(string)] = ip["value"]
			// Converting param value to string type for POST request
			inputVal := interfaceToString(ip["value"])
			ip["value"] = inputVal
		}
		parsedPayload["inputs"] = modifiedInputsParamForPost

		updtVal, err2 := json.Marshal(parsedPayload)
		if err2 != nil {
			fmt.Println("Invalid payload after converting input values to string:", err2)
			return nil, nil, err2
		}
		debugf("Inputs for post request: '%s'\n", updtVal)
	} else {
		debugf("**No Input Parameters Defined**\n")
	}

	return modifiedInputsParamForPost, outputsMap, nil
}

func (k *Config) writeToOutputs(outputsMap map[string]interface{}, comments string) error {

	if outputsMap != nil {
		outputBytes, err := json.Marshal(outputsMap)
		if err != nil {
			return err
		}
		err = writeAsOutput("approvalInputValues", outputBytes)
		if err != nil {
			return err
		}
		debugf("Approval Input Values in outputs: '%s'\n", string(outputBytes))
	}

	err := writeAsOutput("comments", []byte(comments))
	if err != nil {
		return err
	}
	return nil
}

// Add suffix if input param value is default value before writing it to callback handler logs
func (k *Config) formatInputsValsAndWriteToLog(modifiedInputsParamForPost []interface{}) {
	if len(modifiedInputsParamForPost) > 0 {
		k.Output.Printf("\nInput Parameters:\n")
		k.Output.Printf("------------------\n")
		suffix := " (default)"
		for _, input := range modifiedInputsParamForPost {
			ip := input.(map[string]interface{})
			inputVal := ip["value"].(string)
			inputVal = strings.Replace(inputVal, "\n", "<br/>", -1) // replace /n with <br> for html rendering
			if ip["is_default"] == true {
				inputVal += suffix
			}

			k.Output.Printf(" %s: %s \n",
				ip["name"], inputVal)
		}
	}
}

func (k *Config) processApprovalStatus(approvalStatus string, approverUserName string, respondedOn string, comments string) (string, error) {
	var jobStatus string
	switch approvalStatus {
	case "UPDATE_MANUAL_APPROVAL_STATUS_APPROVED":
		jobStatus = "APPROVED"
		k.Output.Printf("Approved by %s on %s with comments:\n%s\n", approverUserName, respondedOn, comments)
	case "UPDATE_MANUAL_APPROVAL_STATUS_REJECTED":
		jobStatus = "REJECTED"
		k.Output.Printf("Rejected by %s on %s with comments:\n%s\n", approverUserName, respondedOn, comments)
	default:
		k.Output.Printf("ERROR: Unexpected approval status '%s'\n", approvalStatus)
		ferr := writeStatus("FAILED", fmt.Sprintf("Unexpected approval status '%s'", approvalStatus))
		if ferr != nil {
			return "", ferr
		}
		return "", fmt.Errorf("Unexpected approval status '%s'", approvalStatus)
	}
	return jobStatus, nil
}

func interfaceToString(i interface{}) string {
	switch v := i.(type) {
	case string:
		return v
	case int:
		return strconv.FormatInt(int64(v), 64)
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		return "unsupported type"
	}
}

func (k *Config) cancel() error {
	debugf("Inside cancel handler\n")

	cancellationReason := os.Getenv("CANCELLATION_REASON")
	if cancellationReason == "" {
		return fmt.Errorf("CANCELLATION_REASON environment variable missing")
	}

	// Construct request body
	body := map[string]interface{}{}
	if cancellationReason == "CANCELLED" {
		k.Output.Println("Workflow aborted by user")
		k.Output.Println("Cancelling the manual approval request")
		body["status"] = "UPDATE_MANUAL_APPROVAL_STATUS_ABORTED"
	} else {
		k.Output.Println("Workflow timed out")
		k.Output.Println("Workflow approval response was not received within allotted time.")
		body["status"] = "UPDATE_MANUAL_APPROVAL_STATUS_TIMED_OUT"
	}

	resp, err := k.post("/v1/workflows/approval/status", body)
	if err != nil {
		k.Output.Printf("ERROR: API call failed with error: '%s'\n", err)
		k.Output.Printf("ERROR: API response: '%s'\n", resp)
		return err
	}
	debugf("Response: '%s'\n", resp)

	return nil
}

func (k *Config) post(apiPath string, requestBody map[string]interface{}) (string, error) {
	debugf("Post http request to the platform API endpoint: '%s'\n", apiPath)

	// Read default configuration from the environment variables
	apiUrl, apiToken, err := k.defaultConfig()
	if err != nil {
		return "", err
	}

	// Construct the request URL for the API call
	requestURL, err := url.JoinPath(apiUrl, apiPath)
	if err != nil {
		return "", err
	}

	// Prepare JSON request body for REST API call
	body, err := json.Marshal(&requestBody)
	if err != nil {
		return "", err
	}
	debugf("Payload: '%s'\n", string(body))

	// Use default http client if it is not already provided in the configuration
	if k.Client == nil {
		k.Client = &RealHttpClient{}
	}

	apiReq, err := http.NewRequest(
		"POST",
		requestURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}

	apiReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))
	apiReq.Header.Set("Content-Type", "application/json")
	apiReq.Header.Set("Accept", "application/json")

	resp, err := k.Client.Do(apiReq)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	response := string(responseBody)

	if resp.StatusCode != 200 {
		return response, fmt.Errorf("failed to send event: \nPOST %s\nHTTP/%d %s\n", requestURL, resp.StatusCode, resp.Status)
	}

	return response, nil
}

func debugf(format string, a ...any) {
	if debug {
		t := time.Now()
		fmt.Printf("%s - DEBUG: "+format, append([]any{t.Format(time.RFC3339)}, a...)...)
	}
}

func writeAsOutput(name string, value []byte) error {
	outputsDir := os.Getenv("CLOUDBEES_OUTPUTS")
	if outputsDir == "" {
		return fmt.Errorf("CLOUDBEES_OUTPUTS environment variable missing")
	}

	outputFile := filepath.Join(outputsDir, name)
	err := os.WriteFile(outputFile, value, 0755)
	if err != nil {
		return fmt.Errorf("failed to write to %s: %w", outputFile, err)
	}
	return nil
}

func writeStatus(status string, message string) error {
	statusFile := os.Getenv("CLOUDBEES_STATUS")
	if statusFile == "" {
		return fmt.Errorf("CLOUDBEES_STATUS environment variable missing")
	}
	output := map[string]interface{}{
		"status":  status,
		"message": message,
	}

	outputBytes, err := json.Marshal(&output)
	if err != nil {
		return err
	}
	err = os.WriteFile(statusFile, outputBytes, 0666)
	if err != nil {
		return fmt.Errorf("failed to write to %s: %w", statusFile, err)
	}
	return nil
}

// Add markdown format support to instructions
func markdown(value string) string {
	var buf bytes.Buffer
	md := goldmark.New()
	if err := md.Convert([]byte(value), &buf); err != nil {
		fmt.Printf("Failed to convert markdown to html: %v\n", err)
	} else {
		value = buf.String()
	}
	return value
}
