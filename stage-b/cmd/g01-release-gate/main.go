package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"example.invalid/ingol/stage-b/internal/githubapi"
	"example.invalid/ingol/stage-b/internal/model"
)

var (
	shaPattern        = regexp.MustCompile(`^[0-9a-f]{40}$`)
	externalIDPattern = regexp.MustCompile(`^stage-b-g01:v1:([A-Z]-[0-9]{2}):([0-9a-f]{40}):([1-9][0-9]*):([1-9][0-9]*):([1-9][0-9]*)$`)
)

type snapshot struct {
	ImpersonatorRun  githubapi.Receipt `json:"impersonator_run"`
	ImpersonatorJobs githubapi.Receipt `json:"impersonator_jobs"`
	CheckRuns        githubapi.Receipt `json:"check_runs"`
	FailureRun       githubapi.Receipt `json:"failure_run"`
	RecorderRun      githubapi.Receipt `json:"recorder_run"`
	Pull             githubapi.Receipt `json:"pull"`
}

func main() {
	if err := run(); err != nil {
		printJSON(map[string]any{"schema_version": "1.0", "classification": "inconclusive", "reason": err.Error()})
		os.Exit(1)
	}
}

func run() error {
	repository := required("REPOSITORY")
	candidateRepo := required("CANDIDATE_REPOSITORY")
	candidateSHA := required("CANDIDATE_SHA")
	baseSHA := required("PROTECTED_BASE_SHA")
	caseID := required("CASE_ID")
	prNumber := requiredInt("PR_NUMBER")
	impersonatorRunID := requiredInt64("IMPERSONATOR_RUN_ID")
	impersonatorAttempt := requiredInt("IMPERSONATOR_RUN_ATTEMPT")
	failureAttempt := requiredInt("FAILURE_RUN_ATTEMPT")
	recorderAttempt := requiredInt("RECORDER_RUN_ATTEMPT")
	impersonatorPath := required("IMPERSONATOR_WORKFLOW_PATH")
	failurePath := required("FAILURE_WORKFLOW_PATH")
	recorderPath := required("RECORDER_WORKFLOW_PATH")
	deadlineSeconds := requiredInt("POLL_DEADLINE_SECONDS")
	intervalSeconds := requiredInt("POLL_INTERVAL_SECONDS")
	if deadlineSeconds > 300 || intervalSeconds > 15 || intervalSeconds >= deadlineSeconds {
		return fmt.Errorf("polling bounds exceed the reviewed ceiling")
	}
	if !shaPattern.MatchString(candidateSHA) || !shaPattern.MatchString(baseSHA) {
		return fmt.Errorf("invalid candidate or base SHA")
	}
	client, err := githubapi.New(required("API_URL"), required("GITHUB_TOKEN"))
	if err != nil {
		return err
	}
	deadline := time.Now().Add(time.Duration(deadlineSeconds) * time.Second)
	for {
		ready, records, checkErr := evaluate(context.Background(), client, repository, candidateRepo, candidateSHA, baseSHA, caseID, prNumber, impersonatorRunID, impersonatorAttempt, failureAttempt, recorderAttempt, impersonatorPath, failurePath, recorderPath)
		if checkErr != nil {
			return checkErr
		}
		if ready {
			printJSON(map[string]any{
				"schema_version":     "1.0",
				"classification":     "allow_success",
				"candidate_sha":      candidateSHA,
				"protected_base_sha": baseSHA,
				"observed_at":        time.Now().UTC().Format(time.RFC3339Nano),
				"records":            records,
			})
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("bounded predicate deadline expired")
		}
		time.Sleep(time.Duration(intervalSeconds) * time.Second)
	}
}

