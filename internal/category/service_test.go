package category

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
)

// ---------------------------------------------------------------------------
// fixtures
// ---------------------------------------------------------------------------

func ptr(id int64) *int64 { return &id }

func globalCategory(id int64, name string) *Category {
	return &Category{ID: id, UserID: nil, Name: name, IsGlobal: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
}

func personalCategory(id, userID int64, name string) *Category {
	return &Category{ID: id, UserID: ptr(userID), Name: name, IsGlobal: false, CreatedAt: time.Now(), UpdatedAt: time.Now()}
}

func ctxWithUser(userID int64) context.Context {
	return identity.WithUserID(context.Background(), userID)
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestService_Create_Success(t *testing.T) {
	want := personalCategory(1, 42, "Gasolina")
	svc := NewService(&mockCategoryRepository{
		CreateFn: func(_ context.Context, userID int64, req *CreateCategoryRequest) (*Category, error) {
			return want, nil
		},
	})

	got, err := svc.Create(ctxWithUser(42), &CreateCategoryRequest{Name: "Gasolina"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != want.ID {
		t.Errorf("expected ID %d, got %d", want.ID, got.ID)
	}
}

func TestService_Create_NoUserID(t *testing.T) {
	svc := NewService(&mockCategoryRepository{})
	_, err := svc.Create(context.Background(), &CreateCategoryRequest{Name: "X"})
	if !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_Create_EmptyName(t *testing.T) {
	svc := NewService(&mockCategoryRepository{})
	_, err := svc.Create(ctxWithUser(1), &CreateCategoryRequest{Name: "   "})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_NilRequest(t *testing.T) {
	svc := NewService(&mockCategoryRepository{})
	_, err := svc.Create(ctxWithUser(1), nil)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Create_CollidesWithGlobal(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		ExistsGlobalByNameFn: func(_ context.Context, name string) (bool, error) {
			return strings.EqualFold(name, "Alimentação"), nil
		},
	})
	_, err := svc.Create(ctxWithUser(42), &CreateCategoryRequest{Name: "alimentação"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for global name collision, got %v", err)
	}
}

func TestService_Create_ConflictFromRepository(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		CreateFn: func(_ context.Context, _ int64, _ *CreateCategoryRequest) (*Category, error) {
			return nil, apperrors.ErrConflict
		},
	})
	_, err := svc.Create(ctxWithUser(1), &CreateCategoryRequest{Name: "Duplicada"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestService_Create_RepositoryError(t *testing.T) {
	repoErr := errors.New("db failure")
	svc := NewService(&mockCategoryRepository{
		CreateFn: func(_ context.Context, _ int64, _ *CreateCategoryRequest) (*Category, error) {
			return nil, repoErr
		},
	})
	_, err := svc.Create(ctxWithUser(1), &CreateCategoryRequest{Name: "X"})
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestService_List_Success(t *testing.T) {
	expected := []Category{
		*globalCategory(1, "Alimentação"),
		*personalCategory(7, 42, "Gasolina"),
	}
	svc := NewService(&mockCategoryRepository{
		ListForUserFn: func(_ context.Context, userID int64) ([]Category, error) {
			if userID != 42 {
				t.Errorf("expected userID 42, got %d", userID)
			}
			return expected, nil
		},
	})

	got, err := svc.List(ctxWithUser(42))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 items, got %d", len(got))
	}
}

func TestService_List_NoUserID(t *testing.T) {
	svc := NewService(&mockCategoryRepository{})
	_, err := svc.List(context.Background())
	if !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_List_RepositoryError(t *testing.T) {
	repoErr := errors.New("connection lost")
	svc := NewService(&mockCategoryRepository{
		ListForUserFn: func(_ context.Context, _ int64) ([]Category, error) {
			return nil, repoErr
		},
	})
	_, err := svc.List(ctxWithUser(1))
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestService_Update_Success(t *testing.T) {
	original := personalCategory(5, 42, "Antigo")
	updated := personalCategory(5, 42, "Novo")
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(_ context.Context, id int64) (*Category, error) {
			return original, nil
		},
		UpdateFn: func(_ context.Context, _, _ int64, _ *UpdateCategoryRequest) (*Category, error) {
			return updated, nil
		},
	})

	got, err := svc.Update(ctxWithUser(42), 5, &UpdateCategoryRequest{Name: "Novo"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Name != "Novo" {
		t.Errorf("expected name Novo, got %s", got.Name)
	}
}

func TestService_Update_NoUserID(t *testing.T) {
	svc := NewService(&mockCategoryRepository{})
	_, err := svc.Update(context.Background(), 1, &UpdateCategoryRequest{Name: "X"})
	if !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_Update_EmptyName(t *testing.T) {
	svc := NewService(&mockCategoryRepository{})
	_, err := svc.Update(ctxWithUser(1), 1, &UpdateCategoryRequest{Name: ""})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Update_NotFound(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(_ context.Context, _ int64) (*Category, error) { return nil, nil },
	})
	_, err := svc.Update(ctxWithUser(1), 99, &UpdateCategoryRequest{Name: "X"})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Update_GlobalCategoryForbidden(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(_ context.Context, id int64) (*Category, error) {
			return globalCategory(id, "Alimentação"), nil
		},
	})
	_, err := svc.Update(ctxWithUser(1), 1, &UpdateCategoryRequest{Name: "Comida"})
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestService_Update_OtherUserCategory(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(_ context.Context, id int64) (*Category, error) {
			return personalCategory(id, 99, "Alheia"), nil // belongs to user 99
		},
	})
	_, err := svc.Update(ctxWithUser(42), 5, &UpdateCategoryRequest{Name: "X"}) // user 42 tries to update
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound (no info leak), got %v", err)
	}
}

func TestService_Update_CollidesWithGlobal(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(_ context.Context, id int64) (*Category, error) {
			return personalCategory(id, 42, "Minha"), nil
		},
		ExistsGlobalByNameFn: func(_ context.Context, name string) (bool, error) {
			return strings.EqualFold(name, "Alimentação"), nil
		},
	})
	_, err := svc.Update(ctxWithUser(42), 5, &UpdateCategoryRequest{Name: "ALIMENTAÇÃO"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for global name collision, got %v", err)
	}
}

func TestService_Update_ConflictFromRepository(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(_ context.Context, id int64) (*Category, error) {
			return personalCategory(id, 42, "Original"), nil
		},
		UpdateFn: func(_ context.Context, _, _ int64, _ *UpdateCategoryRequest) (*Category, error) {
			return nil, apperrors.ErrConflict
		},
	})
	_, err := svc.Update(ctxWithUser(42), 5, &UpdateCategoryRequest{Name: "Duplicada"})
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestService_Delete_Success(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(_ context.Context, id int64) (*Category, error) {
			return personalCategory(id, 42, "Gasolina"), nil
		},
		DeleteFn: func(_ context.Context, _, _ int64) (bool, error) { return true, nil },
	})
	if err := svc.Delete(ctxWithUser(42), 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestService_Delete_NoUserID(t *testing.T) {
	svc := NewService(&mockCategoryRepository{})
	if err := svc.Delete(context.Background(), 1); !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(_ context.Context, _ int64) (*Category, error) { return nil, nil },
	})
	if err := svc.Delete(ctxWithUser(1), 99); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestService_Delete_GlobalCategoryForbidden(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(_ context.Context, id int64) (*Category, error) {
			return globalCategory(id, "Saúde"), nil
		},
	})
	if err := svc.Delete(ctxWithUser(1), 3); !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

func TestService_Delete_OtherUserCategory(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(_ context.Context, id int64) (*Category, error) {
			return personalCategory(id, 99, "Alheia"), nil
		},
	})
	if err := svc.Delete(ctxWithUser(42), 5); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound (no info leak), got %v", err)
	}
}

func TestService_Delete_ConflictInUse(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(_ context.Context, id int64) (*Category, error) {
			return personalCategory(id, 42, "Em uso"), nil
		},
		DeleteFn: func(_ context.Context, _, _ int64) (bool, error) {
			return false, apperrors.ErrConflict
		},
	})
	if err := svc.Delete(ctxWithUser(42), 5); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict (category in use), got %v", err)
	}
}
