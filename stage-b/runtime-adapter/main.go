package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type request struct {
	SchemaVersion string `json:"schema_version"`
	CaseID        string `json:"case_id"`
	Operation     string `json:"operation"`
}

func main() {
	line, err := bufio.NewReaderSize(os.Stdin, 4096).ReadBytes('\n')
	if err != nil {
		os.Exit(2)
	}
	var input request
	if json.Unmarshal(line, &input) != nil || input.SchemaVersion != "1.0" || input.Operation != "evaluate_evidence_disposition" {
		os.Exit(2)
	}
	disposition := candidateDisposition(input.CaseID)
	encoded, _ := json.Marshal(map[string]string{"schema_version": "1.0", "disposition": disposition})
	fmt.Println(string(encoded))
}

func candidateDisposition(caseID string) string {
	if caseID == "B-03" {
		// Deliberate defect: candidate-controlled tests accept an invalid value.
		return "accepted_by_weakened_tests"
	}
	if caseID == "B-10" && !containmentProbesPass() {
		return "containment_probe_failed"
	}
	return "passed"
}

func containmentProbesPass() bool {
	for _, item := range os.Environ() {
		name := strings.SplitN(item, "=", 2)[0]
		if strings.Contains(name, "TOKEN") || strings.Contains(name, "SECRET") || strings.HasPrefix(name, "ACTIONS_") || name == "GITHUB_ENV" || name == "GITHUB_OUTPUT" || name == "GITHUB_PATH" {
			return false
		}
	}
	if _, err := os.Stat("/var/run/docker.sock"); err == nil {
		return false
	}
	if _, err := os.Stat("/dev/sda"); err == nil {
		return false
	}
	if connection, err := net.DialTimeout("tcp", "github.com:443", time.Second); err == nil {
		connection.Close()
		return false
	}
	if err := os.WriteFile("/src/stage-b-forbidden-source-write", []byte("x"), 0600); err == nil {
		return false
	}
	if err := os.WriteFile("/stage-b-forbidden-persistence", []byte("x"), 0600); err == nil {
		return false
	}
	return true
}
