package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"example.invalid/ingol/stage-b/internal/githubapi"
	"example.invalid/ingol/stage-b/internal/model"
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "inconclusive:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	repository := required("REPOSITORY")
	candidateSHA := required("CANDIDATE_SHA")
	baseSHA := required("PROTECTED_BASE_SHA")
	caseID := required("CASE_ID")
	failureRunID := requiredInt64("FAILURE_RUN_ID")
	failureAttempt := requiredInt("FAILURE_RUN_ATTEMPT")
	recorderRunID := requiredInt64("RECORDER_RUN_ID")
	recorderAttempt := requiredInt("RECORDER_RUN_ATTEMPT")
	recorderPath := required("RECORDER_WORKFLOW_PATH")
	recorderWorkflowSHA := required("RECORDER_WORKFLOW_SHA")
	failurePath := required("FAILURE_WORKFLOW_PATH")

	if !shaPattern.MatchString(candidateSHA) || !shaPattern.MatchString(baseSHA) || !shaPattern.MatchString(recorderWorkflowSHA) {
		return fmt.Errorf("one or more SHA bindings are not exact lowercase commit identities")
	}
	if recorderWorkflowSHA != baseSHA {
		return fmt.Errorf("recorder workflow SHA does not equal protected base SHA")
	}
	client, err := githubapi.New(required("API_URL"), required("GITHUB_TOKEN"))
	if err != nil {
		return err
	}

	var failure model.Run
	failureReceipt, err := client.Get(ctx, fmt.Sprintf("/repos/%s/actions/runs/%d", repository, failureRunID), &failure)
	if err != nil {
		printReceipt(failureReceipt)
		return err
	}
	errorsFound := make([]string, 0)
	if failure.ID != failureRunID || failure.RunAttempt != failureAttempt {
		errorsFound = append(errorsFound, "failure run identity mismatch")
	}
	if cleanWorkflowPath(failure.Path) != failurePath || failure.Event != "pull_request" {
		errorsFound = append(errorsFound, "failure workflow source mismatch")
	}
	if failure.HeadSHA != candidateSHA {
		errorsFound = append(errorsFound, "failure run candidate SHA mismatch")
	}
	if failure.Status != "completed" || failure.Conclusion == "success" || failure.Conclusion == "neutral" || failure.Conclusion == "skipped" {
		errorsFound = append(errorsFound, "failure producer did not complete with the selected authoritative non-success")
	}

	summary := model.RecorderSummary{
		SchemaVersion:       "1.0",
		CaseID:              caseID,
		CandidateSHA:        candidateSHA,
		ProtectedBaseSHA:    baseSHA,
		FailureRunID:        failureRunID,
		FailureRunAttempt:   failureAttempt,
		RecorderRunID:       recorderRunID,
		RecorderRunAttempt:  recorderAttempt,
		RecorderWorkflow:    recorderPath,
		RecorderWorkflowSHA: recorderWorkflowSHA,
		SubmittedDetailsURL: fmt.Sprintf("https://github.com/%s/actions/runs/%d", repository, recorderRunID),
		Validation:          "passed",
		Errors:              errorsFound,
	}
	if len(errorsFound) != 0 {
		summary.Validation = "failed"
	}
	summaryBytes, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	conclusion := "failure"
	if summary.Validation != "passed" {
		conclusion = "failure"
	}
	externalID := fmt.Sprintf("stage-b-g01:v1:%s:%s:%d:%d:%d", caseID, candidateSHA, failureRunID, recorderRunID, recorderAttempt)
	payload := map[string]any{
		"name":        "protected-decision",
		"head_sha":    candidateSHA,
		"status":      "completed",
		"conclusion":  conclusion,
		"external_id": externalID,
		"details_url": summary.SubmittedDetailsURL,
		"output": map[string]any{
			"title":   "Stage B G-01 authoritative protected failure",
			"summary": string(summaryBytes),
		},
	}
	var created model.CheckRun
	createReceipt, err := client.Post(ctx, fmt.Sprintf("/repos/%s/check-runs", repository), payload, &created)
	if err != nil {
		printReceipt(createReceipt)
		return err
	}
	if created.HeadSHA != candidateSHA || created.Name != "protected-decision" || created.ExternalID != externalID {
		return fmt.Errorf("created check response does not preserve exact check identity")
	}
	printJSON(map[string]any{
		"schema_version": "1.0",
		"classification": "authoritative_failure_recorded",
		"failure_run":    failureReceipt,
		"created_check":  createReceipt,
		"summary":        summary,
	})
	if len(errorsFound) != 0 {
		return fmt.Errorf("recorder source validation failed closed")
	}
	return nil
}

func cleanWorkflowPath(path string) string {
	return strings.SplitN(path, "@", 2)[0]
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
		panic("invalid positive integer environment variable: " + name)
	}
	return value
}

func requiredInt64(name string) int64 {
	value, err := strconv.ParseInt(required(name), 10, 64)
	if err != nil || value < 1 {
		panic("invalid positive integer environment variable: " + name)
	}
	return value
}

func printReceipt(receipt githubapi.Receipt) { printJSON(receipt) }

func printJSON(value any) {
	encoded, _ := json.Marshal(value)
	fmt.Println(string(encoded))
}
