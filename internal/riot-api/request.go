package riotapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func (c *Client) makeRequest(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("X-Riot-Token", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	if resp.StatusCode == 429 {
		retryAfter := resp.Header.Get("Retry-After")
		if retryAfter != "" {
			seconds, _ := strconv.Atoi(retryAfter)
			time.Sleep(time.Duration(seconds) * time.Second)
		} else {
			time.Sleep(1 * time.Second)
		}
		return c.makeRequest(url)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("API request failed with status code: %d", resp.StatusCode)
	}

	return resp, nil
}
