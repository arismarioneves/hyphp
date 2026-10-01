package update

// platformArtifact é o instalador desta plataforma no manifesto.
func platformArtifact(l Latest) *Artifact {
	return l.WindowsAMD64
}
