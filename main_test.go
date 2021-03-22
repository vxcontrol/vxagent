package main

import "testing"

func TestMain(t *testing.T) {
	if true != true {
		t.Errorf("Test error\n")
	}
}
