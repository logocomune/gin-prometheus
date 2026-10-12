package main

import (
	"testing"
	"time"
)

func TestDurationEnv(t *testing.T) {
	t.Setenv("TEST_DURATION", "")
	if d, err := durationEnv("TEST_DURATION", time.Second); err != nil || d != time.Second {
		t.Fatalf("default: got %v, %v", d, err)
	}
	t.Setenv("TEST_DURATION", "0")
	if d, err := durationEnv("TEST_DURATION", time.Second); err != nil || d != 0 {
		t.Fatalf("zero: got %v, %v", d, err)
	}
	for _, bad := range []string{"abc", "-1s", "10"} {
		t.Setenv("TEST_DURATION", bad)
		if _, err := durationEnv("TEST_DURATION", time.Second); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestIntEnv(t *testing.T) {
	t.Setenv("TEST_INT", "")
	if n, err := intEnv("TEST_INT", 5); err != nil || n != 5 {
		t.Fatalf("default: got %v, %v", n, err)
	}
	t.Setenv("TEST_INT", "42")
	if n, err := intEnv("TEST_INT", 5); err != nil || n != 42 {
		t.Fatalf("set: got %v, %v", n, err)
	}
	for _, bad := range []string{"x", "-1", "1.5"} {
		t.Setenv("TEST_INT", bad)
		if _, err := intEnv("TEST_INT", 5); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
