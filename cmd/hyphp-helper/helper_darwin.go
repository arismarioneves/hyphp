package main

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"hyphp/internal/netcfg"
	"hyphp/internal/paths"
)

const usage = "uso: hyphp-helper <resolver-write|paths-write|ca-trust|uninstall> [flags]"

// security é o binário do keychain do macOS.
const security = "/usr/bin/security"

// flushDNS faz o mDNSResponder esquecer respostas negativas guardadas antes
// da regra existir. É só melhor esforço: sem ele a regra vale do mesmo jeito,
// depois que o cache vence. Variável para os testes não mexerem no sistema.
var flushDNS = func() { _ = exec.Command("/usr/bin/killall", "-HUP", "mDNSResponder").Run() }

func main() {
	// No Mac o osascript devolve o stdout do helper ao app: não há --result.
	code, err := run(os.Args[1:])
	report("", code, err)
	os.Exit(code)
}

func run(args []string) (int, error) {
	if len(args) == 0 {
		return exitUsage, errors.New(usage)
	}
	switch args[0] {
	case "resolver-write":
		return resolverWrite(args[1:])
	case "paths-write":
		return pathsWrite(args[1:])
	case "ca-trust":
		return caTrust(args[1:])
	case "uninstall":
		return uninstall(args[1:])
	default:
		return exitUsage, fmt.Errorf("subcomando desconhecido %q; %s", args[0], usage)
	}
}

// resolverWrite grava a regra de DNS do .test. Só a porta vem de fora: o
// conteúdo é montado aqui.
func resolverWrite(argv []string) (int, error) {
	fs := newFlags("resolver-write")
	port := fs.Int("port", 0, "porta do resolvedor do HyPHP")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("resolver-write: %w", err)
	}
	if *port < 1024 || *port > 65535 {
		return exitUsage, fmt.Errorf("resolver-write: --port fora de 1024..65535: %d", *port)
	}
	if err := writeFileAtomic(netcfg.ResolverRulePath, []byte(netcfg.RenderResolverRule(*port))); err != nil {
		return exitIO, fmt.Errorf("resolver-write: %w", err)
	}
	flushDNS()
	return exitOK, nil
}

// pathsWrite grava o /etc/paths.d/hyphp com a pasta cli do usuário.
func pathsWrite(argv []string) (int, error) {
	fs := newFlags("paths-write")
	dir := fs.String("dir", "", "pasta cli do HyPHP")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("paths-write: %w", err)
	}
	if err := validCliDir(*dir); err != nil {
		return exitUsage, fmt.Errorf("paths-write: %w", err)
	}
	if err := writeFileAtomic(paths.PathsDFile, []byte(paths.PathsDContent(*dir))); err != nil {
		return exitIO, fmt.Errorf("paths-write: %w", err)
	}
	return exitOK, nil
}

// validCliDir aceita só a pasta cli que o app criou. Cada linha do paths.d vira
// uma entrada do PATH de todo shell de login da máquina, então quebra de linha
// injetaria outra; pasta de root seria uma pasta que o usuário não controla.
func validCliDir(dir string) error {
	switch {
	case dir == "" || !filepath.IsAbs(dir) || filepath.Clean(dir) != dir:
		return fmt.Errorf("--dir precisa ser absoluto e limpo: %q", dir)
	case strings.ContainsAny(dir, "\n\r"):
		return fmt.Errorf("--dir com quebra de linha: %q", dir)
	case filepath.Base(dir) != "cli":
		return fmt.Errorf("--dir precisa terminar em /cli: %q", dir)
	}
	st, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("--dir: %w", err)
	}
	if !st.IsDir() {
		return fmt.Errorf("--dir não é pasta: %q", dir)
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok && sys.Uid == 0 {
		return fmt.Errorf("--dir pertence a root: %q", dir)
	}
	return nil
}

