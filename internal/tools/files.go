package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveLocal renvoie le chemin absolu correspondant à p : relatif à
// filesDir si p est relatif, tel quel si p est absolu.
func resolveLocal(filesDir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(filesDir, p)
}

// withinFolder vérifie que resolved (déjà résolu par filepath.EvalSymlinks)
// est le dossier folder (déjà résolu lui aussi) ou l'un de ses descendants.
func withinFolder(folder, resolved string) bool {
	rel, err := filepath.Rel(folder, resolved)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// resolveUpload vérifie qu'un chemin de fichier à envoyer existe, reste
// dans filesDir une fois les liens symboliques résolus, et est un fichier
// régulier. Renvoie le chemin résolu à utiliser pour l'envoi.
func resolveUpload(filesDir, p string) (string, error) {
	folder, err := filepath.EvalSymlinks(filesDir)
	if err != nil {
		return "", fmt.Errorf("dossier autorisé invalide (%s) : %v", filesDir, err)
	}
	resolved, err := filepath.EvalSymlinks(resolveLocal(filesDir, p))
	if err != nil {
		return "", fmt.Errorf("fichier introuvable ou invalide : %s", p)
	}
	if !withinFolder(folder, resolved) {
		return "", fmt.Errorf("chemin hors du dossier autorisé (%s) : %s", filesDir, p)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("fichier introuvable ou invalide : %s", p)
	}
	return resolved, nil
}

// resolveSaveTo vérifie que save_to reste dans filesDir : son dossier
// parent doit exister et résoudre dans le dossier autorisé, et son nom de
// base ne doit être ni vide, ni « . », ni « .. ». Renvoie le chemin cible
// (dossier parent résolu + nom de base), sans le créer.
func resolveSaveTo(filesDir, p string) (string, error) {
	folder, err := filepath.EvalSymlinks(filesDir)
	if err != nil {
		return "", fmt.Errorf("dossier autorisé invalide (%s) : %v", filesDir, err)
	}
	full := resolveLocal(filesDir, p)
	base := filepath.Base(full)
	if base == "" || base == "." || base == ".." {
		return "", fmt.Errorf("chemin hors du dossier autorisé (%s) : %s", filesDir, p)
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(full))
	if err != nil {
		return "", fmt.Errorf("dossier parent introuvable : %s", filepath.Dir(p))
	}
	if !withinFolder(folder, parent) {
		return "", fmt.Errorf("chemin hors du dossier autorisé (%s) : %s", filesDir, p)
	}
	return filepath.Join(parent, base), nil
}
