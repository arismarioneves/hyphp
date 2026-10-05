package services

// defaultRootDir devolve "" no Windows: mantém o diálogo de hoje, sem pasta
// inicial, para não mudar o comportamento já conhecido pelos usuários.
func defaultRootDir() string { return "" }
