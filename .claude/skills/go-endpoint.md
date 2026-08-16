# Skill: go-endpoint
# Quando usar: criar qualquer novo endpoint HTTP no backend Go

## Checklist obrigatório antes de começar
1. Existe usecase para esta funcionalidade? Se não → criar usecase primeiro
2. Existe repository com os métodos necessários? Se não → criar método no repo primeiro
3. Qual grupo de rotas no router? (público vs autenticado vs admin)
4. Qual papel mínimo necessário? (SUPER_ADMIN / TENANT_ADMIN / MANAGER / ENGINEER / VIEWER)

## Estrutura padrão de handler

```go
// internal/delivery/handler/exemplo_handler.go
package handler

import (
    "encoding/json"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/voltiq/server/internal/delivery/request"
    "github.com/voltiq/server/internal/domain"
    "github.com/voltiq/server/internal/usecase"
)

type ExemploHandler struct {
    uc *usecase.ExemploUseCase
}

func NewExemploHandler(uc *usecase.ExemploUseCase) *ExemploHandler {
    return &ExemploHandler{uc: uc}
}

// List godoc — GET /api/v1/exemplos
func (h *ExemploHandler) List(w http.ResponseWriter, r *http.Request) {
    tenantID := r.Context().Value("tenant_id").(string)

    result, err := h.uc.List(r.Context(), domain.UUID(tenantID))
    if err != nil {
        request.Error(w, http.StatusInternalServerError, "erro ao listar")
        return
    }

    request.JSON(w, http.StatusOK, result)
}

// Create godoc — POST /api/v1/exemplos
func (h *ExemploHandler) Create(w http.ResponseWriter, r *http.Request) {
    tenantID := r.Context().Value("tenant_id").(string)

    var input ExemploInput
    if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
        request.Error(w, http.StatusBadRequest, "corpo da requisição inválido")
        return
    }

    // Validar input antes de passar ao usecase
    if input.Name == "" {
        request.Error(w, http.StatusUnprocessableEntity, "name é obrigatório")
        return
    }

    result, err := h.uc.Create(r.Context(), domain.UUID(tenantID), input.Name)
    if err != nil {
        request.Error(w, http.StatusInternalServerError, "erro ao criar")
        return
    }

    request.JSON(w, http.StatusCreated, result)
}

type ExemploInput struct {
    Name string `json:"name"`
}
```

## Registrar no router — sempre em router.go

```go
// Adicionar na struct Config:
ExemploHandler *handler.ExemploHandler

// Registrar no grupo correto:
r.Route("/exemplos", func(r chi.Router) {
    r.Get("/", cfg.ExemploHandler.List)
    r.Post("/", cfg.ExemploHandler.Create)
    r.Get("/{id}", cfg.ExemploHandler.GetByID)
    r.Put("/{id}", cfg.ExemploHandler.Update)
    r.Delete("/{id}", cfg.ExemploHandler.Delete)
})
```

## Registrar no main.go — sempre

```go
// Instanciar usecase e handler:
exemploRepo := repository.NewExemploRepository(db)
exemploUseCase := usecase.NewExemploUseCase(exemploRepo)
exemploHandler := handler.NewExemploHandler(exemploUseCase)

// Passar no Config do router:
cfg := router.Config{
    // ... existentes ...
    ExemploHandler: exemploHandler,
}
```

## Regras de resposta HTTP

| Situação | Status |
|---|---|
| Sucesso com dados | 200 OK |
| Recurso criado | 201 Created |
| Sem conteúdo | 204 No Content |
| Input inválido | 400 Bad Request |
| Não autenticado | 401 Unauthorized |
| Sem permissão | 403 Forbidden |
| Não encontrado | 404 Not Found |
| Violação de regra | 422 Unprocessable Entity |
| Limite do plano | 402 Payment Required |
| Erro interno | 500 Internal Server Error |

## Middleware de papel — quando usar

```go
// Somente SUPER_ADMIN:
r.With(cfg.SuperAdminMiddleware.Handler).Delete("/{id}", cfg.ExemploHandler.Delete)

// TENANT_ADMIN ou acima:
r.With(middleware.RequireRole("TENANT_ADMIN", "SUPER_ADMIN")).Post("/", ...)

// Qualquer autenticado (padrão — não precisa middleware extra):
r.Get("/", cfg.ExemploHandler.List)
```

## Verificar limite de plano antes de criar recursos

```go
// No usecase, antes de inserir:
count, _ := uc.repo.CountByTenant(ctx, tenantID)
if tenant.MaxTransformers > 0 && count >= tenant.MaxTransformers {
    return nil, ErrPlanLimitReached
}
```
