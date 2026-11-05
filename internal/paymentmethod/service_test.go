package paymentmethod

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/jonasjesusamerico/we-sync-api/internal/apperrors"
	"github.com/jonasjesusamerico/we-sync-api/internal/identity"
)

// ---------------------------------------------------------------------------
// fixtures
// ---------------------------------------------------------------------------

var fixedTime = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func ptr(id int64) *int64 { return &id }

func globalPaymentMethod(id int64, name string) *PaymentMethod {
	return &PaymentMethod{ID: id, UserID: nil, Name: name, IsGlobal: true, CreatedAt: fixedTime, UpdatedAt: fixedTime}
}

func personalPaymentMethod(id, userID int64, name string) *PaymentMethod {
	return &PaymentMethod{ID: id, UserID: ptr(userID), Name: name, IsGlobal: false, CreatedAt: fixedTime, UpdatedAt: fixedTime}
}

func archivedPaymentMethod(id, userID int64, name string) *PaymentMethod {
	pm := personalPaymentMethod(id, userID, name)
	archivedAt := fixedTime
	pm.ArchivedAt = &archivedAt
	return pm
}

func ctxWithUser(userID int64) context.Context {
	return identity.WithUserID(context.Background(), userID)
}

func lookup(pm *PaymentMethod) func(context.Context, int64) (*PaymentMethod, error) {
	return func(context.Context, int64) (*PaymentMethod, error) { return pm, nil }
}

// ---------------------------------------------------------------------------
// validation
// ---------------------------------------------------------------------------

