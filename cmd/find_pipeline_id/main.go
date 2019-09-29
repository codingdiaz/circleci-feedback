package main

import (
	"context"
	"fmt"
	"log"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/codingdiaz/circleci-feedback/internal/stepfunc"
	"github.com/codingdiaz/circleci-feedback/pkg/circleci"
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

	client := circleci.Client{Token: c.CircleToken}
	pipelines, err := client.GetProjectPipelines("gh", in.Owner, in.RepoName)
	if err != nil {
		log.Printf("Error getting pipelineIDs, error: %s", err)
		return in, fmt.Errorf("Error getting pipelineIDs, error: %s", err)
	}

	// look for the pipelineid associated with this commit
	for _, pipeline := range pipelines {
		if pipeline.Vcs.Revision == in.CommitSHA {
			log.Printf("found pipeline id for this commit, %s", pipeline.ID)
			in.PipelineID = pipeline.ID
			return in, nil
		}
	}

	return in, fmt.Errorf("Didn't find a pipeline id yet")

}
