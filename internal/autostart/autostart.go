package autostart

import "slices"

// Flag vai na entrada de login gravada por Apply: é por ela que o HyPHP sabe
// que quem o abriu foi o sistema, e não o usuário.
const Flag = "--bandeja"

// Hidden diz se a janela deve nascer escondida: só quando o app foi aberto
// pela entrada de login (args contém Flag) e o usuário pediu para ficar na
// bandeja. Aberto pelo atalho, a janela sempre aparece.
func Hidden(args []string, startHidden bool) bool {
	return startHidden && slices.Contains(args, Flag)
}
