// Comparação publicada no site. Cada bloco de concorrente tem fonte oficial e
// data de leitura, que o painel mostra.
//
// Regras (web/README.md, Comparação): só fonte primária; sem fonte oficial, o
// bloco não afirma nada (`suporte: null` + `nota`); linha com 2 ou mais blocos
// sem fonte sai. As frases do HyPHP têm prova no código do app (web/README.md,
// Fatos e provas).
//
// Cada texto tem a tradução em `en`, com o mesmo sentido: traduzir não muda o
// que a fonte diz. Sem `en`, vale o português.
//
// Script clássico, não módulo: o site precisa funcionar aberto do disco, e o
// navegador bloqueia módulos ES em file://.
window.HYPHP_COMPARACAO = {
  ferramentas: [
    { id: 'hyphp', nome: 'HyPHP' },
    { id: 'laragon', nome: 'Laragon', versao: '8.7.0', versaoCurta: '8.7.0', fonteVersao: 'https://github.com/leokhoa/laragon/releases/tag/8.7.0' },
    { id: 'xampp', nome: 'XAMPP', versao: '8.2.12 (Windows)', versaoCurta: '8.2.12', fonteVersao: 'https://www.apachefriends.org/download.html' },
    { id: 'wampserver', nome: 'WampServer', versao: '3.4.0 · atualização 3.4.2', en: { versao: '3.4.0 · update 3.4.2' }, versaoCurta: '3.4.2', fonteVersao: 'https://wampserver.aviatechno.net/' },
    { id: 'ddev', nome: 'DDEV', nomeLongo: 'Docker (Desktop/DDEV)', versao: 'DDEV 1.25.4', versaoCurta: '1.25.4', fonteVersao: 'https://github.com/ddev/ddev/releases/tag/v1.25.4' },
  ],
  linhas: [
    {
      rotulo: 'PHP por projeto',
      criterio: 'Versão de PHP por projeto',
      en: { rotulo: 'PHP per project', criterio: 'PHP version per project' },
      blocos: {
        hyphp: { suporte: 'sim', texto: 'Declarada no hyphp.yaml' , en: { texto: 'Declared in hyphp.yaml' } },
        laragon: { suporte: 'parcial', texto: 'Troca de versão pelo Profile', fonte: 'https://laragon.org/docs/profile', em: '2026-09-26' , en: { texto: 'Version switched via Profile' } },
        xampp: { suporte: 'nao', texto: 'Um PHP por pacote', fonte: 'https://www.apachefriends.org/download.html', em: '2026-09-26' , en: { texto: 'One PHP per package' } },
        wampserver: { suporte: 'sim', texto: 'PHP por VirtualHost (FCGI)', fonte: 'https://wampserver.aviatechno.net/?lang=en&changelog=afficher', em: '2026-09-26' , en: { texto: 'PHP per VirtualHost (FCGI)' } },
        ddev: { suporte: 'sim', texto: 'php_version no config.yaml', fonte: 'https://docs.ddev.com/en/stable/users/configuration/config/', em: '2026-09-26' , en: { texto: 'php_version in config.yaml' } },
      },
    },
    {
      rotulo: 'Vários PHP ao mesmo tempo',
      criterio: 'Várias versões de PHP ao mesmo tempo',
      en: { rotulo: 'Several PHPs at once', criterio: 'Several PHP versions at the same time' },
      blocos: {
        hyphp: { suporte: 'sim', texto: 'Um pool por versão' , en: { texto: 'One pool per version' } },
        laragon: { suporte: null, nota: 'As docs oficiais (laragon.org/docs/multi-version e /docs/profile) descrevem trocar a versão pelo menu ou por Profile; não dizem se duas versões servem ao mesmo tempo. Lidas em 26/09/2026.', en: { nota: 'The official docs (laragon.org/docs/multi-version and /docs/profile) describe switching the version from the menu or with a Profile; they do not say whether two versions serve at the same time. Read on Sep 26, 2026.' } },
        xampp: { suporte: 'nao', texto: 'Um PHP por pacote', fonte: 'https://www.apachefriends.org/download.html', em: '2026-09-26' , en: { texto: 'One PHP per package' } },
        wampserver: { suporte: 'sim', texto: 'VirtualHosts com PHPs distintos', fonte: 'https://wampserver.aviatechno.net/?lang=en&changelog=afficher', em: '2026-09-26' , en: { texto: 'VirtualHosts with different PHPs' } },
        ddev: { suporte: 'sim', texto: 'Vários projetos ao mesmo tempo', fonte: 'https://docs.ddev.com/en/stable/users/usage/faq/', em: '2026-09-26' , en: { texto: 'Several projects at the same time' } },
      },
    },
    {
      rotulo: 'Sem Docker nem WSL2',
      criterio: 'Roda sem Docker nem WSL2',
      en: { rotulo: 'No Docker or WSL2', criterio: 'Runs without Docker or WSL2' },
      blocos: {
        hyphp: { suporte: 'sim', texto: 'Processos nativos do Windows' , en: { texto: 'Native Windows processes' } },
        laragon: { suporte: 'sim', texto: 'App Windows, sem Docker', fonte: 'https://laragon.org/docs/faq', em: '2026-09-26' , en: { texto: 'Windows app, no Docker' } },
        xampp: { suporte: 'sim', texto: 'Instalador e serviços Windows', fonte: 'https://www.apachefriends.org/faq_windows.html', em: '2026-09-26' , en: { texto: 'Windows installer and services' } },
        wampserver: { suporte: 'sim', texto: 'Plataforma baseada em Windows', fonte: 'https://wampserver.aviatechno.net/', em: '2026-09-26' , en: { texto: 'Windows-based platform' } },
        ddev: { suporte: 'nao', texto: 'Exige um provedor Docker', fonte: 'https://docs.ddev.com/en/stable/users/install/docker-installation/', em: '2026-09-26' , en: { texto: 'Requires a Docker provider' } },
      },
    },
    {
      rotulo: 'Domínio local por projeto',
      criterio: 'Domínio local por projeto',
      en: { rotulo: 'Local domain per project', criterio: 'Local domain per project' },
      blocos: {
        hyphp: { suporte: 'sim', texto: '<nome>.test no hosts' , en: { texto: '<name>.test in hosts' } },
        laragon: { suporte: 'sim', texto: '<pasta>.test automático', fonte: 'https://laragon.org/docs/pretty-urls', em: '2026-09-26' , en: { texto: 'Automatic <folder>.test' } },
        xampp: { suporte: null, nota: 'O FAQ oficial (apachefriends.org/faq_windows.html) só mostra projeto como subpasta de http://localhost/; nada sobre domínio por projeto. Lido em 26/09/2026.', en: { nota: 'The official FAQ (apachefriends.org/faq_windows.html) only shows a project as a subfolder of http://localhost/; nothing about a domain per project. Read on Sep 26, 2026.' } },
        wampserver: { suporte: 'sim', texto: 'VirtualHost pelo menu', fonte: 'https://wampserver.aviatechno.net/?lang=en&changelog=afficher', em: '2026-09-27' , en: { texto: 'VirtualHost from the menu' } },
        ddev: { suporte: 'sim', texto: '<projeto>.ddev.site', fonte: 'https://docs.ddev.com/en/stable/users/configuration/config/', em: '2026-09-26' , en: { texto: '<project>.ddev.site' } },
      },
    },
    {
      rotulo: 'Apache ou nginx',
      criterio: 'Apache ou nginx, alternável',
      en: { rotulo: 'Apache or nginx', criterio: 'Apache or nginx, switchable' },
      blocos: {
        hyphp: { suporte: 'sim', texto: 'Troca a quente' , en: { texto: 'Hot switch' } },
        laragon: { suporte: 'sim', texto: 'Apache ou Nginx, via Profile', fonte: 'https://laragon.org/docs/profile', em: '2026-09-26' , en: { texto: 'Apache or Nginx, via Profile' } },
        xampp: { suporte: 'nao', texto: 'Sem nginx no pacote', fonte: 'https://www.apachefriends.org/download.html', em: '2026-09-26' , en: { texto: 'No nginx in the package' } },
        wampserver: { suporte: 'nao', texto: 'Só Apache 2.4', fonte: 'https://wampserver.aviatechno.net/', em: '2026-09-26' , en: { texto: 'Apache 2.4 only' } },
        ddev: { suporte: 'sim', texto: 'webserver_type por projeto', fonte: 'https://docs.ddev.com/en/stable/users/configuration/config/', em: '2026-09-26' , en: { texto: 'webserver_type per project' } },
      },
    },
  ],
};
