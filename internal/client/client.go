package client

// Totally not stolen from https://github.com/hashicorp-demoapp/hashicups-client-go/blob/main/client.go

import (
	//"encoding/json"
	//"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
	//"strings"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Username   string
	Password   string
}

func NewClient(baseURL, username, password string) *Client {
	return &Client {
		BaseURL: baseURL,
		Username: username,
		Password: password,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *Client) doRequest(req *http.Request) ([]byte, error) {
	req.SetBasicAuth(c.Username, c.Password)

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode / 100 != 2 {
		return nil, fmt.Errorf("status: %d, body: %s", res.StatusCode, body)
	}

	return body, err
}


