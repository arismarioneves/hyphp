package update

// platformArtifact é o pacote desta plataforma no manifesto; nil enquanto o
// hyphp-release não publicar darwin_arm64, e Check trata isso como "em dia".
func platformArtifact(l Latest) *Artifact {
	return l.DarwinARM64
}
