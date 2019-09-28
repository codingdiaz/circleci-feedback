// Package stepfunc provides code needed to be shared across the stepfunction lambdas
package stepfunc

import (
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

// Data is the common input/ouput for all lambda functions in the step function
type Data struct {
	RepoName              string   `json:"repo_name"`
	Owner                 string   `json:"owner"`
	PullRequestNumber     int      `json:"pull_request_number"`
	InstallationID        int      `json:"installation_id"`
	CommitSHA             string   `json:"commit_sha"`
	PipelineID            string   `json:"pipeline_id"`
	WorkflowIDs           []string `json:"workflow_ids"`
	AllJobsDone           bool     `json:"all_jobs_done"`
	WaitForJobsRetryCount int      `json:"wait_for_jobs_retry_count"`
	WaitForJobsWaitTime   int      `json:"wait_for_jobs_wait_time"`
}

// Config holds all the configuration for the lambda function
type Config struct {
	GitHubWebhookSecret string
	GithubAppPrivateKey []byte
	InstallationID      int
}

// GetConfiguration gets all the secret values that the lambda function needs to run and will error if it can't fetch any
func GetConfiguration() (Config, error) {
	config := Config{}
	sess, err := session.NewSession()
	if err != nil {
		return config, err
	}

	ssmsvc := ssm.New(sess, aws.NewConfig())
	keyname := "/circleci-feedback/GithubWebhookSecret"
	withDecryption := true
	param, err := ssmsvc.GetParameter(&ssm.GetParameterInput{
		Name:           &keyname,
		WithDecryption: &withDecryption,
	})
	if err != nil {
		return config, fmt.Errorf("Error getting GitubWebhookSecret, error: %s", err)
	}

	config.GitHubWebhookSecret = *param.Parameter.Value

	keyname = "/circleci-feedback/GithubAppPrivateKey"
	param, err = ssmsvc.GetParameter(&ssm.GetParameterInput{
		Name:           &keyname,
		WithDecryption: &withDecryption,
	})
	if err != nil {
		return config, fmt.Errorf("Error getting GithubAppPrivateKey, error: %s", err)
	}

	config.GithubAppPrivateKey = []byte(*param.Parameter.Value)

	keyname = "/circleci-feedback/InstallationID"
	param, err = ssmsvc.GetParameter(&ssm.GetParameterInput{
		Name:           &keyname,
		WithDecryption: &withDecryption,
	})
	if err != nil {
		return config, fmt.Errorf("Error getting InstallationID, error: %s", err)
	}
	InstallationID, err := strconv.Atoi(*param.Parameter.Value)
	if err != nil {
		return config, fmt.Errorf("Error converting InstallationID to int from string, error: %s", err)
	}

	config.InstallationID = InstallationID

	return config, nil

}
