package circleci

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

const (
	queryLimit = 100 // maximum that CircleCI allows
)

var (
	defaultBaseURL = &url.URL{Host: "circleci.com", Scheme: "https", Path: "/api/v2/"}
	defaultLogger  = log.New(os.Stderr, "", log.LstdFlags)
)

// Logger is a minimal interface for injecting custom logging logic for debug logs
type Logger interface {
	Printf(fmt string, args ...interface{})
}

// APIError represents an error from CircleCI
type APIError struct {
	HTTPStatusCode int
	Message        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%d: %s", e.HTTPStatusCode, e.Message)
}

// Client is a CircleCI client
// Its zero value is a usable client for examining public CircleCI repositories
type Client struct {
	BaseURL    *url.URL     // CircleCI API endpoint (defaults to DefaultEndpoint)
	Token      string       // CircleCI API token (needed for private repositories and mutative actions)
	HTTPClient *http.Client // HTTPClient to use for connecting to CircleCI (defaults to http.DefaultClient)

	Debug  bool   // debug logging enabled
	Logger Logger // logger to send debug messages on (if enabled), defaults to logging to stderr with the standard flags
}

func (c *Client) baseURL() *url.URL {
	if c.BaseURL == nil {
		return defaultBaseURL
	}

	return c.BaseURL
}

func (c *Client) client() *http.Client {
	if c.HTTPClient == nil {
		return http.DefaultClient
	}

	return c.HTTPClient
}

func (c *Client) logger() Logger {
	if c.Logger == nil {
		return defaultLogger
	}

	return c.Logger
}

func (c *Client) debug(format string, args ...interface{}) {
	if c.Debug {
		c.logger().Printf(format, args...)
	}
}

func (c *Client) debugRequest(req *http.Request) {
	if c.Debug {
		out, err := httputil.DumpRequestOut(req, true)
		if err != nil {
			c.debug("error debugging request %+v: %s", req, err)
		}
		c.debug("request:\n%+v", string(out))
	}
}

func (c *Client) debugResponse(resp *http.Response) {
	if c.Debug {
		out, err := httputil.DumpResponse(resp, true)
		if err != nil {
			c.debug("error debugging response %+v: %s", resp, err)
		}
		c.debug("response:\n%+v", string(out))
	}
}

type nopCloser struct {
	io.Reader
}

func (n nopCloser) Close() error { return nil }

