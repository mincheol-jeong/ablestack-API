package main

import "testing"

func TestResolveAPIPort(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", want: "18090"},
		{name: "custom", input: "28090", want: "28090"},
		{name: "trim and normalize", input: " 018090 ", want: "18090"},
		{name: "zero", input: "0", wantErr: true},
		{name: "too large", input: "65536", wantErr: true},
		{name: "not numeric", input: "http", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAPIPort(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("resolveAPIPort() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("resolveAPIPort() = %q, want %q", got, tt.want)
			}
		})
	}
}
