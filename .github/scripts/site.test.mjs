// node --test .github/scripts/site.test.mjs
import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { test } from 'node:test';

import { conferir, espelhar } from './site.mjs';

const TEXTOS = `window.HYPHP_TEXTOS = { en: { titulo: 'HyPHP', baixar: 'Download' }, pt: { titulo: 'HyPHP', baixar: 'Baixar' } };\n`;
const COMPARACAO = `window.HYPHP_COMPARACAO = { linhas: [{ rotulo: 'PHP por projeto', blocos: {
  hyphp: { suporte: 'sim', texto: 'hyphp.yaml' },
  laragon: { suporte: 'parcial', texto: 'Profile', fonte: 'https://laragon.org/docs/profile', em: '2026-09-26' },
  xampp: { suporte: null, nota: 'A documentação não diz.' },
} }] };\n`;

// Um web/ que passa na conferência; cada caso muda uma coisa.
function siteMinimo() {
  return {
    'web/index.html': '<!DOCTYPE html>\n<link rel="stylesheet" href="assets/site.css">\n<a href="https://github.com/arismarioneves/hyphp">GitHub</a>\n<h1 data-t="titulo">HyPHP</h1>\n<button data-t-attr="aria-label:baixar">x</button>\n<script src="releases/releases.js"></script>\n<script src="assets/textos.js"></script>\n<script src="assets/comparacao.js"></script>\n',
    'web/assets/site.css': `@font-face { src: url('fontes/inter.woff2'); }\nbody { background: url("data:image/svg+xml,%3Csvg filter='url(%23g)'%3E") }\n`,
    'web/assets/fontes/inter.woff2': 'fonte',
    'web/assets/textos.js': TEXTOS,
    'web/assets/comparacao.js': COMPARACAO,
    'web/assets/telas/en/dashboard.webp': 'en',
    'web/assets/telas/pt/dashboard.webp': 'pt',
    'web/README.md': '# regras\n',
  };
}

function repositorio(t, versionados, foraDoIndice = {}) {
  const raiz = mkdtempSync(join(tmpdir(), 'site-'));
  t.after(() => rmSync(raiz, { recursive: true, force: true }));
  execFileSync('git', ['-C', raiz, 'init', '-q']);
  gravar(raiz, versionados);
  execFileSync('git', ['-C', raiz, 'add', '-A']);
  gravar(raiz, foraDoIndice);
  return raiz;
}

function gravar(raiz, arquivos) {
  for (const [rel, conteudo] of Object.entries(arquivos)) {
    mkdirSync(dirname(join(raiz, rel)), { recursive: true });
    writeFileSync(join(raiz, rel), conteudo);
  }
}

test('o site mínimo passa na conferência', (t) => {
  assert.deepEqual(conferir(repositorio(t, siteMinimo())), []);
});

test('a conferência recusa o que quebra o site', async (t) => {
  const casos = [
    ['releases/ em web/', { 'web/releases/releases.js': 'window.HYPHP_RELEASES = {};' }, 'não pode ficar em web/'],
    ['catalogo/ em web/', { 'web/catalogo/catalog.json': '{}' }, 'não pode ficar em web/'],
    ['docs/ em web/', { 'web/docs/notas.md': 'x' }, 'não pode ficar em web/'],
    ['package.json', { 'web/package.json': '{}' }, 'não tem build'],
    ['TypeScript', { 'web/assets/app.ts': 'export {}' }, 'não tem build'],
    ['módulo ES', { 'web/assets/app.mjs': 'export {}' }, 'não tem build'],
    ['script de CDN', { 'web/index.html': siteMinimo()['web/index.html'] + '<script src="https://cdn.example.com/x.js"></script>\n' }, 'nada de fora'],
    ['recurso que não existe', { 'web/index.html': siteMinimo()['web/index.html'] + '<img src="assets/logo.png">\n' }, 'assets/logo.png não existe'],
    ['fonte de fora no CSS', { 'web/assets/site.css': "@import url('https://fonts.example.com/inter.css');\n" }, 'nada de fora'],
    ['tradução faltando', { 'web/assets/textos.js': TEXTOS.replace("baixar: 'Baixar' ", '') }, '"baixar" falta em pt'],
    ['chave usada que não existe', { 'web/index.html': siteMinimo()['web/index.html'] + '<p data-t="inexistente"></p>\n' }, '"inexistente" não existe'],
    ['captura só em um idioma', { 'web/assets/telas/en/mail.webp': 'en' }, 'telas/pt/mail.webp: falta'],
    ['concorrente sem fonte', { 'web/assets/comparacao.js': COMPARACAO.replace(", fonte: 'https://laragon.org/docs/profile'", '') }, 'sem fonte https'],
  ];
  for (const [nome, mudanca, quer] of casos) {
    await t.test(nome, (t) => {
      const problemas = conferir(repositorio(t, { ...siteMinimo(), ...mudanca }));
      assert.ok(problemas.some((p) => p.includes(quer)), `quer "${quer}", veio ${JSON.stringify(problemas)}`);
    });
  }
});

test('o espelhamento deixa o destino igual a web/ sem tocar nos protegidos', (t) => {
  const raiz = repositorio(t, siteMinimo(), { 'web/releases/releases.js': 'cópia local', 'web/rascunho.html': 'fora do índice' });
  const protegidos = {
    'releases/index.json': '{"schema":1}',
    'releases/releases.js': 'window.HYPHP_RELEASES = {"schema":1};',
    'releases/.htaccess': 'cache\n',
    'catalogo/catalog.json': '{"serial":2026101100}',
    'catalogo/catalog.json.sig': 'assinatura\n',
    'catalogo/.htaccess': 'cache\n',
    'README.md': '# destino\n',
    'docs/notas.md': '# notas\n',
    '.gitignore': 'x\n',
  };
  const locais = { '.smoke/roteiro.js': 'local', 'assets/notas.txt': 'local' };
  const destino = repositorio(t, {
    ...protegidos,
    'index.html': 'versão antiga',
    'assets/site.css': siteMinimo()['web/assets/site.css'],
    'assets/antigo/velho.css': 'saiu do site',
  }, locais);

  const feito = espelhar(raiz, destino);

  for (const [rel, conteudo] of Object.entries({ ...protegidos, ...locais })) {
    assert.equal(readFileSync(join(destino, rel), 'utf8'), conteudo, `${rel} mudou`);
  }
  assert.equal(readFileSync(join(destino, 'index.html'), 'utf8'), siteMinimo()['web/index.html']);
  assert.ok(!existsSync(join(destino, 'assets/antigo')), 'a pasta vazia continuou');
  assert.ok(!existsSync(join(destino, 'rascunho.html')), 'arquivo fora do índice foi copiado');
  assert.deepEqual(feito.sort(), [
    'apagado assets/antigo/velho.css',
    'copiado assets/comparacao.js',
    'copiado assets/fontes/inter.woff2',
    'copiado assets/telas/en/dashboard.webp',
    'copiado assets/telas/pt/dashboard.webp',
    'copiado assets/textos.js',
    'copiado index.html',
  ]);
  assert.deepEqual(espelhar(raiz, destino), [], 'a segunda vez mudou alguma coisa');
});

test('o espelhamento recusa web/ com um caminho protegido', (t) => {
  const raiz = repositorio(t, { ...siteMinimo(), 'web/catalogo/catalog.json.sig': 'outra assinatura' });
  const destino = mkdtempSync(join(tmpdir(), 'destino-'));
  t.after(() => rmSync(destino, { recursive: true, force: true }));
  assert.throws(() => espelhar(raiz, destino), /catalogo\/catalog\.json\.sig/);
});
