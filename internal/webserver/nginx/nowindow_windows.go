package nginx

// createNoWindow é CREATE_NO_WINDOW da API do Windows.
//
// SysProcAttr.HideWindow sozinho não basta: ele só passa SW_HIDE pelo
// STARTUPINFO, e o console de um processo console nasce antes disso — a janela
// pisca. Validate roda a cada Reconcile, então sem esta flag o usuário via um
// console aparecer toda vez que um projeto mudava.
const createNoWindow = 0x08000000
