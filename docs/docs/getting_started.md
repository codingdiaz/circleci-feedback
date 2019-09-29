# Getting Started

The following guide will walk you through installing circleci-feedback in your [AWS](https://aws.amazon.com/) account and creating a [Github application](https://developer.github.com/apps/).

This has been tested on a MacOS. If you are using Windows or linux this should still help, but some commands will not work.

## Prerequisites

* [AWS account](https://aws.amazon.com/)
* [Nodejs](https://nodejs.org/en/)
* [Github Account](https://github.com/)
* [Golang](https://golang.org/)
* A basic understanding of AWS

## Create CircleCI Token

Because this app currently uses the v2 public preview API for CircleCI, a token is required. 

1. Login to [circleci](https://circleci.com)

2. Go to [user settings](https://circleci.com/account)

3. Go to [Personal API Tokens](https://circleci.com/account/api)

4. Create a token and save it for a subsequent step

It is importaint to note that this GitHub app will only work for the GitHub repos the user you create the API token for has read access to. If you are deploying this in a production envrionment you should create a GitHub account and only give it read access to your repositories. 

## Get a KMS Key ARN

The API you deploy is going to need authentication to a couple services with tokens and a private key. To keep these secure, the application uses [AWS Parameter Store](https://docs.aws.amazon.com/systems-manager/latest/userguide/systems-manager-parameter-store.html) to securely retrieve these values, only when needed. 

Before we deploy the API, we need to know what [AWS KMS](https://aws.amazon.com/kms/) key we will use to encrypt/decrypt these sensitive values. (The serverless application needs to know this value at deploy time)

To quickly deploy, you can use the default key Amazon provides in your account. 

With the AWS ClI installed look for the AWS Managed Key with the alias `alias/aws/ssm`

`aws kms describe-key --key-id alias/aws/ssm | jq -r .KeyMetadata.Arn`

You can also find this information in the console.

If you wish to use a different key, just use that ARN instead of the default ssm key ARN. 

Export the ARN as an environment variable:

`export KMS_KEY_ARN=<arn of key>` (this env variable is used by the serverless framework)


## Deploying the API

Before we configure the Github app, lets deploy the infrastructure/code that will power the Github app. Currently this deploys with the [serverless framework](https://serverless.com/).

1. Clone or fork this repo (totally up to you!)

` git clone https://github.com/codingdiaz/circleci-feedback.git`

` cd circleci-feedback`

2. Install NPM Dependencies for serverless (make sure you have node.js installed)

`npm install`

3. Configure [AWS credentials](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-configure.html)
    * For now the best way to deploy this is with Administrator access to your AWS account, a least priviledge IAM policy document is in the works!

4. Build and Deploy the application

`make deploy`

This will complile the lambda function code and run a serverless deploy command. You need go installed to run this command.

5. On a successfull deployment, save the POST URL serverless created and continue on.

`https://<api-id>.execute-api.us-east-1.amazonaws.com/dev/entry`

## Create a Github App

1. Navigate to https://github.com and login

2. Settings > Developer Settings > New GitHub app

3. Fill out New Github app page:

* Name: Name your App something meaningful to you, GitHub apps need to be unique in name across all apps.
* Homepage URL: https://github.com/codingdiaz/circleci-feedback (or whatever you want)
* Webhook URL: Use the POST URL serverless provided from the previous step
* Webhook Secret: Generate a strong random password, put it here and save it, we will use it later
* Permissions:
    * Contents: Read-only (used to check if the repo/branch has a `.circleci/config.yml` file)
    * Metadata: Read-only (required)
    * Pull Requests: Read & Write (used to add comments to pull requests with build output)
* Subscribe to Events:
    * Pull Requests (the only events this triggers on)
* Where can this GitHub App be installed?
    * Only on this account

Click **Create GitHub App**

4. Generate a private Key for you application

5. Install your app to your organization

## Add Secrets to Parameter Store

You now should have a GitHub App created, installed and sending Pull Request webhook events to API gateway. 

The last step is to provide your lambda functions the appropriate authentication to CircleCI and GitHub.

Put the following values into AWS Parameter store as a secure string using the KMS key you used above.

* `/circleci-feedback/GithubWebhookSecret` (the secret value you generated when you created your GitHub app)
* `/circleci-feedback/GithubAppPrivateKey` (the contents of the private key file you downloaded when you created your GitHub App)
* `/circleci-feedback/InstallationID` (the installationID of your GitHub app)
    * This can be found by going to your GitHub App (Your Profile Settings > Developer Settings > GitHub Apps > The About Page on your GitHub App)
* `/circleci-feedback/CircleToken` (the CircleCI token you generated already)


## Test Your Endpoint With Curl

`curl -X POST https://<api-id>.execute-api.us-east-1.amazonaws.com/dev/entry`

You should get a 403 status code, this validates the lambda function is ensuring the request is coming from GitHub. 

If you get a 500 status code, something is misconfigured, likely the lambda function can not read and decrypt the sensitive information from AWS parameter store. Check the lamda function logs for the entry function in CloudWatch.

## Have fun!

At this point you should have a fully functional GitHub application. You can make a CircleCI job fail to validate it is working.


