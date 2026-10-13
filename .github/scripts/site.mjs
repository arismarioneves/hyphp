// Site (web/).
//
//   node .github/scripts/site.mjs conferir
//   node .github/scripts/site.mjs espelhar <destino>
import { execFileSync } from 'node:child_process';
import { copyFileSync, existsSync, mkdirSync, readdirSync, readFileSync, rmdirSync, unlinkSync } from 'node:fs';
import { dirname, join, posix } from 'node:path';
import { pathToFileURL } from 'node:url';
import vm from 'node:vm';

export const PROTEGIDOS = ['.git/', '.gitignore', '.gitattributes', 'README.md', 'docs/', 'releases/', 'catalogo/'];

// Documentação de web/, que não vai para o site.
const SO_AQUI = ['README.md', 'README.pt-BR.md'];

// Arquivos que só existem com build ou gerenciador de pacotes.
const COM_BUILD = /(^|\/)(package(-lock)?\.json|yarn\.lock|pnpm-lock\.yaml|bun\.lockb?|tsconfig\.json|(vite|webpack|rollup|postcss|tailwind)\.config\.\w+|node_modules\/.*)$|\.(ts|tsx|jsx|mjs|cjs|scss|sass|less|vue|svelte)$/;

const DATA = /^\d{4}-\d{2}-\d{2}$/;

// Só os arquivos versionados: cópias locais fora do índice não contam, e no
// destino só o que ele versiona pode ser apagado.
function versionados(raiz, pasta = 'web') {
  const saida = execFileSync('git', ['-C', raiz, 'ls-files', '-z', '--', pasta], { encoding: 'utf8' });
  const prefixo = pasta === '.' ? '' : `${pasta}/`;
  return saida.split('\0').filter(Boolean).map((p) => p.slice(prefixo.length));
}

function protegido(rel) {
  return PROTEGIDOS.some((p) => (p.endsWith('/') ? rel.startsWith(p) : rel === p));
}

function externo(url) {
  return /^(https?:)?\/\//i.test(url);
}

function carregar(raiz, rel, nome) {
  const window = {};
  vm.runInNewContext(readFileSync(join(raiz, 'web', rel), 'utf8'), { window }, { filename: rel });
  return window[nome];
}

