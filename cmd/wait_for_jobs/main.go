package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/url"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/codingdiaz/circleci-feedback/internal/stepfunc"
	"github.com/codingdiaz/circleci-feedback/pkg/circleci"
	"github.com/codingdiaz/circleci-feedback/pkg/githubapp"
	"github.com/google/go-github/github"
)

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context, in stepfunc.Data) (stepfunc.Data, error) {

	// get configuration for lambda function to run
	c, err := stepfunc.GetConfiguration()
	if err != nil {
		log.Printf("Error getting lambda function configuration, error: %s", err)
		return in, fmt.Errorf("Error getting lambda function configuration, error: %s", err)
	}

	// handle backoff retry, update step function input to set as future outputs
	in.WaitForJobsWaitTime = int(math.Pow(2, float64(in.WaitForJobsRetryCount)))
	in.WaitForJobsRetryCount = in.WaitForJobsRetryCount + 1

	// create v2 circleci client
	client := circleci.Client{Token: c.CircleToken}

	// if we don't have the workflow ids, get them
	if len(in.WorkflowIDs) == 0 {
		// get the full pipeline
		pipeline, err := client.GetPipeline(in.PipelineID)
		if err != nil {
			return in, fmt.Errorf("Error getting pipeline with id %s error: %s", pipeline.ID, err)
		}

		// add all the workflow ids associated with the pipeline to our stepfunc struct
		workflows := []string{}
		for _, workflow := range pipeline.Workflows {
			workflows = append(workflows, workflow.ID)
		}
		in.WorkflowIDs = workflows
	}

	// if we do have the workflow ids, start checking job information / status
	for _, workflow := range in.WorkflowIDs {
		jobs, err := client.GetWorkflowJobs(workflow)
		if err != nil {
			log.Printf("Error getting jobs for workflow with id %v, error: %s", workflow, err)
			return in, fmt.Errorf("Error getting jobs for workflow with id %v, error: %s", workflow, err)
		}

		if in.WorkflowJobs == nil {
			in.WorkflowJobs = make(map[string][]circleci.Job)
		}

		in.WorkflowJobs[workflow] = jobs

		for _, job := range jobs {
			if job.Status != "success" && job.Status != "failed" {
				in.AllJobsDone = false
				return in, nil
			}
		}
	}

	// at this point all jobs should be done, but we are going to send out failures
	for _, workflow := range in.WorkflowIDs {
		jobs, err := client.GetWorkflowJobs(workflow)
		if err != nil {
			log.Printf("Error getting jobs for workflow with id %v, error: %s", workflow, err)
			return in, fmt.Errorf("Error getting jobs for workflow with id %v, error: %s", workflow, err)
		}
		for _, job := range jobs {
			if job.Status == "failed" {
				log.Println("Sending failure logs to github as a comment on a pull request")
				err := sendBuildFailureToGithub(in, job.JobNumber, c)
				if err != nil {
					return in, fmt.Errorf("Error sending build failure to github, %s", err)
				}
			}
		}
	}

	in.AllJobsDone = true

	return in, nil
}

func sendBuildFailureToGithub(in stepfunc.Data, number int, cfg stepfunc.Config) error {

	c := circleci.Client{
		Token:   cfg.CircleToken,
		BaseURL: &url.URL{Host: "circleci.com", Scheme: "https", Path: "/api/v1.1/"},
	}

	build, err := c.GetBuild("gh", in.Owner, in.RepoName, number)
	if err != nil {
		log.Printf("Error getting build %v %s", number, err)
		return fmt.Errorf("Error getting build %v %s", number, err)
	}

	for _, step := range build.Steps {
		for _, action := range step.Actions {
			if action.Status == "failed" {
				buildOutput, err := circleci.GetBuildOutput(action.OutputURL)
				if err != nil {
					log.Printf("Error getting build output for failed build, %s", err)
					return fmt.Errorf("Error getting build output for failed build, %s", err)
				}
				githubClient, err := githubapp.NewGithubClient(cfg.InstallationID, in.InstallationID, cfg.GithubAppPrivateKey)
				if err != nil {
					log.Printf("Unable to create authenticated github client, error: %s\n", err)
					return fmt.Errorf("Unable to create authenticated github client, error: %s", err)
				}
				message := "Build Failed :cry: \n```\n"
				for _, output := range buildOutput {
					message = message + fmt.Sprintf("%s", output.Message)
				}
				message = message + "\n```"
				comment := github.IssueComment{
					Body: &message,
				}
				_, _, err = githubClient.Issues.CreateComment(context.Background(), in.Owner, in.RepoName, in.PullRequestNumber, &comment)
				if err != nil {
					log.Printf("Unable to post a comment on the PR telling the user they don't have a circleci file, error: %s", err)
					return fmt.Errorf("Unable to post a comment on the PR telling the user they don't have a circleci file, error: %s", err)
				}

			}
		}
	}

	return nil
}
