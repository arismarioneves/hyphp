package stack

// createNoWindow é CREATE_NO_WINDOW da API do Windows.
//
// SysProcAttr.HideWindow sozinho não basta: ele só passa SW_HIDE pelo
// STARTUPINFO, e o console de um processo console é criado pelo kernel antes
// disso — a janela pisca na tela. Só esta flag impede a criação do console.
// Como o HyPHP roda sem console próprio, todo utilitário de vida curta que ele
// chama (php -v, mysql, mkcert, netstat) piscaria uma janela a cada varredura.
const createNoWindow = 0x08000000
