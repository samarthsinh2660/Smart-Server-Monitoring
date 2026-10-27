package main

import "testing"

func TestOperatorAuthentication(t *testing.T) {
	const token = "a-local-operator-secret"
	for _, header := range []string{"", token, "Bearer wrong", "Basic " + token, "Bearer " + token + " "} {
		if operatorAuthorized(token, header) {
			t.Fatalf("accepted invalid operator authorization %q", header)
		}
	}
	if !operatorAuthorized(token, "Bearer "+token) {
		t.Fatal("valid operator token rejected")
	}
	if operatorAuthorized("", "Bearer ") {
		t.Fatal("empty control token accepted")
	}
}
