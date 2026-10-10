package stack

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"time"

	"hyphp/internal/i18n"
	"hyphp/internal/paths"
	"hyphp/internal/pkgmgr"
	"hyphp/internal/render"
	"hyphp/internal/runtime"
)

const (
	// cacertFetchTimeout limita o download do pacote (190 KB): um servidor que
	// não responde não pode deixar o download marcado como em voo para sempre.
	cacertFetchTimeout = 2 * time.Minute
	// cacertReconcileTimeout é o prazo do Reconcile que põe o bundle novo no
	// php.ini, o mesmo dos Reconciles do tray e do boot. Só conta depois que o
	// Reconcile toma o lock op, cuja espera não tem prazo.
	cacertReconcileTimeout = 60 * time.Second
)

// syncCACert mantém etc/ssl/cacert.pem para o PHP do Windows, que não traz
// certificados de CA: sem eles todo HTTPS de saída do PHP falha com
// "curl error 60". O bundle é o pacote da Mozilla do catálogo em uso seguido
// da CA local do mkcert, para o PHP também confiar nos sites .test em HTTPS.
//
// Devolve o caminho do bundle para o php.ini, ou "" com o aviso
// cacert-missing enquanto nenhum pacote foi baixado. O download roda fora do
// Reconcile e chama outro Reconcile ao terminar; um download que falhou é
// tentado de novo no Reconcile seguinte.
func (s *Stack) syncCACert(rts []runtime.Installed) (string, []Warning) {
	if len(runtime.ByKind(rts, runtime.PHP)) == 0 {
		return "", nil
	}
	dir := filepath.Join(paths.Var(), "cacert")
	src := ""
	if pkgs := s.d.Catalog().ByKind(pkgmgr.KindCACert); len(pkgs) > 0 {
		want := filepath.Join(dir, pkgs[0].ID+".pem")
		if _, err := os.Stat(want); err == nil {
			src = want
			removeOtherCACerts(dir, want)
		} else {
			s.fetchCACert(pkgs[0], want)
		}
	}
	if src == "" {
		// Versão nova a caminho: a anterior segue valendo até ela chegar.
		src = newestCACert(dir)
	}
	if src == "" {
		return "", []Warning{{Code: "cacert-missing", Message: i18n.T("warn.cacertMissing")}}
	}
	bundle, err := caBundle(src, s.mkcert().CARoot)
	if err == nil {
		_, err = render.WriteFiles(filepath.Join(paths.Etc(), "ssl"), map[string][]byte{"cacert.pem": bundle})
	}
	if err != nil {
		return "", []Warning{{Code: "cacert-missing", Message: i18n.T("warn.cacertWrite", err)}}
	}
	return filepath.Join(paths.Etc(), "ssl", "cacert.pem"), nil
}

// caBundle junta o pacote da Mozilla e a rootCA.pem do mkcert, quando ela
// existe. Ela só existe depois de "instalar certificado"; até lá o bundle é
// só o da Mozilla.
func caBundle(src, caroot string) ([]byte, error) {
	moz, err := os.ReadFile(src)
	if err != nil || caroot == "" {
		return moz, err
	}
	root, err := os.ReadFile(filepath.Join(caroot, "rootCA.pem"))
	if errors.Is(err, os.ErrNotExist) {
		return moz, nil
	}
	if err != nil {
		return nil, err
	}
	out := append(bytes.TrimRight(moz, "\n"), "\n\n"...)
	return append(out, root...), nil
}

// newestCACert devolve o pacote mais recente já baixado. Os ids são datados
// (cacert-AAAA-MM-DD): a ordem dos nomes é a das datas.
func newestCACert(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "cacert-*.pem"))
	if len(files) == 0 {
		return ""
	}
	slices.Sort(files)
	return files[len(files)-1]
}

// removeOtherCACerts apaga os pacotes anteriores depois que o do catálogo em
// uso chegou.
func removeOtherCACerts(dir, keep string) {
	files, _ := filepath.Glob(filepath.Join(dir, "cacert-*.pem"))
	for _, f := range files {
		if f != keep {
			_ = os.Remove(f)
		}
	}
}

// fetchCACert baixa o pacote em segundo plano, um de cada vez, e reconcilia
// ao terminar: o php.ini ganha as linhas de CA e os pools reiniciam.
func (s *Stack) fetchCACert(pkg pkgmgr.Package, dest string) {
	if !s.cacertFetching.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.cacertFetching.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), cacertFetchTimeout)
		err := s.d.Fetch(ctx, pkg, dest)
		cancel()
		if err != nil {
			s.d.Logger.Warn("stack: baixar o pacote de CAs", "id", pkg.ID, "err", err)
			return
		}
		s.d.Logger.Info("stack: pacote de CAs baixado", "id", pkg.ID)
		ctx, cancel = context.WithTimeout(context.Background(), cacertReconcileTimeout)
		defer cancel()
		if _, err := s.Reconcile(ctx); err != nil {
			s.d.Logger.Error("stack: reconciliar com o pacote de CAs", "err", err)
		}
	}()
}
