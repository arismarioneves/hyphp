package update

// platformArtifact é o instalador desta plataforma no manifesto.
func platformArtifact(l Latest) *Artifact {
	return l.WindowsAMD64
}

// saveManifest não guarda nada: o instalador roda sem o helper.
func saveManifest(string, []byte, []byte) error { return nil }
