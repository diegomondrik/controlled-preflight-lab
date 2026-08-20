package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

type result struct {
	SchemaVersion  string `json:"schema_version"`
	Phase          string `json:"phase"`
	Classification string `json:"classification"`
	Entry          string `json:"entry"`
	Mode           string `json:"mode"`
	LinkTarget     string `json:"link_target"`
	SentinelSHA256 string `json:"sentinel_sha256,omitempty"`
	ProbeError     string `json:"probe_error,omitempty"`
}

func main() {
	if len(os.Args) != 6 {
		fail("usage: g02-guard observe|probe ROOT RELATIVE_ENTRY EXPECTED_TARGET SENTINEL_OR_DASH")
	}
	phase, root, relative, expectedTarget, sentinel := os.Args[1], os.Args[2], os.Args[3], os.Args[4], os.Args[5]
	if phase != "observe" && phase != "probe" {
		fail("phase must be observe or probe")
	}
	if filepath.IsAbs(relative) || filepath.Clean(relative) != relative || relative == "." || relative == ".." {
		fail("entry must be one clean relative path")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		fail(err.Error())
	}
	entry := filepath.Join(rootAbs, relative)
	info, err := os.Lstat(entry)
	if err != nil {
		fail("lstat failed: " + err.Error())
	}
	if info.Mode()&os.ModeSymlink == 0 {
		fail("entry is not a symbolic link")
	}
	target, err := os.Readlink(entry)
	if err != nil {
		fail("readlink failed: " + err.Error())
	}
	if target != expectedTarget {
		fail("link target mismatch")
	}
	output := result{
		SchemaVersion:  "1.0",
		Phase:          phase,
		Classification: "passed",
		Entry:          relative,
		Mode:           "120000",
		LinkTarget:     target,
	}
	if phase == "observe" {
		if sentinel == "-" {
			fail("observe phase requires sentinel path")
		}
		digest, digestErr := hashRegularFile(sentinel)
		if digestErr != nil {
			fail("sentinel hash failed: " + digestErr.Error())
		}
		output.SentinelSHA256 = digest
	} else {
		if sentinel != "-" {
			fail("probe phase must not receive a host sentinel path")
		}
		file, openErr := os.Open(entry)
		if openErr == nil {
			file.Close()
			fail("contained hostile link unexpectedly dereferenced")
		}
		if !errors.Is(openErr, syscall.ENOENT) && !errors.Is(openErr, syscall.EACCES) && !errors.Is(openErr, syscall.EPERM) {
			fail("contained probe returned an unclassified error: " + openErr.Error())
		}
		output.ProbeError = openErr.Error()
	}
	encoded, _ := json.Marshal(output)
	fmt.Println(string(encoded))
}

func hashRegularFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("sentinel is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(file, 1<<20)); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fail(message string) {
	encoded, _ := json.Marshal(map[string]string{"schema_version": "1.0", "classification": "inconclusive", "reason": message})
	fmt.Fprintln(os.Stderr, string(encoded))
	os.Exit(1)
}
