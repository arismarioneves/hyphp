# Contribuindo com o HyPHP

Obrigado pelo interesse. Issues e pull requests são bem-vindos.

## Licença das contribuições

Toda contribuição exige aceitar o [CLA](CLA.md) (Contributor License Agreement, baseado no ICLA da Apache). Você continua dono do que escreveu. O que o CLA faz é autorizar o mantenedor a distribuir a sua contribuição, inclusive nas partes pagas do HyPHP (pasta `enterprise/`). Sem isso, código de terceiros não poderia entrar no produto sem risco.

Para aceitar, marque no pull request a caixa:

> - [x] Li o [CLA.md](CLA.md) e aceito os termos para esta e para as minhas próximas contribuições.

Pull request sem essa caixa marcada não é revisado.

## Como a licença funciona

- **Fora de `enterprise/`**: [PolyForm Shield 1.0.0](LICENSE). Qualquer pessoa ou empresa pode usar, estudar, modificar e compartilhar, inclusive no trabalho. O que não pode é oferecer um produto que concorra com o HyPHP ou com os recursos pagos dele, pago ou gratuito. Isso inclui vender o próprio HyPHP.
- **Dentro de `enterprise/`**: [licença própria](enterprise/LICENSE). É onde vão morar os recursos pagos, e o uso exige assinatura. O código fica visível, e você pode modificá-lo para desenvolver e testar contribuições.

Este resumo não substitui o texto das licenças.

## Antes de abrir o pull request

```powershell
go build . ./cmd/... ./services/... ./internal/...
go test . ./cmd/... ./services/... ./internal/...
npx --prefix frontend tsc --noEmit -p frontend
```

Explique nos comentários do código o porquê de cada decisão; o quê o código já diz. Commits em português, sem acentos.
