package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// contribuidorGitHub é um item de GET /repos/{repo}/contributors.
type contribuidorGitHub struct {
	Login string `json:"login"`
	Type  string `json:"type"`
}

// contribuidores pede ao GitHub todas as páginas da lista, já em ordem de
// contribuições, e devolve os logins sem as contas de bot (Dependabot,
// github-actions), que não são gente a creditar na tela Sobre.
func contribuidores(gh func(args ...string) ([]byte, error), repo string) ([]string, error) {
	out, err := gh("api", "--paginate", "--slurp", "repos/"+repo+"/contributors?per_page=100")
	if err != nil {
		return nil, fmt.Errorf("gh api contributors: %w", err)
	}
	var paginas [][]contribuidorGitHub
	if err := json.Unmarshal(out, &paginas); err != nil {
		return nil, fmt.Errorf("gh api contributors ilegível: %w", err)
	}
	var logins []string
	for _, pagina := range paginas {
		for _, c := range pagina {
			if c.Login == "" || c.Type == "Bot" || strings.HasSuffix(c.Login, "[bot]") {
				continue
			}
			logins = append(logins, c.Login)
		}
	}
	// Lista vazia é o GitHub respondendo errado: o autor sempre aparece, e
	// gravar [] tiraria todo mundo do app publicado.
	if len(logins) == 0 {
		return nil, errors.New("gh api contributors: nenhum contribuidor na resposta")
	}
	return logins, nil
}
