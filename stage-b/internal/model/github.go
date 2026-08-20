package model

import "encoding/json"

type Run struct {
	ID           int64  `json:"id"`
	RunAttempt   int    `json:"run_attempt"`
	Event        string `json:"event"`
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	HeadSHA      string `json:"head_sha"`
	Path         string `json:"path"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	CheckSuiteID int64  `json:"check_suite_id"`
	Repository   struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repository"`
}

type Jobs struct {
	TotalCount int   `json:"total_count"`
	Jobs       []Job `json:"jobs"`
}

type Job struct {
	ID          int64   `json:"id"`
	RunID       int64   `json:"run_id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Conclusion  *string `json:"conclusion"`
	StartedAt   string  `json:"started_at"`
	CompletedAt *string `json:"completed_at"`
}

type CheckRuns struct {
	TotalCount int        `json:"total_count"`
	CheckRuns  []CheckRun `json:"check_runs"`
}

type CheckRun struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	HeadSHA     string  `json:"head_sha"`
	Status      string  `json:"status"`
	Conclusion  *string `json:"conclusion"`
	ExternalID  string  `json:"external_id"`
	DetailsURL  string  `json:"details_url"`
	StartedAt   string  `json:"started_at"`
	CompletedAt *string `json:"completed_at"`
	CheckSuite  struct {
		ID int64 `json:"id"`
	} `json:"check_suite"`
	App struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	} `json:"app"`
	Output struct {
		Summary string `json:"summary"`
	} `json:"output"`
}

type Pull struct {
	Number         int     `json:"number"`
	State          string  `json:"state"`
	Mergeable      *bool   `json:"mergeable"`
	MergeableState string  `json:"mergeable_state"`
	Head           PullRef `json:"head"`
	Base           PullRef `json:"base"`
}

type PullRef struct {
	SHA  string `json:"sha"`
	Ref  string `json:"ref"`
	Repo struct {
		ID       int64  `json:"id"`
		FullName string `json:"full_name"`
	} `json:"repo"`
}

type RecorderSummary struct {
	SchemaVersion       string   `json:"schema_version"`
	CaseID              string   `json:"case_id"`
	CandidateSHA        string   `json:"candidate_sha"`
	ProtectedBaseSHA    string   `json:"protected_base_sha"`
	FailureRunID        int64    `json:"failure_run_id"`
	FailureRunAttempt   int      `json:"failure_run_attempt"`
	RecorderRunID       int64    `json:"recorder_run_id"`
	RecorderRunAttempt  int      `json:"recorder_run_attempt"`
	RecorderWorkflow    string   `json:"recorder_workflow_path"`
	RecorderWorkflowSHA string   `json:"recorder_workflow_sha"`
	SubmittedDetailsURL string   `json:"submitted_details_url"`
	Validation          string   `json:"validation"`
	Errors              []string `json:"errors"`
}

type Envelope struct {
	SchemaVersion  string          `json:"schema_version"`
	Kind           string          `json:"kind"`
	Classification string          `json:"classification"`
	Reason         string          `json:"reason"`
	Records        json.RawMessage `json:"records,omitempty"`
}
