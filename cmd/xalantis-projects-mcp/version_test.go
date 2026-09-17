package main

import (
	"runtime/debug"
	"testing"
)

func TestResolveVersion(t *testing.T) {
	info := func(v string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: v}}
	}
	cases := []struct {
		name     string
		injected string
		info     *debug.BuildInfo
		ok       bool
		want     string
	}{
		{"valeur injectée prioritaire", "v0.1.0", info("v9.9.9"), true, "v0.1.0"},
		{"version du module (go install)", "", info("v0.2.0"), true, "v0.2.0"},
		{"module sans version ((devel))", "", info("(devel)"), true, "dev"},
		{"version de module vide", "", info(""), true, "dev"},
		{"pas d'informations de build", "", nil, false, "dev"},
		{"informations nulles", "", nil, true, "dev"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resolveVersion(c.injected, c.info, c.ok); got != c.want {
				t.Errorf("resolveVersion(%q, …) = %q, attendu %q", c.injected, got, c.want)
			}
		})
	}
}
