package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUsersReflectWorkloadAndExcludeObservationTraffic(t *testing.T) {
	server := httptest.NewServer(newHandler("primary", 1))
	defer server.Close()
	readUsers := func() map[string]any {
		t.Helper()
		response, err := http.Get(server.URL + "/users")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var users map[string]any
		if err := json.NewDecoder(response.Body).Decode(&users); err != nil {
			t.Fatal(err)
		}
		return users
	}
	if users := readUsers(); users["active_users"] != float64(0) || users["total_users_served"] != float64(0) {
		t.Fatalf("initial users: %v", users)
	}
	done := make(chan error, 1)
	go func() {
		response, err := http.Get(server.URL + "/workload?duration_ms=500")
		if err == nil {
			_, err = io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
		done <- err
	}()
	active := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if readUsers()["active_users"] == float64(1) {
			active = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !active {
		t.Fatal("in-flight visit was not visible")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if users := readUsers(); users["active_users"] != float64(0) || users["total_users_served"] != float64(1) {
		t.Fatalf("completed users: %v", users)
	}
	response, err := http.Get(server.URL + "/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var stats struct {
		RequestCount uint64 `json:"request_count"`
	}
	if err := json.NewDecoder(response.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	if stats.RequestCount != 1 {
		t.Fatalf("user observations inflated request count: %d", stats.RequestCount)
	}
}
