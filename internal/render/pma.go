package render

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
)

// NewBlowfishSecret gera o segredo de sessão. Exatamente 32 bytes: o
// phpMyAdmin exibe um aviso permanente e recusa cookies com outro tamanho.
//
// 24 bytes aleatórios em base64 sem padding dão exatamente 32 caracteres, e o
// alfabeto URL-safe (A-Z a-z 0-9 - _) não contém aspa nem contrabarra, então o
// segredo nunca quebra a string do config.inc.php. Sortear caracteres de um
// alfabeto de 62 com `%` introduziria viés de módulo sem ganho nenhum.
func NewBlowfishSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("render: sortear blowfish_secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// PhpMyAdminConfig devolve o config.inc.php. Determinístico: mesma entrada,
// mesmos bytes — o segredo é parâmetro justamente para não ser sorteado a cada
// Reconcile, o que derrubaria a sessão aberta do usuário.
//
// root sem senha é o padrão com que o HyPHP inicializa o MySQL (spec §11: nada
// de credencial inventada); sem AllowNoPassword o phpMyAdmin recusa o login e
// não há como entrar. Com auth_type 'config' o usuário cai direto na lista de
// bancos, que é o ponto de abrir a ferramenta a partir da UI.
//
// tmpDir é parâmetro (o plano previa só porta e segredo) porque sem TempDir
// gravável o phpMyAdmin não consegue cachear os templates Twig: a ferramenta
// funciona, porém lenta e com aviso em toda página. O diretório é do HyPHP,
// não da instalação do phpMyAdmin — a pasta baixada é descartada e recriada a
// cada atualização de versão.
func PhpMyAdminConfig(mysqlPort int, secret, tmpDir string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "<?php\n")
	fmt.Fprintf(&b, "// Gerado pelo HyPHP — não editar à mão.\n")
	fmt.Fprintf(&b, "// Fonte: internal/render/pma.go — reescrito a cada Reconcile.\n\n")

	fmt.Fprintf(&b, "$cfg['blowfish_secret'] = %s;\n\n", phpString(secret))

	fmt.Fprintf(&b, "$i = 1;\n")
	fmt.Fprintf(&b, "$cfg['Servers'][$i]['host'] = '127.0.0.1';\n")
	// A porta vai como string porque é assim que o phpMyAdmin documenta e
	// compara o valor; um int aqui aparece em mensagens de erro como vazio.
	fmt.Fprintf(&b, "$cfg['Servers'][$i]['port'] = '%d';\n", mysqlPort)
	fmt.Fprintf(&b, "$cfg['Servers'][$i]['socket'] = '';\n")
	fmt.Fprintf(&b, "$cfg['Servers'][$i]['auth_type'] = 'config';\n")
	fmt.Fprintf(&b, "$cfg['Servers'][$i]['user'] = 'root';\n")
	fmt.Fprintf(&b, "$cfg['Servers'][$i]['password'] = '';\n")
	fmt.Fprintf(&b, "$cfg['Servers'][$i]['AllowNoPassword'] = true;\n")
	fmt.Fprintf(&b, "$cfg['Servers'][$i]['compress'] = false;\n")
	fmt.Fprintf(&b, "$cfg['ServerDefault'] = 1;\n\n")

	fmt.Fprintf(&b, "$cfg['TempDir'] = %s;\n", phpString(slashDir(tmpDir)))
	// Upload e export ficam desligados: apontá-los para uma pasta do HyPHP faria
	// o phpMyAdmin oferecer arquivos do ambiente na interface. O navegador já
	// resolve os dois casos por envio e download direto.
	fmt.Fprintf(&b, "$cfg['UploadDir'] = '';\n")
	fmt.Fprintf(&b, "$cfg['SaveDir'] = '';\n\n")

	// No Windows a checagem de permissão do config.inc.php sempre acusa arquivo
	// "gravável por todos" e imprime um aviso que o usuário não tem como
	// resolver — o arquivo é gerado e reescrito pelo próprio HyPHP.
	fmt.Fprintf(&b, "$cfg['CheckConfigurationPermissions'] = false;\n")
	// Ambiente de desenvolvimento offline: consultar o servidor de versões a
	// cada abertura só adiciona latência e uma requisição externa silenciosa.
	fmt.Fprintf(&b, "$cfg['VersionCheck'] = false;\n")
	fmt.Fprintf(&b, "$cfg['SendErrorReports'] = 'never';\n")

	return b.Bytes()
}

// phpString envolve um valor em aspas simples do PHP. Só contrabarra e aspa
// simples têm significado nesse tipo de literal; qualquer outra sequência entra
// crua, inclusive UTF-8 de caminho com acento.
func phpString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	return "'" + s + "'"
}
