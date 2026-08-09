package db

import "testing"

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig("")
	if config.GraphName != "openchain" || config.DSN == "" { t.Fatalf("config = %#v", config) }
}
