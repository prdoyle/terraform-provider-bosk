package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

type BoskClient struct {
	httpClient *http.Client
}

func NewBoskClient(httpClient *http.Client) *BoskClient {
	return &BoskClient{httpClient: httpClient}
}

// Portions taken from: https://github.com/hashicorp/terraform-provider-http/blob/main/internal/provider/data_source_http.go
func (client *BoskClient) GetJSONAsString(url string, diag *diag.Diagnostics) string {
	httpResp, err := client.httpClient.Get(url)
	if err != nil {
		diag.AddError("Client Error", fmt.Sprintf("Unable to read node: %s", err))
		return "ERROR"
	}

	defer httpResp.Body.Close()

	bytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		diag.AddError(
			"Error reading response body",
			fmt.Sprintf("Error reading response body: %s", err),
		)
		return "ERROR"
	}
	if !utf8.Valid(bytes) {
		diag.AddWarning(
			"Response body is not recognized as UTF-8",
			"Terraform may not properly handle the response_body if the contents are binary.",
		)
	}

	normalized, err := normalizeJSON(bytes)
	if err != nil {
		diag.AddWarning(
			"Error normalizing JSON response",
			fmt.Sprintf("Error reading response body: %s", err),
		)
		return string(bytes)
	}

	return string(normalized)
}

func normalizeJSON(input []byte) ([]byte, error) {
	var parsed interface{}
	err := json.Unmarshal(input, &parsed)
	if err != nil {
		return input, err
	}
	result, err := json.Marshal(parsed)
	if err != nil {
		return input, err
	}
	return result, nil
}