func (c *Client) request(method, path string, responseStruct interface{}, params url.Values, bodyStruct interface{}) error {
	if params == nil {
		params = url.Values{}
	}
	params.Set("circle-token", c.Token)

	u := c.baseURL().ResolveReference(&url.URL{Path: path, RawQuery: params.Encode()})

	c.debug("building request for %s", u)

	req, err := http.NewRequest(method, u.String(), nil)
	if err != nil {
		return err
	}

	if bodyStruct != nil {
		b, err := json.Marshal(bodyStruct)
		if err != nil {
			return err
		}

		req.Body = nopCloser{bytes.NewBuffer(b)}
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")

	c.debugRequest(req)

	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	c.debugResponse(resp)

	if resp.StatusCode >= 300 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return &APIError{HTTPStatusCode: resp.StatusCode, Message: "unable to parse response: %s"}
		}

		if len(body) > 0 {
			message := struct {
				Message string `json:"message"`
			}{}
			err = json.Unmarshal(body, &message)
			if err != nil {
				return &APIError{
					HTTPStatusCode: resp.StatusCode,
					Message:        fmt.Sprintf("unable to parse API response: %s", err),
				}
			}
			return &APIError{HTTPStatusCode: resp.StatusCode, Message: message.Message}
		}

		return &APIError{HTTPStatusCode: resp.StatusCode}
	}

	if responseStruct != nil {
		err = json.NewDecoder(resp.Body).Decode(responseStruct)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetBuild fetches a given build by number
func (c *Client) GetBuild(vcs, account, repo string, buildNum int) (*Build, error) {
	build := &Build{}

	err := c.request("GET", fmt.Sprintf("project/%s/%s/%s/%d", vcs, account, repo, buildNum), build, nil, nil)
	if err != nil {
		return nil, err
	}

	return build, nil
}

// Me returns information about the current user
func (c *Client) Me() (*User, error) {
	user := &User{}

	err := c.request("GET", "me", user, nil, nil)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetProject gets a project with the v2 API
func (c *Client) GetProject(vcsProvider, account, repo string) (*Project, error) {
	project := &Project{}

	err := c.request("GET", fmt.Sprintf("project/%s/%s/%s", vcsProvider, account, repo), project, nil, nil)
	if err != nil {
		return nil, err
	}

	return project, nil
}

// GetWorkflow gets a worflow with the v2 API
func (c *Client) GetWorkflow(workflowID string) (*Workflow, error) {
	workflow := &Workflow{}
	err := c.request("GET", fmt.Sprintf("workflow/%s", workflowID), workflow, nil, nil)
	if err != nil {
		return nil, err
	}

	return workflow, nil

}

// GetWorkflowJobs gets jobs associated with a workflow with the V2 API
func (c *Client) GetWorkflowJobs(workflowID string) ([]Job, error) {
	resp := &GetWorkflowJobsResponse{}
	err := c.request("GET", fmt.Sprintf("workflow/%s/jobs", workflowID), resp, nil, nil)
	if err != nil {
		return nil, err
	}

	return resp.Items, nil
}

// GetProjectPipelines gets recent pipelines for a repo with the V2 API
func (c *Client) GetProjectPipelines(vcsProvider, account, repo string) ([]Pipeline, error) {
	resp := &GetProjectPipelinesResponse{}

	err := c.request("GET", fmt.Sprintf("project/%s/%s/%s/pipeline", vcsProvider, account, repo), resp, nil, nil)
	if err != nil {
		return nil, err
	}

	return resp.Items, nil
}

// GetPipeline gets a specific Pipeline with the V2 API
func (c *Client) GetPipeline(pipelineID string) (*Pipeline, error) {
	pipeline := &Pipeline{}
	err := c.request("GET", fmt.Sprintf("pipeline/%s", pipelineID), pipeline, nil, nil)
	if err != nil {
		return pipeline, err
	}

	return pipeline, nil
}

func GetBuildOutput(buildOutputURL string) ([]BuildOutput, error) {

	output := &[]BuildOutput{}

	req, err := http.NewRequest("GET", buildOutputURL, nil)
	if err != nil {
		return nil, nil
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, nil
	}

	err = json.Unmarshal(body, &output)
	if err != nil {
		return nil, err
	}

	return *output, nil
}

// func (c *Client)GetPipelineConfig(pipelineID string) (*PipelineConfig, error) {
// 	pipelineConfig := &PipelineConfig{}
// 	err := c.request("GET", fmt.Sprintf("pipeline/%s/config", pipelineID), pipelineConfig, nil, nil)
// 	if err != nil {
// 		return pipelineConfig, err
// 	}

// 	return pipelineConfig, nil
// }

// type PipelineConfig struct {
// 	Source   string `json:"source"`
// 	Compiled string `json:"compiled"`
// }

type GetProjectPipelinesResponse struct {
	Items         []Pipeline  `json:"items"`
	NextPageToken interface{} `json:"next_page_token"`
}

type Pipeline struct {
	Workflows []struct {
		ID string `json:"id"`
	} `json:"workflows"`
	ID          string        `json:"id"`
	Errors      []interface{} `json:"errors"`
	ProjectSlug string        `json:"project_slug"`
	UpdatedAt   time.Time     `json:"updated_at"`
	Number      int           `json:"number"`
	State       string        `json:"state"`
	CreatedAt   time.Time     `json:"created_at"`
	Trigger     struct {
		Type       string    `json:"type"`
		ReceivedAt time.Time `json:"received_at"`
		Actor      struct {
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		} `json:"actor"`
	} `json:"trigger"`
	Vcs struct {
		ProviderName        string `json:"provider_name"`
		OriginRepositoryURL string `json:"origin_repository_url"`
		TargetRepositoryURL string `json:"target_repository_url"`
		Revision            string `json:"revision"`
		Branch              string `json:"branch"`
	} `json:"vcs"`
}

type GetWorkflowJobsResponse struct {
	NextPageToken interface{} `json:"next_page_token"`
	Items         []Job       `json:"items"`
}

type Job struct {
	Dependencies []string  `json:"dependencies"`
	JobNumber    int       `json:"job_number"`
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ProjectSlug  string    `json:"project_slug"`
	Status       string    `json:"status"`
	StopTime     time.Time `json:"stop_time"`
	Type         string    `json:"type"`
	StartTime    time.Time `json:"start_time"`
}

type Workflow struct {
	CreatedAt      time.Time `json:"created_at"`
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	PipelineID     string    `json:"pipeline_id"`
	PipelineNumber int       `json:"pipeline_number"`
	ProjectSlug    string    `json:"project_slug"`
	Status         string    `json:"status"`
	StoppedAt      time.Time `json:"stopped_at"`

	// v1
	JobName        string    `json:"job_name"`
	JobID          string    `json:"job_id"`
	UpstreamJobIds []*string `json:"upstream_job_ids"`
	WorkflowID     string    `json:"workflow_id"`
	WorkspaceID    string    `json:"workspace_id"`
	WorkflowName   string    `json:"workflow_name"`
}

// User represents a CircleCI user
type User struct {
	Admin               bool      `json:"admin"`
	AllEmails           []string  `json:"all_emails"`
	AvatarURL           string    `json:"avatar_url"`
	BasicEmailPrefs     string    `json:"basic_email_prefs"`
	Containers          int       `json:"containers"`
	CreatedAt           time.Time `json:"created_at"`
	DaysLeftInTrial     int       `json:"days_left_in_trial"`
	GithubID            int       `json:"github_id"`
	GithubOauthScopes   []string  `json:"github_oauth_scopes"`
	GravatarID          *string   `json:"gravatar_id"`
	HerokuAPIKey        *string   `json:"heroku_api_key"`
	LastViewedChangelog time.Time `json:"last_viewed_changelog"`
	Login               string    `json:"login"`
	Name                *string   `json:"name"`
	Parallelism         int       `json:"parallelism"`
	Plan                *string   `json:"plan"`
	//Projects            map[string]*UserProject `json:"projects"`
	SelectedEmail *string   `json:"selected_email"`
	SignInCount   int       `json:"sign_in_count"`
	TrialEnd      time.Time `json:"trial_end"`
}

type Project struct {
	Slug             string `json:"slug"`
	Name             string `json:"name"`
	OrganizationName string `json:"organization_name"`
	VcsInfo          struct {
		VcsURL        string `json:"vcs_url"`
		Provider      string `json:"provider"`
		DefaultBranch string `json:"default_branch"`
	} `json:"vcs_info"`
}

// BuildStatus represents status information about the build
// Used when a short summary of previous builds is included
type BuildStatus struct {
	BuildTimeMillis int    `json:"build_time_millis"`
	Status          string `json:"status"`
	BuildNum        int    `json:"build_num"`
}

// BuildUser represents the user that triggered the build
type BuildUser struct {
	Email  *string `json:"email"`
	IsUser bool    `json:"is_user"`
	Login  string  `json:"login"`
	Name   *string `json:"name"`
}

// Build represents the details of a build
type Build struct {
	//AllCommitDetails        []*CommitDetails  `json:"all_commit_details"`
	AuthorDate         *time.Time        `json:"author_date"`
	AuthorEmail        string            `json:"author_email"`
	AuthorName         string            `json:"author_name"`
	Body               string            `json:"body"`
	Branch             string            `json:"branch"`
	BuildNum           int               `json:"build_num"`
	BuildParameters    map[string]string `json:"build_parameters"`
	BuildTimeMillis    *int              `json:"build_time_millis"`
	BuildURL           string            `json:"build_url"`
	Canceled           bool              `json:"canceled"`
	CircleYML          *CircleYML        `json:"circle_yml"`
	CommitterDate      *time.Time        `json:"committer_date"`
	CommitterEmail     string            `json:"committer_email"`
	CommitterName      string            `json:"committer_name"`
	Compare            *string           `json:"compare"`
	DontBuild          *string           `json:"dont_build"`
	Failed             *bool             `json:"failed"`
	FeatureFlags       map[string]string `json:"feature_flags"`
	InfrastructureFail bool              `json:"infrastructure_fail"`
	IsFirstGreenBuild  bool              `json:"is_first_green_build"`
	JobName            *string           `json:"job_name"`
	Lifecycle          string            `json:"lifecycle"`
	Messages           []*Message        `json:"messages"`
	//Node                    []*Node           `json:"node"`
	OSS      bool   `json:"oss"`
	Outcome  string `json:"outcome"`
	Parallel int    `json:"parallel"`
	//Picard                  *Picard           `json:"picard"`
	Platform                string       `json:"platform"`
	Previous                *BuildStatus `json:"previous"`
	PreviousSuccessfulBuild *BuildStatus `json:"previous_successful_build"`
	//PullRequests            []*PullRequest    `json:"pull_requests"`
	QueuedAt string `json:"queued_at"`
	Reponame string `json:"reponame"`
	Retries  []int  `json:"retries"`
	RetryOf  *int   `json:"retry_of"`
	//SSHEnabled              *bool             `json:"ssh_enabled"`
	//SSHUsers                []*SSHUser        `json:"ssh_users"`
	StartTime     *time.Time `json:"start_time"`
	Status        string     `json:"status"`
	Steps         []*Step    `json:"steps"`
	StopTime      *time.Time `json:"stop_time"`
	Subject       string     `json:"subject"`
	Timedout      bool       `json:"timedout"`
	UsageQueuedAt string     `json:"usage_queued_at"`
	User          *BuildUser `json:"user"`
	Username      string     `json:"username"`
	VcsRevision   string     `json:"vcs_revision"`
	VcsTag        string     `json:"vcs_tag"`
	VCSURL        string     `json:"vcs_url"`
	Workflows     *Workflow  `json:"workflows"`
	Why           string     `json:"why"`
}

// Message represents build messages
type Message struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// Step represents an individual step in a build
// Will contain more than one action if the step was parallelized
type Step struct {
	Name    string    `json:"name"`
	Actions []*Action `json:"actions"`
}

// Action represents an individual action within a build step
type Action struct {
	Background         bool       `json:"background"`
	BashCommand        *string    `json:"bash_command"`
	Canceled           *bool      `json:"canceled"`
	Continue           *string    `json:"continue"`
	EndTime            *time.Time `json:"end_time"`
	ExitCode           *int       `json:"exit_code"`
	Failed             *bool      `json:"failed"`
	HasOutput          bool       `json:"has_output"`
	Index              int        `json:"index"`
	InfrastructureFail *bool      `json:"infrastructure_fail"`
	Messages           []string   `json:"messages"`
	Name               string     `json:"name"`
	OutputURL          string     `json:"output_url"`
	Parallel           bool       `json:"parallel"`
	RunTimeMillis      int        `json:"run_time_millis"`
	StartTime          *time.Time `json:"start_time"`
	Status             string     `json:"status"`
	Step               int        `json:"step"`
	Timedout           *bool      `json:"timedout"`
	Truncated          bool       `json:"truncated"`
	Type               string     `json:"type"`
}

// CircleYML represents the serialized CircleCI YML file for a given build
type CircleYML struct {
	String string `json:"string"`
}

type BuildOutput struct {
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
}
