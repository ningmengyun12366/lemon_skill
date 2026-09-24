package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func readInputFile(path string) (retrieveInput, error) {
	if strings.TrimSpace(path) == "" {
		return retrieveInput{}, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return retrieveInput{}, fmt.Errorf("read input JSON %s: %w", path, err)
	}
	defer file.Close()
	var input retrieveInput
	if err := json.NewDecoder(file).Decode(&input); err != nil {
		return retrieveInput{}, fmt.Errorf("parse input JSON %s: %w", path, err)
	}
	return input, nil
}

func resolveRetrieveInput(query string, apiKey string, input retrieveInput) retrieveInput {
	result := input
	if strings.TrimSpace(query) != "" {
		result.Query = query
	}
	if strings.TrimSpace(apiKey) != "" {
		result.APIKey = apiKey
	}
	return result
}
