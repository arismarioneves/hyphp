// HyPHP — site. Script clássico sem dependências: abre direto do disco.
(function () {
  'use strict';

  var reduzir = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

  // ---------- idioma ----------
  //
  // Inglês é o padrão. A escolha fica no localStorage; ?lang=pt|en na URL
  // passa por cima, para mandar o link já no idioma certo.

  var TEXTOS = window.HYPHP_TEXTOS;
  var CHAVE_IDIOMA = 'hyphp-idioma';
  var idioma = idiomaInicial();

  function idiomaInicial() {
    var q = new URLSearchParams(location.search).get('lang');
    if (q && TEXTOS[q]) return q;
    // Alguns navegadores negam o localStorage em file:// ou em modo restrito
    // e lançam SecurityError; aí vale o padrão.
    try {
      var salvo = localStorage.getItem(CHAVE_IDIOMA);
      if (salvo && TEXTOS[salvo]) return salvo;
    } catch (e) {}
    return 'en';
  }

  function t(chave, vars) {
    var s = TEXTOS[idioma][chave];
    if (s === undefined || s === null) s = TEXTOS.en[chave];
    if (!vars) return s;
    return s.replace(/\{(\w+)\}/g, function (m, k) { return k in vars ? vars[k] : m; });
  }

  // Campo traduzido dos dados da comparação: `obj.en.campo` em inglês, senão
  // o português de `obj.campo`.
  function tx(obj, campo) {
    return idioma === 'en' && obj.en && obj.en[campo] ? obj.en[campo] : obj[campo];
  }

  // ---------- utilidades ----------

  function el(tag, attrs, filhos) {
    var n = document.createElement(tag);
    if (attrs) {
      Object.keys(attrs).forEach(function (k) {
        if (k === 'texto') n.textContent = attrs[k];
        else if (k === 'classe') n.className = attrs[k];
        else n.setAttribute(k, attrs[k]);
      });
    }
    (filhos || []).forEach(function (f) {
      n.append(f);
    });
    return n;
  }

  // AAAA-MM-DD → "28/09/2026" ou "Sep 28, 2026", sem passar por Date, que
  // aplicaria fuso.
  function data(iso) {
    var p = String(iso).split('-');
    if (p.length !== 3) return String(iso);
    var meses = TEXTOS[idioma].meses;
    return meses ? meses[Number(p[1]) - 1] + ' ' + Number(p[2]) + ', ' + p[0] : p[2] + '/' + p[1] + '/' + p[0];
  }

  function tamanho(bytes) {
    var loc = idioma === 'en' ? 'en-US' : 'pt-BR';
    return (bytes / 1048576).toLocaleString(loc, { maximumFractionDigits: 1, minimumFractionDigits: 1 }) + ' MB';
  }

  // O caminho vem do releases.js; mesmo gerado pelo hyphp-release, só nome de
  // arquivo comum vira link.
  function caminhoSeguro(p) {
    return typeof p === 'string' && /^[\w.\-/]+$/.test(p) && p.indexOf('..') === -1 && p.charAt(0) !== '/';
  }

  // Textos fixos do HTML: data-t (texto), data-t-html (as strings "…Html" do
  // dicionário, que levam marcação) e data-t-attr="atributo:chave;…".
  function traduzirPagina() {
    document.documentElement.lang = idioma === 'en' ? 'en' : 'pt-BR';
    document.title = t('titulo');
    document.querySelector('meta[name="description"]').setAttribute('content', t('descricao'));
    document.querySelectorAll('[data-t]').forEach(function (n) { n.textContent = t(n.dataset.t); });
    document.querySelectorAll('[data-t-html]').forEach(function (n) { n.innerHTML = t(n.dataset.tHtml); });
    document.querySelectorAll('[data-t-attr]').forEach(function (n) {
      n.dataset.tAttr.split(';').forEach(function (par) {
        var kv = par.split(':');
        n.setAttribute(kv[0], t(kv[1]));
      });
    });
    document.querySelectorAll('[data-idioma]').forEach(function (b) {
      b.setAttribute('aria-pressed', String(b.dataset.idioma === idioma));
    });
  }

  function montarIdiomas() {
    document.querySelectorAll('[data-idioma]').forEach(function (b) {
      b.addEventListener('click', function () {
        if (b.dataset.idioma === idioma) return;
        idioma = b.dataset.idioma;
        try { localStorage.setItem(CHAVE_IDIOMA, idioma); } catch (e) {}
        // O ?lang= da URL venceria a escolha num recarregar; sai dela.
        var url = new URL(location.href);
        if (url.searchParams.has('lang')) {
          url.searchParams.delete('lang');
          history.replaceState(null, '', url);
        }
        aplicarIdioma();
      });
    });
  }

  // ---------- download ----------

  var PLATAFORMAS = [
    { chave: 'windows_amd64', nome: 'Windows' },
    { chave: 'darwin_arm64', nome: 'macOS' },
    { chave: 'linux_amd64', nome: 'Linux' },
  ];

  // "<dono>/<repo>" das releases no GitHub, validado antes de entrar em URL.
  function repoGitHub() {
    var g = window.HYPHP_RELEASES && window.HYPHP_RELEASES.github;
    return typeof g === 'string' && /^[\w.-]+\/[\w.-]+$/.test(g) ? g : null;
  }

  function releases() {
    var d = window.HYPHP_RELEASES;
    if (!d || d.schema !== 1 || !Array.isArray(d.releases) || !d.releases.length || !repoGitHub()) return null;
    return d.releases.filter(function (r) {
      return r && /^\d+\.\d+\.\d+$/.test(r.version) && r.windows_amd64 && caminhoSeguro(r.windows_amd64.path);
    });
  }

  // Os instaladores são assets das releases do GitHub (contrato-releases.md
  // §Download): o asset é o último segmento do path, na tag v<versão>. A 1.0.0
  // guarda o path da época em que o site servia o arquivo, e cai na mesma regra.
  function urlDownload(r, a) {
    return 'https://github.com/' + repoGitHub() + '/releases/download/v' + r.version + '/' + a.path.split('/').pop();
  }

  // Refeito a cada troca de idioma: as abas, a linha de metadados, o comando
  // do Mac e do Linux e o aviso levam texto.
  function montarDownload(lista) {
    var raiz = document.querySelector('[data-baixar]');
    var abas = raiz.querySelector('.abas');
    var arquivo = raiz.querySelector('[data-arquivo]');
    var botao = raiz.querySelector('[data-botao]');
    var meta = raiz.querySelector('[data-meta]');
    var comando = raiz.querySelector('[data-comando]');
    var aviso = raiz.querySelector('[data-aviso]');
    abas.replaceChildren();
    aviso.innerHTML = t('avisoHtml');
    comando.hidden = true;

    if (!lista || !lista.length) {
      arquivo.textContent = t('semVersoes');
      return;
    }
    var ultima = lista[0];

    function mostrar(chave) {
      var a = ultima[chave];
      var mac = chave === 'darwin_arm64';
      var linux = chave === 'linux_amd64';
      arquivo.replaceChildren(
        el('span', { classe: 'seta', 'aria-hidden': 'true', texto: '↓' }),
        el('span', { texto: a.path.split('/').pop() }),
        el('span', { classe: 'peso', texto: tamanho(a.size) })
      );
      botao.href = urlDownload(ultima, a);
      botao.removeAttribute('aria-disabled');
      botao.setAttribute('aria-label', t('baixarAria', { versao: ultima.version, sistema: nomeDe(chave), tamanho: tamanho(a.size) }));

      var hash = el('span', { classe: 'hash', title: a.sha256, texto: a.sha256.slice(0, 8) + '…' + a.sha256.slice(-4) });
      var copiarHash = el('button', { type: 'button', classe: 'copiar', texto: t('copiar') });
      copiarAoClicar(copiarHash, hash, a.sha256);
      meta.replaceChildren(
        el('span', { texto: 'v' + ultima.version }),
        el('span', { texto: data(ultima.date) }),
        el('span', { texto: 'SHA-256' }),
        hash,
        copiarHash
      );
      if (mac) meta.append(el('span', { texto: t('requisitoMac') }));
      if (linux) meta.append(el('span', { texto: t('requisitoLinux') }));

      // No Mac, o install.sh pelo Terminal instala sem o passo do
      // Gatekeeper, que barra o dmg baixado pelo navegador. No Linux, o apt
      // instala o .deb baixado com as dependências. Uma linha só, cortada com
      // reticências; o title mostra e o copiar copia o inteiro.
      comando.replaceChildren();
      comando.hidden = !mac && !linux;
      if (mac || linux) {
        var linha = mac
          ? 'curl -fsSL https://github.com/' + repoGitHub() + '/releases/latest/download/install.sh | sh'
          : 'sudo apt install ./' + a.path.split('/').pop();
        var codigo = el('code', { title: linha, texto: linha });
        var copiarComando = el('button', { type: 'button', classe: 'copiar', texto: t('copiar') });
        copiarAoClicar(copiarComando, codigo, linha);
        comando.append(codigo, copiarComando);
      }
      aviso.innerHTML = t(mac ? 'avisoMacHtml' : linux ? 'avisoLinuxHtml' : 'avisoHtml');
    }

    PLATAFORMAS.forEach(function (p) {
      var tem = !!ultima[p.chave] && caminhoSeguro(ultima[p.chave].path);
      var aba = el('button', { type: 'button', role: 'tab', classe: 'aba', 'aria-selected': String(p.chave === 'windows_amd64') }, [p.nome]);
      if (!tem) {
        aba.disabled = true;
        aba.title = t('emBreveTitulo');
        aba.append(el('span', { classe: 'selo', texto: t('emBreve') }));
      } else {
        aba.addEventListener('click', function () {
          abas.querySelectorAll('.aba').forEach(function (b) { b.setAttribute('aria-selected', String(b === aba)); });
          mostrar(p.chave);
        });
      }
      abas.append(aba);
    });
    mostrar('windows_amd64');
    document.querySelector('[data-versao-rodape]').textContent = '· v' + ultima.version;
  }

  function nomeDe(chave) {
    for (var i = 0; i < PLATAFORMAS.length; i++) if (PLATAFORMAS[i].chave === chave) return PLATAFORMAS[i].nome;
    return chave;
  }

  // copiarAoClicar copia texto quando o botão é clicado e confirma no próprio
  // botão. Sem clipboard (alguns navegadores em file://), o texto inteiro
  // aparece selecionado em alvo para o Ctrl+C.
  function copiarAoClicar(botao, alvo, texto) {
    botao.addEventListener('click', function () {
      var ok = navigator.clipboard && navigator.clipboard.writeText(texto);
      if (!ok) return mostrarParaCopiar(alvo, texto, botao);
      ok.then(
        function () {
          botao.textContent = t('copiado');
          setTimeout(function () { botao.textContent = t('copiar'); }, 2000);
        },
        function () { mostrarParaCopiar(alvo, texto, botao); }
      );
    });
  }

  function mostrarParaCopiar(alvo, texto, botao) {
    alvo.textContent = texto;
    botao.textContent = 'Ctrl+C';
    var sel = window.getSelection();
    var r = document.createRange();
    r.selectNodeContents(alvo);
    sel.removeAllRanges();
    sel.addRange(r);
  }

  function ligarVersoes() {
    var dialogo = document.querySelector('[data-versoes]');
    document.querySelector('[data-abrir-versoes]').addEventListener('click', function () { dialogo.showModal(); });
    dialogo.querySelector('[data-fechar]').addEventListener('click', function () { dialogo.close(); });
    dialogo.addEventListener('click', function (e) { if (e.target === dialogo) dialogo.close(); });
  }

  // Notas no idioma escolhido: `notes` é o português e `notes_en` o inglês
  // (contrato-releases.md). A 1.0.0 e a 2.0.0 saíram só em português, e é
  // o que aparece para elas em inglês também.
  function notasDe(r) {
    var n = idioma === 'en' && Array.isArray(r.notes_en) && r.notes_en.length ? r.notes_en : r.notes;
    return Array.isArray(n) && n.length ? n : [t('semNotas')];
  }

  function montarVersoes(lista) {
    var ol = document.querySelector('[data-lista-versoes]');
    ol.replaceChildren();
    if (!lista) {
      ol.append(el('li', { texto: t('semVersoes') }));
      return;
    }
    lista.forEach(function (r) {
      var notas = el('ul');
      notasDe(r).forEach(function (n) {
        notas.append(el('li', { texto: String(n) }));
      });
      // Cada plataforma publicada na versão: Windows desde a 1.0.0, macOS
      // desde a 4.0.0, Linux desde a 5.0.0.
      var plats = PLATAFORMAS.filter(function (p) { return r[p.chave] && caminhoSeguro(r[p.chave].path); });
      var cab = el('div', { classe: 'cab' }, [
        el('strong', { texto: 'v' + r.version }),
        el('span', { texto: data(r.date) }),
      ]);
      plats.forEach(function (p) {
        cab.append(el('a', { href: urlDownload(r, r[p.chave]), texto: p.nome + ' · ' + tamanho(r[p.chave].size) }));
      });
      var item = el('li', null, [cab, notas]);
      plats.forEach(function (p) {
        item.append(el('p', { classe: 'hash', texto: p.nome + ' · SHA-256 ' + r[p.chave].sha256 }));
      });
      ol.append(item);
    });
  }

  function versaoAtual() {
    var lista = releases();
    return lista && lista.length ? lista[0].version : '';
  }

  // ---------- comparação ----------

  // Sobrevive à troca de idioma: o bloco escolhido e se o ciclo já parou.
  var matriz = { li: null, fi: null, fixado: false, timer: null };

  function montarMatriz() {
    var dados = window.HYPHP_COMPARACAO;
    var tabela = document.querySelector('[data-matriz]');
    var detalhe = document.querySelector('[data-detalhe]');
    if (!dados) return;
    var ferr = dados.ferramentas;
    var rotulos = { sim: t('sim'), parcial: t('parcial'), nao: t('nao') };
    clearInterval(matriz.timer);
    tabela.querySelectorAll('thead, tbody').forEach(function (n) { n.remove(); });

    var cab = el('tr', null, [el('td')]);
    // A versão avaliada fica sob o nome: lida uma vez por coluna, ela não
    // precisa se repetir na linha da fonte de cada bloco.
    ferr.forEach(function (f) {
      var th = el('th', { scope: 'col', classe: f.id === 'hyphp' ? 'eu' : '', title: f.versao ? (f.nomeLongo || f.nome) + ' — ' + t('versaoAvaliada') + ' ' + tx(f, 'versao') : f.nome }, [f.nome]);
      th.append(el('small', { texto: f.versaoCurta || versaoAtual() }));
      cab.append(th);
    });
    var corpo = el('tbody');
    var botoes = [];
    var i = 0;
    dados.linhas.forEach(function (linha, li) {
      var tr = el('tr', null, [el('th', { scope: 'row', texto: tx(linha, 'rotulo') })]);
      ferr.forEach(function (f, fi) {
        var b = linha.blocos[f.id];
        var classe = 'b ' + (b.suporte || 'sem') + (f.id === 'hyphp' ? ' eu' : '');
        var btn = el('button', {
          type: 'button',
          classe: classe,
          'aria-pressed': 'false',
          'aria-label': (f.nomeLongo || f.nome) + ', ' + tx(linha, 'criterio') + ': ' + (b.suporte ? rotulos[b.suporte] + ', ' + tx(b, 'texto') : t('semInfo')),
          style: '--i:' + i++,
        });
        // Passar o mouse só pré-visualiza; clique (ou foco pelo teclado) fixa.
        // Sem isso, levar o mouse até o link da fonte cruzaria outros blocos
        // e trocaria o conteúdo no caminho.
        btn.addEventListener('click', function () { escolher(li, fi, true, true); });
        btn.addEventListener('focus', function () { escolher(li, fi, true, true); });
        btn.addEventListener('mouseenter', function () { if (!matriz.fixado) escolher(li, fi, true, false); });
        botoes.push({ btn: btn, li: li, fi: fi, fonte: !!b.fonte });
        tr.append(el('td', null, [btn]));
      });
      corpo.append(tr);
    });
    tabela.append(el('thead', null, [cab]), corpo);

    var atual = null;
    function escolher(li, fi, doUsuario, fixar) {
      if (doUsuario) pararCiclo();
      if (fixar) matriz.fixado = true;
      var alvo = botoes.filter(function (x) { return x.li === li && x.fi === fi; })[0].btn;
      if (alvo === atual && !fixar) return;
      if (atual) atual.setAttribute('aria-pressed', 'false');
      atual = alvo;
      matriz.li = li;
      matriz.fi = fi;
      atual.setAttribute('aria-pressed', 'true');
      desenharDetalhe(dados.linhas[li], ferr[fi]);
    }

    // Cada linha do painel ocupa uma linha só, e o painel tem altura fixa:
    // passar o mouse por blocos com textos de tamanhos diferentes não faz o
    // card pular. Texto que não coube abre no "mais", por clique.
    function desenharDetalhe(linha, f) {
      var b = linha.blocos[f.id];
      var fonte;
      if (f.id === 'hyphp') {
        fonte = el('p', { classe: 'fonte', texto: t('recursoProprio') });
      } else if (b.fonte) {
        fonte = el('p', { classe: 'fonte' }, [
          t('fonte'),
          el('a', { href: b.fonte, target: '_blank', rel: 'noopener noreferrer', texto: new URL(b.fonte).host.replace(/^www\./, '') + ' ↗' }),
          t('lidoEm') + data(b.em),
        ]);
      } else {
        fonte = el('p', { classe: 'fonte', texto: tx(b, 'nota') });
      }
      var mais = el('button', { type: 'button', classe: 'mais', 'aria-expanded': 'false', texto: t('mais') });
      detalhe.classList.remove('aberto');
      detalhe.replaceChildren(
        el('p', { classe: 'quem' }, [el('b', { texto: f.nomeLongo || f.nome }), ' · ' + tx(linha, 'criterio')]),
        el('p', { classe: 'oque' }, [
          el('i', { classe: 'b ' + (b.suporte || 'sem') + (f.id === 'hyphp' ? ' eu' : ''), 'aria-hidden': 'true' }),
          b.suporte ? rotulos[b.suporte] + ' — ' + tx(b, 'texto') : t('semInfo'),
        ]),
        el('div', { classe: 'fonte-linha' }, [fonte, mais])
      );
      var cortado = [].some.call(detalhe.querySelectorAll('.quem, .oque, .fonte'), function (p) {
        return p.scrollWidth > p.clientWidth + 1;
      });
      mais.hidden = !cortado;
      mais.addEventListener('click', function () {
        var aberto = detalhe.classList.toggle('aberto');
        mais.textContent = aberto ? t('menos') : t('mais');
        mais.setAttribute('aria-expanded', String(aberto));
      });
    }

    // Enquanto ninguém mexe, o destaque passeia pelos blocos com fonte, um de
    // cada vez. Ao primeiro toque ele para e não volta: quem está lendo manda.
    var ciclo = botoes.filter(function (x) { return x.fonte; });
    var passo = 0;
    function pararCiclo() {
      clearInterval(matriz.timer);
      matriz.timer = null;
      detalhe.setAttribute('aria-live', 'polite');
    }
    if (matriz.li !== null) {
      // Troca de idioma: o mesmo bloco, e o ciclo só volta se ninguém mexeu.
      passo = Math.max(0, ciclo.findIndex(function (x) { return x.li === matriz.li && x.fi === matriz.fi; }));
      escolher(matriz.li, matriz.fi, false, matriz.fixado);
      if (matriz.fixado) return pararCiclo();
    } else {
      escolher(ciclo[0].li, ciclo[0].fi, false);
    }
    if (!reduzir) {
      detalhe.setAttribute('aria-live', 'off');
      matriz.timer = setInterval(function () {
        if (document.hidden) return;
        passo = (passo + 1) % ciclo.length;
        escolher(ciclo[passo].li, ciclo[passo].fi, false);
      }, 3200);
    }
  }

  // ---------- telas ----------
  //
  // A aba Telas troca o conteúdo do palco pela galeria, sem sair da página:
  // #screens mostra a galeria, qualquer outro hash volta ao início. Enquanto
  // ninguém mexe, as telas passam sozinhas; ao primeiro toque, param.

  var TELAS = [
    { id: 'dashboard', titulo: 'telaDashboard', texto: 'telaDashboardTexto' },
    { id: 'projects', titulo: 'telaProjetos', texto: 'telaProjetosTexto' },
    { id: 'runtimes', titulo: 'telaRuntimes', texto: 'telaRuntimesTexto' },
    { id: 'database', titulo: 'telaBanco', texto: 'telaBancoTexto' },
    { id: 'mail', titulo: 'telaMail', texto: 'telaMailTexto' },
    { id: 'cli', titulo: 'telaCli', texto: 'telaCliTexto' },
    { id: 'settings', titulo: 'telaConfig', texto: 'telaConfigTexto' },
  ];
  // As capturas têm 1200×760 de janela em escala 2.
  var TELA_LARGURA = 2400;
  var TELA_ALTURA = 1520;
  var galeria = { atual: 0, timer: null, parado: false };

  function vistaAtual() {
    return location.hash === '#screens' ? 'telas' : 'inicio';
  }

  function mostrarVista() {
    var vista = vistaAtual();
    document.querySelectorAll('[data-vista]').forEach(function (m) { m.hidden = m.dataset.vista !== vista; });
    document.querySelectorAll('[data-vista-link]').forEach(function (a) {
      if (a.dataset.vistaLink === vista) a.setAttribute('aria-current', 'page');
      else a.removeAttribute('aria-current');
    });
    document.body.dataset.vista = vista;
    if (vista === 'telas') cicloGaleria();
    else clearInterval(galeria.timer);
  }

  function montarGaleria() {
    var lista = document.querySelector('[data-telas-lista]');
    var quadro = document.querySelector('[data-quadro]');
    var pasta = idioma === 'en' ? 'en' : 'pt';
    lista.replaceChildren();
    quadro.replaceChildren();
    TELAS.forEach(function (tela, i) {
      var id = 'tela-' + tela.id;
      var aba = el('button', { type: 'button', role: 'tab', classe: 'tela-aba', id: id + '-aba', 'aria-controls': 'tela-quadro', 'aria-selected': String(i === galeria.atual), tabindex: i === galeria.atual ? '0' : '-1' }, [
        el('span', { classe: 'tela-titulo', texto: t(tela.titulo) }),
        el('span', { classe: 'tela-texto', texto: t(tela.texto) }),
      ]);
      aba.addEventListener('click', function () { escolherTela(i, true); });
      aba.addEventListener('keydown', function (e) {
        var passo = e.key === 'ArrowDown' ? 1 : e.key === 'ArrowUp' ? -1 : 0;
        if (!passo) return;
        e.preventDefault();
        var prox = (galeria.atual + passo + TELAS.length) % TELAS.length;
        escolherTela(prox, true);
        lista.children[prox].focus();
      });
      lista.append(aba);
      quadro.append(el('img', {
        src: 'assets/telas/' + pasta + '/' + tela.id + '.webp',
        width: String(TELA_LARGURA),
        height: String(TELA_ALTURA),
        alt: t('telaAlt', { titulo: t(tela.titulo) }),
        classe: i === galeria.atual ? 'ativa' : '',
        loading: 'lazy',
        decoding: 'async',
      }));
    });
    quadro.id = 'tela-quadro';
    quadro.setAttribute('aria-labelledby', 'tela-' + TELAS[galeria.atual].id + '-aba');
  }

  function escolherTela(i, doUsuario) {
    if (doUsuario) {
      galeria.parado = true;
      clearInterval(galeria.timer);
      document.querySelector('[data-telas-lista]').classList.remove('ciclando');
    }
    galeria.atual = i;
    var abas = document.querySelectorAll('.tela-aba');
    var imgs = document.querySelectorAll('[data-quadro] img');
    abas.forEach(function (a, k) {
      a.setAttribute('aria-selected', String(k === i));
      a.tabIndex = k === i ? 0 : -1;
    });
    imgs.forEach(function (img, k) { img.classList.toggle('ativa', k === i); });
    document.querySelector('[data-quadro]').setAttribute('aria-labelledby', 'tela-' + TELAS[i].id + '-aba');
  }

  function cicloGaleria() {
    clearInterval(galeria.timer);
    var rodando = !reduzir && !galeria.parado;
    document.querySelector('[data-telas-lista]').classList.toggle('ciclando', rodando);
    if (!rodando) return;
    galeria.timer = setInterval(function () {
      if (document.hidden) return;
      escolherTela((galeria.atual + 1) % TELAS.length, false);
    }, 5000);
  }

  // ---------- idioma aplicado ----------

  var lista = releases();

  function aplicarIdioma() {
    traduzirPagina();
    montarDownload(lista);
    montarVersoes(lista);
    montarMatriz();
    montarGaleria();
  }

  // ---------- fundo de blocos ----------
  //
  // O glifo do HyPHP (coluna alta, quadrado, 2×2) desenhado em quadradinhos
  // pontilhados, que se desfazem em poeira. Monta uma vez ao abrir e depois só
  // cintila alguns pixels de vez em quando; com movimento reduzido fica parado.

  var PECAS = [
    [0, 0, 7, 16, 1], [9, 0, 16, 7, 1],
    [9, 9, 12, 12, 0.6], [13, 9, 16, 12, 0.6], [9, 13, 12, 16, 0.6], [13, 13, 16, 16, 0.6],
  ];
  var BAYER = [0, 8, 2, 10, 12, 4, 14, 6, 3, 11, 1, 9, 15, 7, 13, 5];

  function aleatorio(semente) {
    return function () {
      semente |= 0;
      semente = (semente + 0x6d2b79f5) | 0;
      var t = Math.imul(semente ^ (semente >>> 15), 1 | semente);
      t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
      return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
    };
  }

  // Distância com sinal até o glifo (negativa dentro), em unidades do viewBox.
  function distancia(x, y) {
    var d = Infinity;
    for (var i = 0; i < PECAS.length; i++) {
      var p = PECAS[i], r = p[4];
      var cx = (p[0] + p[2]) / 2, cy = (p[1] + p[3]) / 2;
      var qx = Math.abs(x - cx) - ((p[2] - p[0]) / 2 - r);
      var qy = Math.abs(y - cy) - ((p[3] - p[1]) / 2 - r);
      var fora = Math.hypot(Math.max(qx, 0), Math.max(qy, 0));
      var v = fora + Math.min(Math.max(qx, qy), 0) - r;
      if (v < d) d = v;
    }
    return d;
  }

  var CORES = [[29, 58, 107], [59, 130, 246], [96, 165, 250], [230, 237, 247]];

  function mistura(t) {
    var s = Math.max(0, Math.min(1, t)) * (CORES.length - 2);
    var i = Math.floor(s), f = s - i;
    var a = CORES[i], b = CORES[i + 1];
    return [a[0] + (b[0] - a[0]) * f, a[1] + (b[1] - a[1]) * f, a[2] + (b[2] - a[2]) * f];
  }

  function montarBlocos() {
    var canvas = document.querySelector('.blocos');
    var ctx = canvas.getContext('2d');
    var pixels = [], cintilam = [], passo = 6, lado = 5, dpr = 1;
    var animando = null;

    function calcular() {
      dpr = Math.min(window.devicePixelRatio || 1, 2);
      var w = window.innerWidth, h = window.innerHeight;
      canvas.width = Math.round(w * dpr);
      canvas.height = Math.round(h * dpr);
      var rnd = aleatorio(20260926);
      // Abaixo do topo: pixel atrás do menu atrapalha a leitura.
      var topo = document.querySelector('.topo').getBoundingClientRect().bottom + 12;
      var alto = Math.min(h - topo - 24, w * 0.48);
      var u = alto / 16;
      var ox = w - alto * 0.86, oy = topo + (h - topo - alto) / 2;
      // Contorno e poeira medidos em blocos, não em unidades do glifo: a
      // espessura fica igual em qualquer tamanho de tela.
      var borda = (passo * 1.6) / u, poeira = (passo * 1.2) / u;
      var cols = Math.ceil(w / passo), lins = Math.ceil(h / passo);
      pixels = [];
      cintilam = [];
      for (var gy = 0; gy < lins; gy++) {
        for (var gx = 0; gx < cols; gx++) {
          var px = gx * passo, py = gy * passo;
          var x = (px + passo / 2 - ox) / u, y = (py + passo / 2 - oy) / u;
          var d = distancia(x, y);
          if (d > poeira * 5) continue;
          var limiar = (BAYER[(gy % 4) * 4 + (gx % 4)] + 0.5) / 16;
          var alto01 = 1 - Math.max(0, Math.min(1, y / 16));
          var alfa = 0, tom = 0.5, pisca = false;
          if (d > -borda && d <= 0) {
            // Contorno aceso: é ele que desenha a forma.
            alfa = 0.42 + alto01 * 0.18;
            tom = 0.55 + alto01 * 0.25;
            pisca = true;
          } else if (d <= 0) {
            // Miolo: pontilhado esparso que escurece para dentro.
            var fundo = Math.min(1, -d / 2.5);
            if (limiar < 0.3 - fundo * 0.22 + alto01 * 0.06) {
              alfa = 0.16 + (1 - fundo) * 0.14;
              tom = 0.2 + (1 - fundo) * 0.25;
            }
          } else if (rnd() < 0.18 * Math.exp(-d / poeira)) {
            // Poeira colada à borda, que some em poucos blocos.
            alfa = 0.15 + rnd() * 0.35;
            tom = 0.5 + rnd() * 0.35;
            pisca = true;
          }
          if (alfa > 0) {
            var c = mistura(tom + (rnd() - 0.5) * 0.1);
            var item = { x: px, y: py, c: c, a: alfa };
            pixels.push(item);
            if (pisca) cintilam.push(item);
          }
        }
      }
    }

    function pintar(item, a) {
      var c = item.c;
      ctx.clearRect(item.x * dpr, item.y * dpr, lado * dpr, lado * dpr);
      ctx.fillStyle = 'rgba(' + (c[0] | 0) + ',' + (c[1] | 0) + ',' + (c[2] | 0) + ',' + a.toFixed(3) + ')';
      ctx.fillRect(item.x * dpr, item.y * dpr, lado * dpr, lado * dpr);
    }

    function tudo() {
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      for (var i = 0; i < pixels.length; i++) pintar(pixels[i], pixels[i].a);
    }

    // O CSS esconde o fundo quando a página empilha; aí não há o que desenhar.
    function visivel() {
      return getComputedStyle(canvas).display !== 'none';
    }

    function montar() {
      if (!visivel()) return;
      calcular();
      if (reduzir) return tudo();
      // Entrada: os blocos aparecem em ordem embaralhada em ~0,9 s.
      var ordem = pixels.slice();
      var rnd = aleatorio(7);
      for (var i = ordem.length - 1; i > 0; i--) {
        var j = Math.floor(rnd() * (i + 1));
        var t = ordem[i]; ordem[i] = ordem[j]; ordem[j] = t;
      }
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      var feito = 0, inicio = performance.now();
      cancelAnimationFrame(animando);
      (function quadro(agora) {
        var alvo = Math.min(ordem.length, Math.ceil(((agora - inicio) / 900) * ordem.length));
        for (; feito < alvo; feito++) pintar(ordem[feito], ordem[feito].a);
        if (feito < ordem.length) animando = requestAnimationFrame(quadro);
      })(inicio);
    }

    // Cintilar: poucos pixels por vez, só com a aba visível.
    if (!reduzir) {
      var rndC = aleatorio(42);
      setInterval(function () {
        if (document.hidden || !cintilam.length) return;
        for (var k = 0; k < 5; k++) {
          var item = cintilam[Math.floor(rndC() * cintilam.length)];
          pintar(item, rndC() < 0.5 ? item.a * 0.3 : Math.min(1, item.a * 1.6));
        }
      }, 140);
    }

    var espera = null;
    window.addEventListener('resize', function () {
      clearTimeout(espera);
      espera = setTimeout(function () {
        if (!visivel()) {
          pixels = [];
          cintilam = [];
          return;
        }
        calcular();
        tudo();
      }, 150);
    });
    montar();
  }

  // ---------- início ----------
  montarIdiomas();
  ligarVersoes();
  aplicarIdioma();
  montarBlocos();
  window.addEventListener('hashchange', mostrarVista);
  mostrarVista();
})();
