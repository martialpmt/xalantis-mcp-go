package main

import "runtime/debug"

// version est injectée à la compilation par GoReleaser
// (-ldflags "-X main.version=vX.Y.Z").
var version = ""

// resolveVersion choisit la version annoncée : la valeur injectée, sinon la
// version du module (go install …@vX.Y.Z), sinon "dev".
func resolveVersion(injected string, info *debug.BuildInfo, ok bool) string {
	if injected != "" {
		return injected
	}
	if ok && info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

// currentVersion renvoie la version du binaire en cours d'exécution.
func currentVersion() string {
	info, ok := debug.ReadBuildInfo()
	return resolveVersion(version, info, ok)
}
