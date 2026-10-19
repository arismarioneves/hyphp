package update

// platformArtifact é o pacote do Linux no manifesto. Sem linux_amd64, Check
// trata como "em dia".
func platformArtifact(l Latest) *Artifact {
	return l.LinuxAMD64
}
