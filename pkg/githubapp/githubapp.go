// Package githubapp provides helper functions for githubapp actions
// primarily when you run a githubapp in API Gateway / Lambda
package githubapp

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/aws/aws-lambda-go/events"
	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/github"
)

// ValidateRequest verifies the request is a GitHub webhook event and verifies the request with a secret
func ValidateRequest(r events.APIGatewayProxyRequest, webhookSecret string) error {
	if r.HTTPMethod != "POST" {
		return fmt.Errorf("HTTPMethod is not POST, this server only accepts post requests on this endpoint")
	}

	eventType := r.Headers["X-GitHub-Event"]

	if eventType == "" {
		return fmt.Errorf("X-GitHub-Event header is not present, this has to be present to be a valid github webhook event")
	}

	signature := r.Headers["X-Hub-Signature"]
	if signature == "" {
		return fmt.Errorf("X-Hub-Signature header is not present, this has to be present to sign the request from GitHub")
	}

	payload := []byte(r.Body)
	mac := hmac.New(sha1.New, []byte(webhookSecret))
	_, _ = mac.Write(payload)
	expectedMAC := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature[5:]), []byte(expectedMAC)) {
		return fmt.Errorf("HMAC verification failed, this request might not be coming from GitHub")
	}
	return nil
}

// NewGithubClient returns an authenticated GithubClient from the go-github package
// This is to be used for github app authorization
func NewGithubClient(integrationID, installationID int, privateKey []byte) (*github.Client, error) {
	itr, err := ghinstallation.New(http.DefaultTransport, integrationID, installationID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("Error creating github client, error: %s", err)
	}

	return github.NewClient(&http.Client{Transport: itr}), nil
}
