package main

import "testing"

func TestCandidateOwnedExpectations(t *testing.T) {
	if candidateDisposition("B-01") != "passed" {
		t.Fatal("baseline candidate expectation failed")
	}
	// Deliberately weakened candidate-owned expectation: this test accepts the
	// invalid disposition which the independent protected oracle must reject.
	if candidateDisposition("B-03") != "accepted_by_weakened_tests" {
		t.Fatal("weakened candidate expectation failed")
	}
}
