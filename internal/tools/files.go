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

// lexicallyOutside vérifie, sans toucher le système de fichiers hormis le
// dossier autorisé lui-même, si full est hors de filesDir de façon évidente
// (avant résolution des liens symboliques du chemin visé). Sans ce filtre,
// une simple stat ou un EvalSymlinks échouant faute d'exister révélerait,
// par un message différent, si un chemin situé hors du dossier existe ou
// non sur le disque : voir aussi resolveUpload et resolveSaveTo.
func lexicallyOutside(filesDir, full string) bool {
	clean := filepath.Clean(full)
	if withinFolder(filepath.Clean(filesDir), clean) {
		return false
	}
	if resolvedFolder, err := filepath.EvalSymlinks(filesDir); err == nil {
		if withinFolder(resolvedFolder, clean) {
			return false
		}
	}
	return true
}

// resolveUpload vérifie qu'un chemin de fichier à envoyer existe, reste
// dans filesDir une fois les liens symboliques résolus, et est un fichier
// régulier. Renvoie le chemin résolu à utiliser pour l'envoi.
func resolveUpload(filesDir, p string) (string, error) {
	full := resolveLocal(filesDir, p)
	if lexicallyOutside(filesDir, full) {
		return "", fmt.Errorf("chemin hors du dossier autorisé (%s) : %s", filesDir, p)
	}
	folder, err := filepath.EvalSymlinks(filesDir)
	if err != nil {
		return "", fmt.Errorf("dossier autorisé invalide (%s) : %v", filesDir, err)
	}
	resolved, err := filepath.EvalSymlinks(full)
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
	full := resolveLocal(filesDir, p)
	base := filepath.Base(full)
	if base == "" || base == "." || base == ".." {
		return "", fmt.Errorf("chemin hors du dossier autorisé (%s) : %s", filesDir, p)
	}
	parentFull := filepath.Dir(full)
	if lexicallyOutside(filesDir, parentFull) {
		return "", fmt.Errorf("chemin hors du dossier autorisé (%s) : %s", filesDir, p)
	}
	folder, err := filepath.EvalSymlinks(filesDir)
	if err != nil {
		return "", fmt.Errorf("dossier autorisé invalide (%s) : %v", filesDir, err)
	}
	parent, err := filepath.EvalSymlinks(parentFull)
	if err != nil {
		return "", fmt.Errorf("dossier parent introuvable : %s", filepath.Dir(p))
	}
	if !withinFolder(folder, parent) {
		return "", fmt.Errorf("chemin hors du dossier autorisé (%s) : %s", filesDir, p)
	}
	return filepath.Join(parent, base), nil
}
