package tools

// IsManagedGeneratedConfigContent reports whether content begins with one of
// the exact whole-file ownership headers emitted by this project. Planning uses
// this pure form so ownership, digest, and private revision authority all come
// from one descriptor-anchored read.
func IsManagedGeneratedConfigContent(content []byte) bool {
	return hasGeneratedConfigHeader(content)
}

// IsLegacyGeneratedYaziThemeContent reports the one exact pre-header Yazi
// theme format eligible for migration, without re-reading the planned path.
func IsLegacyGeneratedYaziThemeContent(content []byte) bool {
	return hasLegacyGeneratedYaziThemeHeader(content)
}
