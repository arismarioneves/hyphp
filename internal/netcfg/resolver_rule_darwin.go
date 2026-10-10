package netcfg

// ResolverRulePath é a regra de DNS do sufixo .test no macOS. Variável, não
// constante: os testes do helper e do stack apontam para uma pasta temporária
// e nunca escrevem em /etc.
var ResolverRulePath = "/etc/resolver/test"
