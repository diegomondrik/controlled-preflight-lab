package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

const maxProtocolBytes = 4096

type request struct {
	SchemaVersion string `json:"schema_version"`
	CaseID        string `json:"case_id"`
	Operation     string `json:"operation"`
}

type response struct {
	SchemaVersion string `json:"schema_version"`
	Disposition   string `json:"disposition"`
}

func main() {
	if len(os.Args) != 3 {
		fail("usage: protected-oracle CANDIDATE_BINARY CASE_ID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[1])
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	stdin, err := command.StdinPipe()
	if err != nil {
		fail(err.Error())
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		fail(err.Error())
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		fail(err.Error())
	}
	if err := command.Start(); err != nil {
		fail("candidate start failed: " + err.Error())
	}
	encoded, _ := json.Marshal(request{SchemaVersion: "1.0", CaseID: os.Args[2], Operation: "evaluate_evidence_disposition"})
	if _, err := stdin.Write(append(encoded, '\n')); err != nil {
		fail("request write failed: " + err.Error())
	}
	stdin.Close()
	line, err := readBoundedLine(stdout)
	if err != nil {
		fail("candidate response invalid: " + err.Error())
	}
	stderrLine, stderrErr := readOptionalBounded(stderr)
	if stderrErr != nil {
		fail("candidate stderr exceeded boundary")
	}
	if err := command.Wait(); err != nil {
		fail("candidate exited non-zero")
	}
	if ctx.Err() != nil {
		fail("candidate timed out")
	}
	if len(stderrLine) != 0 {
		fail("candidate emitted unexpected stderr")
	}
	var observed response
	if err := json.Unmarshal(line, &observed); err != nil {
		fail("response is not strict JSON: " + err.Error())
	}
	if observed.SchemaVersion != "1.0" {
		fail("protocol version mismatch")
	}
	allowed := map[string]bool{"passed": true, "failed": true, "not_applicable": true, "unverified": true}
	if !allowed[observed.Disposition] {
		printJSON(map[string]any{"schema_version": "1.0", "classification": "oracle_rejected", "reason": "disposition_outside_allowlist", "observed_disposition": observed.Disposition})
		os.Exit(1)
	}
	printJSON(map[string]any{"schema_version": "1.0", "classification": "oracle_passed", "observed_disposition": observed.Disposition})
}

func readBoundedLine(reader io.Reader) ([]byte, error) {
	buffered := bufio.NewReader(io.LimitReader(reader, maxProtocolBytes+2))
	line, err := buffered.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	if len(line) > maxProtocolBytes || len(line) < 2 {
		return nil, errors.New("response length outside boundary")
	}
	if _, extraErr := buffered.ReadByte(); !errors.Is(extraErr, io.EOF) {
		if extraErr != nil {
			return nil, extraErr
		}
		return nil, errors.New("multiple response records")
	}
	return line[:len(line)-1], nil
}

func readOptionalBounded(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxProtocolBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxProtocolBytes {
		return nil, errors.New("bounded stream exceeded")
	}
	return data, nil
}

func fail(reason string) {
	printJSON(map[string]any{"schema_version": "1.0", "classification": "inconclusive", "reason": reason})
	os.Exit(1)
}
func printJSON(value any) { encoded, _ := json.Marshal(value); fmt.Println(string(encoded)) }
