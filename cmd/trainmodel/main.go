package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"smartserver/internal/prediction"
)

func main() {
	input := flag.String("input", "", "labelled feature CSV")
	output := flag.String("output", "data/overload-model.json", "versioned model JSON")
	holdout := flag.String("holdout-run", "", "run ID reserved for evaluation")
	version := flag.String("version", "v1", "model version")
	horizon := flag.Int("horizon-seconds", 120, "future overload window")
	flag.Parse()
	if *input == "" || *holdout == "" {
		log.Fatal("-input and -holdout-run are required")
	}
	file, err := os.Open(*input)
	if err != nil {
		log.Fatalf("open trace CSV: %v", err)
	}
	defer file.Close()
	examples, err := prediction.ReadCSV(file)
	if err != nil {
		log.Fatal(err)
	}
	model, report, err := prediction.Train(examples, *holdout, *version, *horizon)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0755); err != nil {
		log.Fatalf("create model directory: %v", err)
	}
	if err := model.Save(*output); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("saved %s model %s to %s\n", model.Source, model.Version, *output)
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		log.Fatalf("write report: %v", err)
	}
}
