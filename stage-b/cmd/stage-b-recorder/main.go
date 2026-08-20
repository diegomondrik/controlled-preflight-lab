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
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	kind := required("PRODUCER_KIND")
	if kind != "runtime" && kind != "oracle" {
		return fmt.Errorf("producer kind is not allowed")
	}
	repository, candidateSHA, baseSHA := required("REPOSITORY"), required("CANDIDATE_SHA"), required("PROTECTED_BASE_SHA")
	workflowSHA := required("RECORDER_WORKFLOW_SHA")
	if !shaPattern.MatchString(candidateSHA) || !shaPattern.MatchString(baseSHA) || workflowSHA != baseSHA {
		return fmt.Errorf("SHA binding mismatch")
	}
	producerRunID, producerAttempt := requiredInt64("PRODUCER_RUN_ID"), requiredInt("PRODUCER_RUN_ATTEMPT")
	recorderRunID, recorderAttempt := requiredInt64("RECORDER_RUN_ID"), requiredInt("RECORDER_RUN_ATTEMPT")
	client, err := githubapi.New(required("API_URL"), required("GITHUB_TOKEN"))
	if err != nil {
		return err
	}
	var producer model.Run
	receipt, err := client.Get(context.Background(), fmt.Sprintf("/repos/%s/actions/runs/%d", repository, producerRunID), &producer)
	if err != nil {
		return err
	}
	errorsFound := make([]string, 0)
	if producer.ID != producerRunID || producer.RunAttempt != producerAttempt || producer.Event != "pull_request" || strings.SplitN(producer.Path, "@", 2)[0] != required("PRODUCER_WORKFLOW_PATH") || producer.HeadSHA != candidateSHA {
		errorsFound = append(errorsFound, "producer identity mismatch")
	}
	if producer.Status != "completed" {
		errorsFound = append(errorsFound, "producer is not completed")
	}
	validation, conclusion := "passed", "success"
	if producer.Conclusion != "success" || len(errorsFound) != 0 {
		validation, conclusion = "failed", "failure"
	}
	summary := map[string]any{
		"schema_version": "1.0", "kind": kind, "case_id": required("CASE_ID"), "candidate_sha": candidateSHA,
		"protected_base_sha": baseSHA, "producer_run_id": producerRunID, "producer_run_attempt": producerAttempt,
		"producer_workflow_path": required("PRODUCER_WORKFLOW_PATH"), "producer_conclusion": producer.Conclusion,
		"recorder_run_id": recorderRunID, "recorder_run_attempt": recorderAttempt,
		"recorder_workflow_path": required("RECORDER_WORKFLOW_PATH"), "protected_workflow_sha": workflowSHA,
		"validation": validation, "errors": errorsFound,
	}
	summaryBytes, _ := json.Marshal(summary)
	payload := map[string]any{
		"name": "stage-b-" + kind + "-record", "head_sha": candidateSHA, "status": "completed", "conclusion": conclusion,
		"external_id": fmt.Sprintf("stage-b:%s:v1:%s:%d:%d:%d", kind, candidateSHA, producerRunID, recorderRunID, recorderAttempt),
		"output":      map[string]any{"title": "Stage B " + kind + " " + validation, "summary": string(summaryBytes)},
	}
	var created model.CheckRun
	createdReceipt, err := client.Post(context.Background(), fmt.Sprintf("/repos/%s/check-runs", repository), payload, &created)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(map[string]any{"schema_version": "1.0", "producer_receipt": receipt, "created_check_receipt": createdReceipt, "summary": summary})
	fmt.Println(string(encoded))
	if validation != "passed" {
		return fmt.Errorf("producer record failed closed")
	}
	return nil
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
