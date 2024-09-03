package riotapi

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

// makeRequest performs an HTTP GET request to the specified URL using the client's API key.
// It handles rate limiting through the client's rate limiter and manages API-specific errors.
//
// The function will:
//  1. Wait for the rate limiter before making the request.
//  2. Create and send an HTTP GET request with the Riot API key in the header.
//  3. Handle non-200 status codes, creating a RiotAPIError for detailed error information.
//  4. Specifically manage rate limit errors (HTTP 429) by respecting the Retry-After header
//     or using a default wait time, then retrying the request.
func (c *Client) makeRequest(url string) (*http.Response, error) {
	c.rateLimiter.Wait()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("X-Riot-Token", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		apiError := &RiotAPIError{
			StatusCode: resp.StatusCode,
			Message:    string(body),
			Headers:    resp.Header,
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := resp.Header.Get("Retry-After")
			if retryAfter != "" {
				seconds, err := strconv.Atoi(retryAfter)
				if err != nil {
					log.Printf("Error parsing Retry-After header: %v. Using default wait time.", err)
					time.Sleep(1 * time.Second)
				} else {
					time.Sleep(time.Duration(seconds) * time.Second)
				}
			} else {
				time.Sleep(1 * time.Second)
			}
			return c.makeRequest(url) // Retry the request
		}

		return nil, apiError
	}

	return resp, nil
}
