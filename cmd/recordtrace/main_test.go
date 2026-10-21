package main

import (
	"os"
	"path/filepath"
	"testing"

	"smartserver/internal/prediction"
)

func TestAppendRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traces.csv")
	examples := []prediction.Example{{Source: "recorded", RunID: "steady-1", Label: 0}}
	if err := appendRun(path, "steady-1", examples); err != nil {
		t.Fatal(err)
	}
	if err := appendRun(path, "steady-1", examples); err == nil {
		t.Fatal("duplicate run accepted")
	}
	examples[0].RunID = "ramp-1"
	if err := appendRun(path, "ramp-1", examples); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := prediction.ReadCSV(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].RunID != "steady-1" || got[1].RunID != "ramp-1" {
		t.Fatalf("saved runs: %+v", got)
	}
}
