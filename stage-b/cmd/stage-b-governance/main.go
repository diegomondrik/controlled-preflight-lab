package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"

	"example.invalid/ingol/stage-b/internal/githubapi"
	"example.invalid/ingol/stage-b/internal/model"
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type casesManifest struct {
	SchemaVersion string `json:"schema_version"`
	Cases         []struct {
		CaseID               string   `json:"case_id"`
		Branch               string   `json:"branch"`
		AllowedChangedPaths  []string `json:"allowed_changed_paths"`
		RequiredChangedPaths []string `json:"required_changed_paths"`
	} `json:"cases"`
	FrozenPaths []string `json:"frozen_candidate_paths"`
}

type pullFile struct {
	Filename string `json:"filename"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	repository, sourceRepo, sourceRef := required("REPOSITORY"), required("SOURCE_REPOSITORY"), required("SOURCE_REF")
	candidateSHA, baseSHA, workflowSHA := required("CANDIDATE_SHA"), required("PROTECTED_BASE_SHA"), required("WORKFLOW_SHA")
	if !shaPattern.MatchString(candidateSHA) || !shaPattern.MatchString(baseSHA) || workflowSHA != baseSHA {
		return fmt.Errorf("protected workflow/base or candidate SHA binding mismatch")
	}
	rawManifest, err := os.ReadFile(required("CASES_MANIFEST"))
	if err != nil {
		return err
	}
	var manifest casesManifest
	if err := json.Unmarshal(rawManifest, &manifest); err != nil || manifest.SchemaVersion != "1.0" {
		return fmt.Errorf("invalid cases manifest")
	}
	var selected *struct {
		CaseID               string   `json:"case_id"`
		Branch               string   `json:"branch"`
		AllowedChangedPaths  []string `json:"allowed_changed_paths"`
		RequiredChangedPaths []string `json:"required_changed_paths"`
	}
	for index := range manifest.Cases {
		if manifest.Cases[index].Branch == sourceRef {
			selected = &manifest.Cases[index]
		}
	}
	if selected == nil {
		return fmt.Errorf("candidate branch is not enumerated")
	}

	client, err := githubapi.New(required("API_URL"), required("GITHUB_TOKEN"))
	if err != nil {
		return err
	}
	var files []pullFile
	filesReceipt, err := client.Get(context.Background(), fmt.Sprintf("/repos/%s/pulls/%s/files?per_page=100", repository, required("PR_NUMBER")), &files)
	if err != nil {
		return err
	}
	changed := make(map[string]bool, len(files))
	errorsFound := make([]string, 0)
	for _, file := range files {
		if changed[file.Filename] {
			errorsFound = append(errorsFound, "duplicate changed path: "+file.Filename)
		}
		changed[file.Filename] = true
	}
	allowed := stringSet(selected.AllowedChangedPaths)
	for path := range changed {
		if !allowed[path] {
			errorsFound = append(errorsFound, "unexpected changed path: "+path)
		}
	}
	for _, path := range selected.RequiredChangedPaths {
		if !changed[path] {
			errorsFound = append(errorsFound, "required changed path absent: "+path)
		}
	}
	for _, path := range manifest.FrozenPaths {
		if changed[path] {
			errorsFound = append(errorsFound, "candidate modified frozen path: "+path)
		}
	}
	sort.Strings(errorsFound)
	validation := "passed"
	conclusion := "success"
	if len(errorsFound) != 0 {
		validation, conclusion = "failed", "failure"
	}
	summary := map[string]any{
		"schema_version": "1.0", "kind": "governance", "case_id": selected.CaseID,
		"candidate_sha": candidateSHA, "protected_base_sha": baseSHA,
		"source_repository": sourceRepo, "source_ref": sourceRef,
		"producer_run_id": requiredInt64("RUN_ID"), "producer_run_attempt": requiredInt("RUN_ATTEMPT"),
		"producer_workflow_path": ".github/workflows/stage-b-governance.yml",
		"protected_workflow_sha": workflowSHA, "validation": validation, "errors": errorsFound,
	}
	summaryBytes, _ := json.Marshal(summary)
	payload := map[string]any{
		"name": "stage-b-governance-record", "head_sha": candidateSHA, "status": "completed", "conclusion": conclusion,
		"external_id": fmt.Sprintf("stage-b:governance:v1:%s:%d:%d", candidateSHA, requiredInt64("RUN_ID"), requiredInt("RUN_ATTEMPT")),
		"output":      map[string]any{"title": "Stage B protected governance " + validation, "summary": string(summaryBytes)},
	}
	var created model.CheckRun
	createReceipt, err := client.Post(context.Background(), fmt.Sprintf("/repos/%s/check-runs", repository), payload, &created)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(map[string]any{"schema_version": "1.0", "files_receipt": filesReceipt, "created_check_receipt": createReceipt, "summary": summary})
	fmt.Println(string(encoded))
	if len(errorsFound) != 0 {
		return fmt.Errorf("governance rejected candidate metadata")
	}
	return nil
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
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
