package util

import "strings"

// NormalizeRegistry canonicalises a registry host so that equivalent spellings
// compare and look up equally. It strips an explicit :443 and folds the
// various Docker Hub hostnames to "docker.io".
func NormalizeRegistry(reg string) string {
	reg = strings.TrimSuffix(reg, ":443")
	if reg == "index.docker.io" || reg == "docker.io" || reg == "registry-1.docker.io" {
		return "docker.io"
	}
	return reg
}