// conferir devolve os problemas de web/ (vazio quando está tudo certo).
export function conferir(raiz) {
  const arquivos = versionados(raiz);
  const tem = new Set(arquivos);
  const problemas = [];
  const textos = tem.has('assets/textos.js') ? (carregar(raiz, 'assets/textos.js', 'HYPHP_TEXTOS') ?? {}) : null;

  for (const f of arquivos) {
    if (COM_BUILD.test(f)) problemas.push(`web/${f}: o site não tem build nem dependências`);
    else if (protegido(f) && !SO_AQUI.includes(f)) problemas.push(`web/${f}: não pode ficar em web/`);
  }

  if (tem.has('index.html')) {
    const html = readFileSync(join(raiz, 'web', 'index.html'), 'utf8');
    for (const [, tag, attrs] of html.matchAll(/<([a-z]+)\b([^>]*)>/gi)) {
      for (const [, attr, valor] of attrs.matchAll(/\b(src|href)="([^"]*)"/gi)) {
        if (tag.toLowerCase() === 'a' || valor.startsWith('#') || valor.startsWith('data:')) continue;
        if (externo(valor)) {
          problemas.push(`web/index.html: <${tag} ${attr}="${valor}">: o site não carrega nada de fora`);
          continue;
        }
        const rel = posix.normalize(valor.split(/[?#]/)[0]);
        if (!protegido(rel) && !tem.has(rel)) problemas.push(`web/index.html: ${valor} não existe em web/`);
      }
    }
    if (textos) {
      const usadas = new Set();
      for (const [, k] of html.matchAll(/\bdata-t(?:-html)?="([^"]+)"/g)) usadas.add(k);
      for (const [, pares] of html.matchAll(/\bdata-t-attr="([^"]+)"/g)) {
        for (const par of pares.split(';')) usadas.add(par.split(':')[1]);
      }
      for (const k of usadas) {
        if (!(k in (textos.en ?? {}))) problemas.push(`web/index.html: a chave "${k}" não existe em assets/textos.js`);
      }
    }
  }

  for (const css of arquivos.filter((f) => f.endsWith('.css'))) {
    const texto = readFileSync(join(raiz, 'web', css), 'utf8');
    // Com aspas, o valor vai até a aspa de fechamento: um data: URI pode ter
    // aspas simples, parênteses e outro url() dentro.
    for (const [, duplas, simples, sem] of texto.matchAll(/url\(\s*(?:"([^"]*)"|'([^']*)'|([^)'"]*))\s*\)/g)) {
      const url = (duplas ?? simples ?? sem).trim();
      if (url.startsWith('data:')) continue;
      if (externo(url)) {
        problemas.push(`web/${css}: url(${url}): o site não carrega nada de fora`);
        continue;
      }
      const rel = posix.normalize(posix.join(posix.dirname(css), url.split(/[?#]/)[0]));
      if (!tem.has(rel)) problemas.push(`web/${css}: url(${url}) não existe em web/`);
    }
  }

  if (textos) {
    const { en = {}, pt = {} } = textos;
    for (const k of Object.keys(en)) if (!(k in pt)) problemas.push(`web/assets/textos.js: "${k}" falta em pt`);
    for (const k of Object.keys(pt)) if (!(k in en)) problemas.push(`web/assets/textos.js: "${k}" falta em en`);
  }

  const telas = (idioma) => new Set(arquivos.filter((f) => f.startsWith(`assets/telas/${idioma}/`)).map((f) => posix.basename(f)));
  const [telasEN, telasPT] = [telas('en'), telas('pt')];
  for (const f of telasEN) if (!telasPT.has(f)) problemas.push(`web/assets/telas/pt/${f}: falta (existe em en)`);
  for (const f of telasPT) if (!telasEN.has(f)) problemas.push(`web/assets/telas/en/${f}: falta (existe em pt)`);

  if (tem.has('assets/comparacao.js')) {
    const comparacao = carregar(raiz, 'assets/comparacao.js', 'HYPHP_COMPARACAO') ?? { linhas: [] };
    for (const linha of comparacao.linhas) {
      for (const [quem, bloco] of Object.entries(linha.blocos)) {
        if (quem === 'hyphp') continue;
        const onde = `web/assets/comparacao.js: "${linha.rotulo}" › ${quem}`;
        if (bloco.suporte === null) {
          if (!bloco.nota) problemas.push(`${onde}: sem fonte e sem nota`);
        } else if (!/^https:\/\//.test(bloco.fonte ?? '') || !DATA.test(bloco.em ?? '')) {
          problemas.push(`${onde}: afirma "${bloco.suporte}" sem fonte https e data de leitura (AAAA-MM-DD)`);
        }
      }
    }
  }
  return problemas;
}

function apagarVazias(dir, prefixo = '') {
  for (const e of readdirSync(join(dir, prefixo), { withFileTypes: true })) {
    const rel = prefixo ? `${prefixo}/${e.name}` : e.name;
    if (!e.isDirectory() || protegido(`${rel}/`)) continue;
    apagarVazias(dir, rel);
    if (readdirSync(join(dir, rel)).length === 0) rmdirSync(join(dir, rel));
  }
}

// espelhar deixa destino igual a web/, fora os PROTEGIDOS. Devolve o que mudou.
export function espelhar(raiz, destino) {
  const origem = versionados(raiz).filter((f) => !SO_AQUI.includes(f));
  const proibidos = origem.filter(protegido);
  if (proibidos.length > 0) throw new Error(`não podem ficar em web/: ${proibidos.join(', ')}`);
  const feito = [];
  for (const f of origem) {
    const de = join(raiz, 'web', f);
    const para = join(destino, f);
    if (existsSync(para) && readFileSync(de).equals(readFileSync(para))) continue;
    mkdirSync(dirname(para), { recursive: true });
    copyFileSync(de, para);
    feito.push(`copiado ${f}`);
  }
  const manter = new Set(origem);
  for (const f of versionados(destino, '.')) {
    if (manter.has(f) || protegido(f) || !existsSync(join(destino, f))) continue;
    unlinkSync(join(destino, f));
    feito.push(`apagado ${f}`);
  }
  apagarVazias(destino);
  return feito;
}

if (import.meta.url === pathToFileURL(process.argv[1]).href) {
  const [comando, destino] = process.argv.slice(2);
  const raiz = process.cwd();
  if (comando === 'conferir') {
    const problemas = conferir(raiz);
    for (const p of problemas) console.error(p);
    if (problemas.length > 0) {
      console.error(`${problemas.length} problema(s) em web/`);
      process.exit(1);
    }
    console.log(`web/ ok: ${versionados(raiz).length} arquivos`);
  } else if (comando === 'espelhar' && destino) {
    const feito = espelhar(raiz, destino);
    for (const linha of feito) console.log(linha);
    console.log(feito.length === 0 ? 'nada mudou' : `${feito.length} arquivo(s) mudaram`);
  } else {
    console.error('uso: node .github/scripts/site.mjs conferir | espelhar <destino>');
    process.exit(2);
  }
}
