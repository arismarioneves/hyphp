package update

// platformArtifact é o dmg desta plataforma no manifesto. As releases até a
// 3.0.1 não têm darwin_arm64, e Check trata a falta como "em dia".
func platformArtifact(l Latest) *Artifact {
	return l.DarwinARM64
}

// saveManifest não guarda nada: a troca do bundle não passa pelo helper.
func saveManifest(string, []byte, []byte) error { return nil }
