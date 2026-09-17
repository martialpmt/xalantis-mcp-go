package main

import (
	"strings"
	"testing"
)

func TestParseReadOnly(t *testing.T) {
	cases := []struct {
		in      string
		want    bool
		wantErr bool
	}{
		{"", false, false},
		{"  ", false, false},
		{"1", true, false},
		{"true", true, false},
		{"TRUE", true, false},
		{" 1 ", true, false},
		{"0", false, false},
		{"false", false, false},
		{"oui", false, true},
	}
	for _, c := range cases {
		got, err := parseReadOnly(c.in)
		if got != c.want || (err != nil) != c.wantErr {
			t.Errorf("parseReadOnly(%q) = %v, %v ; attendu %v, erreur %v", c.in, got, err, c.want, c.wantErr)
		}
		if err != nil && !strings.Contains(err.Error(), "XALANTIS_READ_ONLY") {
			t.Errorf("parseReadOnly(%q) : l'erreur doit nommer la variable : %v", c.in, err)
		}
	}
}

func TestServerInstructions(t *testing.T) {
	rw := serverInstructions(false)
	if !strings.Contains(rw, "xalantis_read_operation") || !strings.Contains(rw, "xalantis_call_operation") {
		t.Errorf("mode normal : %s", rw)
	}
	ro := serverInstructions(true)
	if strings.Contains(ro, "xalantis_call_operation") || !strings.Contains(ro, "XALANTIS_READ_ONLY") {
		t.Errorf("lecture seule : %s", ro)
	}
}
