package domain

// TrainingLessons is the catalog of in-app training lessons (o conteúdo fica
// no frontend; o servidor valida as chaves e calcula o progresso).
var TrainingLessons = []string{
	"prodist-balanco",
	"perdas-tecnicas",
	"cadastro-rede",
	"importacao-leituras",
	"dashboard-operacional",
	"dashboard-gerencial",
	"mapa",
	"alertas",
	"relatorios-aneel",
	"usuarios-permissoes",
}

// IsTrainingLesson reports whether key is a lesson of the catalog.
func IsTrainingLesson(key string) bool {
	for _, k := range TrainingLessons {
		if k == key {
			return true
		}
	}
	return false
}
