package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

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
	repository, candidateSHA, baseSHA := required("REPOSITORY"), required("CANDIDATE_SHA"), required("PROTECTED_BASE_SHA")
	if !shaPattern.MatchString(candidateSHA) || !shaPattern.MatchString(baseSHA) || required("WORKFLOW_SHA") != baseSHA {
		return fmt.Errorf("protected decision SHA binding mismatch")
	}
	client, err := githubapi.New(required("API_URL"), required("GITHUB_TOKEN"))
	if err != nil {
		return err
	}
	deadline := time.Now().Add(180 * time.Second)
	var receipts []githubapi.Receipt
	var selected map[string]model.CheckRun
	for {
		var checks model.CheckRuns
		receipt, getErr := client.Get(context.Background(), fmt.Sprintf("/repos/%s/commits/%s/check-runs?filter=all&per_page=100", repository, candidateSHA), &checks)
		if getErr != nil {
			return getErr
		}
		receipts = append(receipts, receipt)
		selected, err = exactRequired(checks.CheckRuns)
		if err != nil {
			return err
		}
		if len(selected) == 3 {
			break
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("required producer records did not become exact within deadline")
		}
		time.Sleep(5 * time.Second)
	}
	errorsFound := make([]string, 0)
	caseID := ""
	for kind, check := range selected {
		if check.HeadSHA != candidateSHA || check.Status != "completed" || check.Conclusion == nil || *check.Conclusion != "success" || check.App.Slug != "github-actions" {
			errorsFound = append(errorsFound, kind+" record is not an attributable success")
		}
		var summary map[string]any
		if json.Unmarshal([]byte(check.Output.Summary), &summary) != nil {
			errorsFound = append(errorsFound, kind+" summary is invalid JSON")
			continue
		}
		if summary["candidate_sha"] != candidateSHA || summary["protected_base_sha"] != baseSHA || summary["kind"] != kind || summary["validation"] != "passed" {
			errorsFound = append(errorsFound, kind+" summary binding mismatch")
		}
		observedCase, _ := summary["case_id"].(string)
		if caseID == "" {
			caseID = observedCase
		} else if observedCase != caseID {
			errorsFound = append(errorsFound, "producer case IDs differ")
		}
	}
	validation, conclusion := "passed", "success"
	if len(errorsFound) != 0 {
		validation, conclusion = "failed", "failure"
	}
	summary := map[string]any{
		"schema_version": "1.0", "case_id": caseID, "candidate_sha": candidateSHA, "protected_base_sha": baseSHA,
		"governance_check_id": selected["governance"].ID, "runtime_check_id": selected["runtime"].ID, "oracle_check_id": selected["oracle"].ID,
		"decision_run_id": requiredInt64("RUN_ID"), "decision_run_attempt": requiredInt("RUN_ATTEMPT"), "validation": validation, "errors": errorsFound,
	}
	summaryBytes, _ := json.Marshal(summary)
	payload := map[string]any{
		"name": "protected-decision", "head_sha": candidateSHA, "status": "completed", "conclusion": conclusion,
		"external_id": fmt.Sprintf("stage-b:decision:v1:%s:%d:%d", candidateSHA, requiredInt64("RUN_ID"), requiredInt("RUN_ATTEMPT")),
		"output":      map[string]any{"title": "Stage B protected decision " + validation, "summary": string(summaryBytes)},
	}
	var created model.CheckRun
	createReceipt, err := client.Post(context.Background(), fmt.Sprintf("/repos/%s/check-runs", repository), payload, &created)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(map[string]any{"schema_version": "1.0", "record_receipts": receipts, "created_check_receipt": createReceipt, "summary": summary})
	fmt.Println(string(encoded))
	if validation != "passed" {
		return fmt.Errorf("protected conjunction failed closed")
	}
	return nil
}

func exactRequired(checks []model.CheckRun) (map[string]model.CheckRun, error) {
	names := map[string]string{"stage-b-governance-record": "governance", "stage-b-runtime-record": "runtime", "stage-b-oracle-record": "oracle"}
	selected := make(map[string]model.CheckRun)
	for _, check := range checks {
		kind, ok := names[check.Name]
		if !ok {
			continue
		}
		if _, duplicate := selected[kind]; duplicate {
			return nil, fmt.Errorf("duplicate %s record", kind)
		}
		selected[kind] = check
	}
	return selected, nil
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