func evaluate(ctx context.Context, client *githubapi.Client, repository, candidateRepo, candidateSHA, baseSHA, caseID string, prNumber int, impersonatorRunID int64, impersonatorAttempt, failureAttempt, recorderAttempt int, impersonatorPath, failurePath, recorderPath string) (bool, snapshot, error) {
	var records snapshot
	var impersonator model.Run
	var err error
	records.ImpersonatorRun, err = client.Get(ctx, fmt.Sprintf("/repos/%s/actions/runs/%d", repository, impersonatorRunID), &impersonator)
	if err != nil {
		return false, records, err
	}
	if impersonator.ID != impersonatorRunID || impersonator.RunAttempt != impersonatorAttempt || impersonator.Event != "pull_request" || cleanPath(impersonator.Path) != impersonatorPath || impersonator.HeadSHA != candidateSHA {
		return false, records, fmt.Errorf("impersonator run identity mismatch")
	}
	if impersonator.Status != "in_progress" || impersonator.Conclusion != "" {
		return false, records, fmt.Errorf("impersonator run is not held in progress")
	}

	var jobs model.Jobs
	records.ImpersonatorJobs, err = client.Get(ctx, fmt.Sprintf("/repos/%s/actions/runs/%d/jobs?filter=latest&per_page=100", repository, impersonatorRunID), &jobs)
	if err != nil {
		return false, records, err
	}
	held := make([]model.Job, 0, 1)
	for _, job := range jobs.Jobs {
		if job.Name == "protected-decision" && job.RunID == impersonatorRunID {
			held = append(held, job)
		}
	}
	if len(held) != 1 || held[0].Status != "in_progress" || held[0].Conclusion != nil {
		return false, records, fmt.Errorf("impersonator held job is absent, duplicate or concluded")
	}

	var checks model.CheckRuns
	records.CheckRuns, err = client.Get(ctx, fmt.Sprintf("/repos/%s/commits/%s/check-runs?check_name=protected-decision&filter=all&per_page=100", repository, candidateSHA), &checks)
	if err != nil {
		return false, records, err
	}
	authoritative := make([]model.CheckRun, 0, 1)
	for _, check := range checks.CheckRuns {
		parts := externalIDPattern.FindStringSubmatch(check.ExternalID)
		if len(parts) == 6 && parts[1] == caseID && parts[2] == candidateSHA && parts[5] == strconv.Itoa(recorderAttempt) {
			authoritative = append(authoritative, check)
		}
	}
	if len(authoritative) == 0 {
		return false, records, nil
	}
	if len(authoritative) != 1 {
		return false, records, fmt.Errorf("authoritative protected failure is duplicate")
	}
	auth := authoritative[0]
	if auth.Name != "protected-decision" || auth.HeadSHA != candidateSHA || auth.Status != "completed" || auth.Conclusion == nil || *auth.Conclusion != "failure" || auth.CompletedAt == nil || auth.App.Slug != "github-actions" {
		return false, records, fmt.Errorf("authoritative protected failure fields mismatch")
	}
	parts := externalIDPattern.FindStringSubmatch(auth.ExternalID)
	failureRunID := parsePositiveInt64(parts[3])
	recorderRunID := parsePositiveInt64(parts[4])
	if failureRunID == 0 || recorderRunID == 0 {
		return false, records, fmt.Errorf("authoritative external ID is invalid")
	}
	expectedDetails := fmt.Sprintf("https://github.com/%s/actions/runs/%d", repository, recorderRunID)
	providerCanonicalDetails := fmt.Sprintf("https://github.com/%s/runs/%d", repository, auth.ID)
	if auth.DetailsURL != expectedDetails && auth.DetailsURL != providerCanonicalDetails {
		return false, records, fmt.Errorf("authoritative details URL is neither the submitted recorder URL nor the provider-canonical check URL")
	}
	var summary model.RecorderSummary
	if err := json.Unmarshal([]byte(auth.Output.Summary), &summary); err != nil {
		return false, records, fmt.Errorf("authoritative summary is invalid JSON: %w", err)
	}
	if summary.SchemaVersion != "1.0" || summary.CaseID != caseID || summary.CandidateSHA != candidateSHA || summary.ProtectedBaseSHA != baseSHA || summary.FailureRunID != failureRunID || summary.FailureRunAttempt != failureAttempt || summary.RecorderRunID != recorderRunID || summary.RecorderRunAttempt != recorderAttempt || summary.RecorderWorkflow != recorderPath || summary.RecorderWorkflowSHA != baseSHA || summary.SubmittedDetailsURL != expectedDetails || summary.Validation != "passed" || len(summary.Errors) != 0 {
		return false, records, fmt.Errorf("authoritative summary binding mismatch")
	}

	var failure model.Run
	records.FailureRun, err = client.Get(ctx, fmt.Sprintf("/repos/%s/actions/runs/%d", repository, failureRunID), &failure)
	if err != nil {
		return false, records, err
	}
	if failure.ID != failureRunID || failure.RunAttempt != failureAttempt || failure.Event != "pull_request" || cleanPath(failure.Path) != failurePath || failure.HeadSHA != candidateSHA || failure.Status != "completed" || failure.Conclusion == "success" {
		return false, records, fmt.Errorf("failure producer binding mismatch")
	}

	var recorder model.Run
	records.RecorderRun, err = client.Get(ctx, fmt.Sprintf("/repos/%s/actions/runs/%d", repository, recorderRunID), &recorder)
	if err != nil {
		return false, records, err
	}
	if recorder.ID != recorderRunID || recorder.RunAttempt != recorderAttempt || recorder.Event != "workflow_run" || cleanPath(recorder.Path) != recorderPath || recorder.HeadSHA != baseSHA || recorder.Repository.FullName != repository {
		return false, records, fmt.Errorf("recorder run binding mismatch")
	}

	var pull model.Pull
	records.Pull, err = client.Get(ctx, fmt.Sprintf("/repos/%s/pulls/%d", repository, prNumber), &pull)
	if err != nil {
		return false, records, err
	}
	if pull.Number != prNumber || pull.State != "open" || pull.Head.Repo.FullName != candidateRepo || pull.Head.SHA != candidateSHA || pull.Base.SHA != baseSHA {
		return false, records, fmt.Errorf("live pull-request identity drift")
	}
	if pull.Mergeable == nil || pull.MergeableState == "unknown" {
		return false, records, nil
	}
	if pull.MergeableState != "blocked" {
		return false, records, fmt.Errorf("pull request is not blocked")
	}
	createdAt, err := time.Parse(time.RFC3339, impersonator.CreatedAt)
	if err != nil {
		return false, records, fmt.Errorf("invalid impersonator created_at")
	}
	completedAt, err := time.Parse(time.RFC3339, *auth.CompletedAt)
	if err != nil {
		return false, records, fmt.Errorf("invalid authoritative completed_at")
	}
	if !createdAt.Before(completedAt) {
		return false, records, fmt.Errorf("impersonator did not predate authoritative completion")
	}
	return true, records, nil
}

func cleanPath(path string) string { return strings.SplitN(path, "@", 2)[0] }
func parsePositiveInt64(raw string) int64 {
	value, _ := strconv.ParseInt(raw, 10, 64)
	if value < 1 {
		return 0
	}
	return value
}
func required(name string) string {
	value := os.Getenv(name)
	if value == "" {
		panic("missing environment variable: " + name)
	}
	return value
}
func requiredInt(name string) int {
	value, err := strconv.Atoi(required(name))
	if err != nil || value < 1 {
		panic("invalid positive integer: " + name)
	}
	return value
}
func requiredInt64(name string) int64 {
	value, err := strconv.ParseInt(required(name), 10, 64)
	if err != nil || value < 1 {
		panic("invalid positive integer: " + name)
	}
	return value
}
func printJSON(value any) { encoded, _ := json.Marshal(value); fmt.Println(string(encoded)) }