func TestValidateName(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantName string
		wantErr  string
	}{
		{"empty", "", "", "name is required"},
		{"blank", "   ", "", "name is required"},
		{"one rune", "V", "V", ""},
		{"trimmed", "  Vale-refeição ", "Vale-refeição", ""},
		{"40 runes", strings.Repeat("ç", 40), strings.Repeat("ç", 40), ""},
		{"41 runes", strings.Repeat("ç", 41), "", "name must be at most 40 characters"},
		{"40 emojis", strings.Repeat("💳", 40), strings.Repeat("💳", 40), ""},
		{"41 emojis", strings.Repeat("💳", 41), "", "name must be at most 40 characters"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateName(tc.in)
			if tc.wantErr == "" {
				if err != nil || got != tc.wantName {
					t.Fatalf("got (%q, %v), want (%q, nil)", got, err, tc.wantName)
				}
				return
			}
			if !errors.Is(err, apperrors.ErrValidation) || err.Error() != tc.wantErr {
				t.Fatalf("err = %v, want validation error %q", err, tc.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestService_Create_Success(t *testing.T) {
	want := personalPaymentMethod(1, 42, "Vale-refeição")
	var gotUserID int64
	var gotName string
	svc := NewService(&mockPaymentMethodRepository{
		CreateFn: func(_ context.Context, userID int64, req *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
			gotUserID, gotName = userID, req.Name
			return want, true, nil
		},
	})

	got, created, err := svc.Create(ctxWithUser(42), &CreatePaymentMethodRequest{Name: " Vale-refeição  "})
	if err != nil || got != want || !created {
		t.Fatalf("got (%v, %v, %v), want (%v, true, nil)", got, created, err, want)
	}
	if gotUserID != 42 || gotName != "Vale-refeição" {
		t.Fatalf("repository received userID=%d name=%q", gotUserID, gotName)
	}
}

func TestService_Create_Reactivates(t *testing.T) {
	want := personalPaymentMethod(203, 42, "Cartão antigo")
	svc := NewService(&mockPaymentMethodRepository{
		CreateFn: func(context.Context, int64, *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
			return want, false, nil
		},
	})
	got, created, err := svc.Create(ctxWithUser(42), &CreatePaymentMethodRequest{Name: "cartao antigo"})
	if err != nil || got != want || created {
		t.Fatalf("got (%v, %v, %v), want (%v, false, nil)", got, created, err, want)
	}
}

func TestService_Create_ActiveOwnName(t *testing.T) {
	cases := []struct {
		name string
		fn   func(context.Context, int64, *CreatePaymentMethodRequest) (*PaymentMethod, bool, error)
	}{
		{"no row", func(context.Context, int64, *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
			return nil, false, nil
		}},
		{"conflict", func(context.Context, int64, *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
			return nil, false, apperrors.ErrConflict
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(&mockPaymentMethodRepository{CreateFn: tc.fn})
			_, _, err := svc.Create(ctxWithUser(42), &CreatePaymentMethodRequest{Name: "Cartão da empresa"})
			if !errors.Is(err, ErrNameConflict) {
				t.Fatalf("err = %v, want ErrNameConflict", err)
			}
		})
	}
}

func TestService_Create_DefaultName(t *testing.T) {
	for _, name := range []string{"pix", "CARTAO DE CREDITO"} {
		t.Run(name, func(t *testing.T) {
			var gotName string
			svc := NewService(&mockPaymentMethodRepository{
				ExistsGlobalByNameFn: func(_ context.Context, n string) (bool, error) { gotName = n; return true, nil },
				CreateFn: func(context.Context, int64, *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
					t.Fatal("repository Create must not be called")
					return nil, false, nil
				},
			})
			_, _, err := svc.Create(ctxWithUser(42), &CreatePaymentMethodRequest{Name: name})
			if !errors.Is(err, ErrDefaultNameConflict) {
				t.Fatalf("err = %v, want ErrDefaultNameConflict", err)
			}
			if gotName != name {
				t.Fatalf("ExistsGlobalByName received %q", gotName)
			}
		})
	}
}

func TestService_Create_NoUserID(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, _, err := svc.Create(context.Background(), &CreatePaymentMethodRequest{Name: "X"})
	if !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_Create_NilRequest(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, _, err := svc.Create(ctxWithUser(1), nil)
	if !errors.Is(err, apperrors.ErrValidation) || err.Error() != "name is required" {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestService_Create_NameTooLong(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, _, err := svc.Create(ctxWithUser(1), &CreatePaymentMethodRequest{Name: strings.Repeat("a", 41)})
	if !errors.Is(err, apperrors.ErrValidation) || err.Error() != "name must be at most 40 characters" {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestService_Create_RepositoryError(t *testing.T) {
	repoErr := errors.New("db failure")
	svc := NewService(&mockPaymentMethodRepository{
		CreateFn: func(context.Context, int64, *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
			return nil, false, repoErr
		},
	})
	_, _, err := svc.Create(ctxWithUser(1), &CreatePaymentMethodRequest{Name: "X"})
	if !errors.Is(err, repoErr) {
		t.Errorf("expected wrapped repoErr, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestService_List_Success(t *testing.T) {
	expected := []PaymentMethod{
		*globalPaymentMethod(1, "Pix"),
		*personalPaymentMethod(201, 42, "Vale-refeição"),
		*archivedPaymentMethod(203, 42, "Cartão antigo"),
	}
	svc := NewService(&mockPaymentMethodRepository{
		ListForUserFn: func(_ context.Context, userID int64) ([]PaymentMethod, error) {
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
	if len(got) != 3 {
		t.Errorf("expected 3 items, archived included, got %d", len(got))
	}
}

func TestService_List_NoUserID(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, err := svc.List(context.Background())
	if !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_List_RepositoryError(t *testing.T) {
	repoErr := errors.New("connection lost")
	svc := NewService(&mockPaymentMethodRepository{
		ListForUserFn: func(context.Context, int64) ([]PaymentMethod, error) { return nil, repoErr },
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
	updated := personalPaymentMethod(202, 42, "Cartão corporativo")
	var gotUserID, gotID int64
	var gotName string
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: lookup(personalPaymentMethod(202, 42, "Cartão da empresa")),
		UpdateFn: func(_ context.Context, userID, id int64, req *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
			gotUserID, gotID, gotName = userID, id, req.Name
			return updated, nil
		},
	})

	got, err := svc.Update(ctxWithUser(42), 202, &UpdatePaymentMethodRequest{Name: " Cartão corporativo "})
	if err != nil || got != updated {
		t.Fatalf("got (%v, %v), want (%v, nil)", got, err, updated)
	}
	if gotUserID != 42 || gotID != 202 || gotName != "Cartão corporativo" {
		t.Fatalf("repository received userID=%d id=%d name=%q", gotUserID, gotID, gotName)
	}
}

func TestService_Update_Archived(t *testing.T) {
	called := false
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: lookup(archivedPaymentMethod(203, 42, "Cartão antigo")),
		UpdateFn: func(context.Context, int64, int64, *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
			called = true
			return archivedPaymentMethod(203, 42, "Cartão velho"), nil
		},
	})
	if _, err := svc.Update(ctxWithUser(42), 203, &UpdatePaymentMethodRequest{Name: "Cartão velho"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("an archived payment method must be editable")
	}
}

func TestService_Update_NoUserID(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, err := svc.Update(context.Background(), 1, &UpdatePaymentMethodRequest{Name: "X"})
	if !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_Update_NilRequest(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{})
	_, err := svc.Update(ctxWithUser(1), 1, nil)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation, got %v", err)
	}
}

func TestService_Update_Ownership(t *testing.T) {
	tests := []struct {
		name     string
		existing *PaymentMethod
		want     error
	}{
		{"missing", nil, apperrors.ErrNotFound},
		{"global", globalPaymentMethod(1, "Pix"), apperrors.ErrForbidden},
		{"other user", personalPaymentMethod(5, 99, "Alheia"), apperrors.ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(&mockPaymentMethodRepository{
				GetByIDFn: lookup(tc.existing),
				UpdateFn: func(context.Context, int64, int64, *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
					t.Fatal("repository Update must not be called")
					return nil, nil
				},
			})
			_, err := svc.Update(ctxWithUser(42), 5, &UpdatePaymentMethodRequest{Name: "Outra"})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestService_Update_ValidationBeforeLookup(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: func(context.Context, int64) (*PaymentMethod, error) {
			t.Fatal("GetByID must not be called before validation")
			return nil, nil
		},
	})
	_, err := svc.Update(ctxWithUser(42), 5, &UpdatePaymentMethodRequest{Name: strings.Repeat("a", 41)})
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestService_Update_DefaultName(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn:            lookup(personalPaymentMethod(202, 42, "Cartão da empresa")),
		ExistsGlobalByNameFn: func(context.Context, string) (bool, error) { return true, nil },
	})
	_, err := svc.Update(ctxWithUser(42), 202, &UpdatePaymentMethodRequest{Name: "pix"})
	if !errors.Is(err, ErrDefaultNameConflict) {
		t.Fatalf("err = %v, want ErrDefaultNameConflict", err)
	}
}

func conflictRepo(other *PaymentMethod, gotUserID *int64, gotName *string) *mockPaymentMethodRepository {
	return &mockPaymentMethodRepository{
		GetByIDFn: lookup(personalPaymentMethod(202, 42, "Cartão da empresa")),
		UpdateFn: func(context.Context, int64, int64, *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
			return nil, apperrors.ErrConflict
		},
		FindOwnByNameFn: func(_ context.Context, userID int64, name string) (*PaymentMethod, error) {
			*gotUserID, *gotName = userID, name
			return other, nil
		},
	}
}

func TestService_Update_ConflictWithActive(t *testing.T) {
	var gotUserID int64
	var gotName string
	svc := NewService(conflictRepo(personalPaymentMethod(201, 42, "Vale-refeição"), &gotUserID, &gotName))
	_, err := svc.Update(ctxWithUser(42), 202, &UpdatePaymentMethodRequest{Name: " vale-refeicao "})
	if !errors.Is(err, ErrNameConflict) || errors.Is(err, ErrArchivedNameConflict) {
		t.Fatalf("err = %v, want ErrNameConflict", err)
	}
	if gotUserID != 42 || gotName != "vale-refeicao" {
		t.Fatalf("FindOwnByName received userID=%d name=%q", gotUserID, gotName)
	}
}

func TestService_Update_ConflictWithArchived(t *testing.T) {
	var gotUserID int64
	var gotName string
	svc := NewService(conflictRepo(archivedPaymentMethod(203, 42, "Cartão antigo"), &gotUserID, &gotName))
	_, err := svc.Update(ctxWithUser(42), 202, &UpdatePaymentMethodRequest{Name: "Cartão antigo"})
	if !errors.Is(err, ErrArchivedNameConflict) {
		t.Fatalf("err = %v, want ErrArchivedNameConflict", err)
	}
}

func TestService_Update_ConflictRowGone(t *testing.T) {
	var gotUserID int64
	var gotName string
	svc := NewService(conflictRepo(nil, &gotUserID, &gotName))
	_, err := svc.Update(ctxWithUser(42), 202, &UpdatePaymentMethodRequest{Name: "Cartão antigo"})
	if !errors.Is(err, ErrNameConflict) {
		t.Fatalf("err = %v, want ErrNameConflict", err)
	}
}

func TestService_Update_NotFoundOnWrite(t *testing.T) {
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: lookup(personalPaymentMethod(202, 42, "Cartão da empresa")),
		UpdateFn: func(context.Context, int64, int64, *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
			return nil, nil
		},
	})
	_, err := svc.Update(ctxWithUser(42), 202, &UpdatePaymentMethodRequest{Name: "Outra"})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestService_Update_RepositoryError(t *testing.T) {
	repoErr := errors.New("db failure")
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn: lookup(personalPaymentMethod(202, 42, "Cartão da empresa")),
		UpdateFn: func(context.Context, int64, int64, *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
			return nil, repoErr
		},
	})
	_, err := svc.Update(ctxWithUser(42), 202, &UpdatePaymentMethodRequest{Name: "Outra"})
	if !errors.Is(err, repoErr) {
		t.Fatalf("expected wrapped repoErr, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Archive / Unarchive
// ---------------------------------------------------------------------------

func TestService_ArchiveAndUnarchive(t *testing.T) {
	ops := []struct {
		name     string
		archived bool
		call     func(*Service, context.Context, int64) error
	}{
		{"Archive", true, (*Service).Archive},
		{"Unarchive", false, (*Service).Unarchive},
	}
	cases := []struct {
		name     string
		ctx      context.Context
		existing *PaymentMethod
		found    bool
		want     error
		setCalls int
	}{
		{"own", ctxWithUser(42), personalPaymentMethod(5, 42, "Vale-refeição"), true, nil, 1},
		{"own archived", ctxWithUser(42), archivedPaymentMethod(5, 42, "Cartão antigo"), true, nil, 1},
		{"global", ctxWithUser(42), globalPaymentMethod(5, "Pix"), true, apperrors.ErrForbidden, 0},
		{"other user", ctxWithUser(42), personalPaymentMethod(5, 99, "Alheia"), true, apperrors.ErrNotFound, 0},
		{"missing", ctxWithUser(42), nil, true, apperrors.ErrNotFound, 0},
		{"gone on write", ctxWithUser(42), personalPaymentMethod(5, 42, "Vale-refeição"), false, apperrors.ErrNotFound, 1},
		{"no user", context.Background(), personalPaymentMethod(5, 42, "Vale-refeição"), true, apperrors.ErrUnauthorized, 0},
	}
	for _, op := range ops {
		for _, tc := range cases {
			t.Run(op.name+"/"+tc.name, func(t *testing.T) {
				var calls int
				svc := NewService(&mockPaymentMethodRepository{
					GetByIDFn: lookup(tc.existing),
					SetArchivedFn: func(_ context.Context, userID, id int64, archived bool) (bool, error) {
						calls++
						if userID != 42 || id != 5 || archived != op.archived {
							t.Errorf("SetArchived(%d, %d, %v), want (42, 5, %v)", userID, id, archived, op.archived)
						}
						return tc.found, nil
					},
				})
				err := op.call(svc, tc.ctx, 5)
				if tc.want == nil {
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
				} else if !errors.Is(err, tc.want) {
					t.Fatalf("err = %v, want %v", err, tc.want)
				}
				if calls != tc.setCalls {
					t.Fatalf("SetArchived calls = %d, want %d", calls, tc.setCalls)
				}
			})
		}
	}
}

func TestService_Archive_RepositoryError(t *testing.T) {
	repoErr := errors.New("db failure")
	svc := NewService(&mockPaymentMethodRepository{
		GetByIDFn:     lookup(personalPaymentMethod(5, 42, "Vale-refeição")),
		SetArchivedFn: func(context.Context, int64, int64, bool) (bool, error) { return false, repoErr },
	})
	if err := svc.Archive(ctxWithUser(42), 5); !errors.Is(err, repoErr) {
		t.Fatalf("expected wrapped repoErr, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// isolation
// ---------------------------------------------------------------------------

func TestService_UserIDAlwaysFromContext(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		userID := rapid.Int64Range(1, 1_000_000).Draw(rt, "userID")
		id := rapid.Int64Range(1, 1_000_000).Draw(rt, "id")
		check := func(op string, got int64) {
			if got != userID {
				rt.Fatalf("%s: repository received userID %d, want %d (from the context)", op, got, userID)
			}
		}
		svc := NewService(&mockPaymentMethodRepository{
			GetByIDFn: lookup(personalPaymentMethod(id, userID, "Vale-refeição")),
			CreateFn: func(_ context.Context, u int64, _ *CreatePaymentMethodRequest) (*PaymentMethod, bool, error) {
				check("Create", u)
				return personalPaymentMethod(id, u, "Vale-refeição"), true, nil
			},
			UpdateFn: func(_ context.Context, u, _ int64, _ *UpdatePaymentMethodRequest) (*PaymentMethod, error) {
				check("Update", u)
				return personalPaymentMethod(id, u, "Vale-refeição"), nil
			},
			SetArchivedFn: func(_ context.Context, u, _ int64, _ bool) (bool, error) {
				check("SetArchived", u)
				return true, nil
			},
		})
		ctx := ctxWithUser(userID)

		if _, _, err := svc.Create(ctx, &CreatePaymentMethodRequest{Name: "Vale-refeição"}); err != nil {
			rt.Fatalf("Create: %v", err)
		}
		if _, err := svc.Update(ctx, id, &UpdatePaymentMethodRequest{Name: "Vale-refeição"}); err != nil {
			rt.Fatalf("Update: %v", err)
		}
		if err := svc.Archive(ctx, id); err != nil {
			rt.Fatalf("Archive: %v", err)
		}
		if err := svc.Unarchive(ctx, id); err != nil {
			rt.Fatalf("Unarchive: %v", err)
		}
	})
}
