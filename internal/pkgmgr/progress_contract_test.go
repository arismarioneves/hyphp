package pkgmgr

import (
	"reflect"
	"testing"
)

// Progress viaja para a UI pelo evento "download:progress", e eventos não
// aparecem em assinatura de método bindado — por isso o gerador de bindings do
// Wails não emite este modelo, e o tipo TypeScript equivalente é mantido à mão
// em frontend/src/lib/types.ts.
//
// Contrato cruzado sem compilador dos dois lados: mudar um campo aqui quebra a
// barra de progresso da tela Runtimes silenciosamente, em runtime. Este teste
// falha primeiro e lembra de atualizar o espelho.
func TestProgressContratoComOFrontend(t *testing.T) {
	want := map[string]string{
		"PackageID": "packageId",
		"Done":      "done",
		"Total":     "total",
		"Phase":     "phase",
		"Error":     "error",
		"Message":   "message",
	}

	rt := reflect.TypeOf(Progress{})
	if rt.NumField() != len(want) {
		t.Fatalf("Progress tem %d campos, o espelho TS tem %d — atualize frontend/src/lib/types.ts", rt.NumField(), len(want))
	}
	for i := range rt.NumField() {
		f := rt.Field(i)
		tag, ok := want[f.Name]
		if !ok {
			t.Errorf("campo %s não existe no espelho TS; atualize frontend/src/lib/types.ts", f.Name)
			continue
		}
		if got := f.Tag.Get("json"); got != tag {
			t.Errorf("%s: tag json = %q, o espelho TS espera %q", f.Name, got, tag)
		}
	}
}

// As fases são uma união literal no TypeScript (ProgressPhase). Emitir uma fase
// fora da lista faria a UI cair no default silenciosamente.
func TestFasesDeProgressoConhecidas(t *testing.T) {
	want := map[string]bool{"download": true, "verify": true, "extract": true, "done": true, "error": true, "canceled": true}
	for _, p := range []string{PhaseDownload, PhaseVerify, PhaseExtract, PhaseDone, PhaseError, PhaseCanceled} {
		if !want[p] {
			t.Errorf("fase %q não está em ProgressPhase (frontend/src/lib/types.ts)", p)
		}
	}
}
