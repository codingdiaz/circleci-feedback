package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sfn"
	"github.com/codingdiaz/circleci-feedback/internal/stepfunc"
	"github.com/codingdiaz/circleci-feedback/pkg/githubapp"
	"github.com/google/go-github/github"
	githubEvents "gopkg.in/go-playground/webhooks.v5/github"
)

func main() {
	lambda.Start(handler)
}

func handler(request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	// get configuration for lambda function to run
	c, err := stepfunc.GetConfiguration()
	if err != nil {
		log.Printf("Error getting lambda function configuration, error: %s", err)
		return events.APIGatewayProxyResponse{Body: request.Body, StatusCode: 500}, nil
	}

	// validate the request and return unauthorized if the validate function returns an error
	err = githubapp.ValidateRequest(request, c.GitHubWebhookSecret)
	if err != nil {
		log.Printf("The request was not valid, error: %s", err)
		return events.APIGatewayProxyResponse{StatusCode: 403}, nil
	}

	// only trigger on certain events that come in a header from github
	// if the event type is outside of what we want to process, return
	eventType := request.Headers["X-GitHub-Event"]
	if eventType != "pull_request" {
		log.Printf("Request eventType is not supported, %s\n", eventType)
		return events.APIGatewayProxyResponse{Body: request.Body, StatusCode: 200}, nil
	}

	// unmarshal request body into go struct
	event := githubEvents.PullRequestPayload{}
	err = json.Unmarshal([]byte(request.Body), &event)
	if err != nil {
		log.Printf("Unable to unmarshal request body into go struct, error: %s\n", err)
		return events.APIGatewayProxyResponse{Body: request.Body, StatusCode: 200}, nil
	}

	// only process syncronize events (new commits on a pr) and the opened PR event
	// https://developer.github.com/v3/activity/events/types/#pullrequestevent
	if event.Action != "synchronize" && event.Action != "opened" {
		return events.APIGatewayProxyResponse{StatusCode: 200}, nil
	}

	// Create an autorized GitHub client
	githubClient, err := githubapp.NewGithubClient(c.InstallationID, int(event.Installation.ID), c.GithubAppPrivateKey)
	if err != nil {
		log.Printf("Unable to create authenticated github client, error: %s\n", err)
		return events.APIGatewayProxyResponse{Body: request.Body, StatusCode: 500}, nil
	}

	// Check to see if the repo has a file at `.circleci/config.yml`
	_, _, resp, err := githubClient.Repositories.GetContents(context.Background(), event.Repository.Owner.Login, event.Repository.Name, ".circleci/config.yml", &github.RepositoryContentGetOptions{
		Ref: event.PullRequest.Head.Ref,
	})

	// if the repo doesn't have a `.circleci/config.yml file`, simply comment on the PR and return
	if err != nil {
		if resp.StatusCode == 404 {
			comment := github.IssueComment{
				Body: github.String("You don't seem to have a .circleci/config.yml file in your repo\n Register with CircleCI to use this GITHUB APP."),
			}
			_, _, err = githubClient.Issues.CreateComment(context.Background(), event.Repository.Owner.Login, event.Repository.Name, int(event.Number), &comment)
			if err != nil {
				log.Printf("Unable to post a comment on the PR telling the user they don't have a circleci file, error: %s", err)
				return events.APIGatewayProxyResponse{Body: request.Body, StatusCode: 500}, nil
			}
		} else {
			log.Printf("Got a bad status code (%v) trying to see if the repo has a circleci/config.yml file, error: %s", resp.StatusCode, err)
			return events.APIGatewayProxyResponse{Body: request.Body, StatusCode: 500}, nil
		}
	}

	// If the repo has a `.circleci/config.yml` file, start the step function
	sess, err := session.NewSession()
	if err != nil {
		panic(err)
	}
	svc := sfn.New(sess, aws.NewConfig())

	input := stepfunc.Data{
		InstallationID:    int(event.Installation.ID),
		CommitSHA:         event.PullRequest.Head.Sha,
		RepoName:          event.Repository.Name,
		Owner:             event.Repository.Owner.Login,
		PullRequestNumber: int(event.Number),
	}

	data, _ := json.Marshal(input)

	sfnExecutionInput := &sfn.StartExecutionInput{
		StateMachineArn: aws.String(os.Getenv("STEP_FUNCTION_ARN")),
		Input:           aws.String(string(data)),
	}
	_, err = svc.StartExecution(sfnExecutionInput)
	if err != nil {
		log.Printf("Error starting step function, error: %s", err)
		return events.APIGatewayProxyResponse{StatusCode: 500}, nil
	}
	return events.APIGatewayProxyResponse{StatusCode: 200}, nil
}