// caTrust marca a CA do mkcert como confiável para todo o macOS.
func caTrust(argv []string) (int, error) {
	fs := newFlags("ca-trust")
	cert := fs.String("cert", "", "rootCA.pem do mkcert")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("ca-trust: %w", err)
	}
	if err := validCACert(*cert); err != nil {
		return exitUsage, fmt.Errorf("ca-trust: %w", err)
	}
	out, err := exec.Command(security, "add-trusted-cert", "-d", "-r", "trustRoot", "-k", netcfg.SystemKeychain, *cert).CombinedOutput()
	if err != nil {
		return exitIO, fmt.Errorf("security add-trusted-cert: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return exitOK, nil
}

// validCACert aceita só um rootCA.pem de CA: o helper roda como root, e
// confiar em qualquer arquivo indicado abriria o keychain do sistema para
// certificados de terceiros.
func validCACert(path string) error {
	if !filepath.IsAbs(path) || filepath.Base(path) != "rootCA.pem" {
		return fmt.Errorf("--cert precisa ser um rootCA.pem com caminho absoluto: %q", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("--cert: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		return fmt.Errorf("--cert não é um certificado PEM: %q", path)
	}
	c, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("--cert: %w", err)
	}
	if !c.IsCA {
		return fmt.Errorf("--cert não é de uma CA: %q", path)
	}
	return nil
}

// uninstall desfaz o que o HyPHP gravou no sistema. O que já não existe conta
// como sucesso: o botão pode ser usado depois de um desfazer parcial.
func uninstall(argv []string) (int, error) {
	fs := newFlags("uninstall")
	cert := fs.String("cert", "", "rootCA.pem a tirar do keychain do sistema (opcional)")
	if err := fs.Parse(argv); err != nil {
		return exitUsage, fmt.Errorf("uninstall: %w", err)
	}
	if *cert != "" {
		if err := validCACert(*cert); err != nil {
			return exitUsage, fmt.Errorf("uninstall: %w", err)
		}
	}
	// A regra de outro programa (Laravel Valet) fica: só a com a nossa marca sai.
	switch raw, err := os.ReadFile(netcfg.ResolverRulePath); {
	case err == nil:
		if ours, _ := netcfg.ParseResolverRule(string(raw)); ours {
			if err := os.Remove(netcfg.ResolverRulePath); err != nil {
				return exitIO, fmt.Errorf("uninstall: %w", err)
			}
			flushDNS()
		}
	case !errors.Is(err, os.ErrNotExist):
		return exitIO, fmt.Errorf("uninstall: %w", err)
	}
	if err := os.Remove(paths.PathsDFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return exitIO, fmt.Errorf("uninstall: %w", err)
	}
	if *cert != "" {
		return untrust(*cert)
	}
	return exitOK, nil
}

// untrust apaga a CA do keychain do sistema pelo SHA-1, e a confiança some
// junto. `remove-trusted-cert -d` fica de fora: ele espera interação mesmo
// como root e travou o spike de 2026-10-10.
func untrust(cert string) (int, error) {
	sha, err := netcfg.CertSHA1(cert)
	if err != nil {
		return exitUsage, fmt.Errorf("uninstall: %w", err)
	}
	in, err := netcfg.InSystemKeychain(sha)
	if err != nil {
		return exitIO, fmt.Errorf("uninstall: %w", err)
	}
	if !in {
		return exitOK, nil
	}
	out, err := exec.Command(security, "delete-certificate", "-Z", sha, netcfg.SystemKeychain).CombinedOutput()
	if err != nil {
		return exitIO, fmt.Errorf("security delete-certificate: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return exitOK, nil
}

// writeFileAtomic grava num temporário e renomeia: uma regra de DNS pela
// metade mandaria as consultas .test para lugar nenhum. O temporário fica na
// pasta acima da do destino (ex.: /etc/.test.hyphp-tmp), no mesmo sistema de
// arquivos para o rename seguir atômico: um resto dentro de /etc/resolver
// após uma queda viraria regra do domínio test.hyphp-tmp, e um em
// /etc/paths.d seria lido pelo path_helper. O chmod explícito tira a
// dependência do umask do processo do osascript: o mDNSResponder e o
// path_helper precisam ler o arquivo.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(filepath.Dir(path)), "."+filepath.Base(path)+".hyphp-tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o644); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
