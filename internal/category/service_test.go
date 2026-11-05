package category

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

func globalCategory(id int64, name string) *Category {
	return &Category{ID: id, UserID: nil, Name: name, IsGlobal: true, Icon: IconFood, Color: ColorFood,
		CreatedAt: fixedTime, UpdatedAt: fixedTime}
}

func personalCategory(id, userID int64, name string) *Category {
	return &Category{ID: id, UserID: ptr(userID), Name: name, IsGlobal: false, Icon: IconTag, Color: ColorFood,
		CreatedAt: fixedTime, UpdatedAt: fixedTime}
}

func archivedCategory(id, userID int64, name string) *Category {
	c := personalCategory(id, userID, name)
	archivedAt := fixedTime
	c.ArchivedAt = &archivedAt
	return c
}

// validReq is Pets of the prototype: HEART icon and LEISURE color.
func validReq(name string) *CreateCategoryRequest {
	return &CreateCategoryRequest{Name: name, Icon: IconHeart, Color: ColorLeisure}
}

func validUpdate(name string) *UpdateCategoryRequest {
	return &UpdateCategoryRequest{Name: name, Icon: IconHouse, Color: ColorHome}
}

func ctxWithUser(userID int64) context.Context {
	return identity.WithUserID(context.Background(), userID)
}

// ---------------------------------------------------------------------------
// enums and validation
// ---------------------------------------------------------------------------

func TestIcon_IsValid(t *testing.T) {
	for _, i := range []Icon{IconTag, IconHeart, IconSmile, IconBook, IconCar, IconHouse, IconFood, IconCard, IconGift, IconSparkle} {
		if !i.IsValid() {
			t.Errorf("%q must be valid", i)
		}
	}
	for _, i := range []Icon{"", "heart", "DOG", "NEUTRAL"} {
		if i.IsValid() {
			t.Errorf("%q must not be valid", i)
		}
	}
}

func TestColor_IsValid(t *testing.T) {
	for _, c := range []Color{ColorFood, ColorTransport, ColorHome, ColorHealth, ColorEducation, ColorLeisure} {
		if !c.IsValid() {
			t.Errorf("%q must be valid", c)
		}
	}
	for _, c := range []Color{ColorNeutral, "", "leisure", "PINK"} {
		if c.IsValid() {
			t.Errorf("%q must not be valid", c)
		}
	}
}

func TestValidateInput(t *testing.T) {
	const iconMsg = "; allowed values: TAG, HEART, SMILE, BOOK, CAR, HOUSE, FOOD, CARD, GIFT, SPARKLE"
	const colorMsg = "; allowed values: FOOD, TRANSPORT, HOME, HEALTH, EDUCATION, LEISURE"
	tests := []struct {
		name     string
		inName   string
		icon     Icon
		color    Color
		wantName string
		wantErr  string
	}{
		{"empty name", "", IconHeart, ColorLeisure, "", "name is required"},
		{"blank name", "   ", IconHeart, ColorLeisure, "", "name is required"},
		{"one rune", "P", IconHeart, ColorLeisure, "P", ""},
		{"trimmed", "  Pets ", IconHeart, ColorLeisure, "Pets", ""},
		{"40 runes", strings.Repeat("ç", 40), IconHeart, ColorLeisure, strings.Repeat("ç", 40), ""},
		{"41 runes", strings.Repeat("ç", 41), IconHeart, ColorLeisure, "", "name must be at most 40 characters"},
		{"40 emojis", strings.Repeat("🐶", 40), IconHeart, ColorLeisure, strings.Repeat("🐶", 40), ""},
		{"41 emojis", strings.Repeat("🐶", 41), IconHeart, ColorLeisure, "", "name must be at most 40 characters"},
		{"empty icon", "Pets", "", ColorLeisure, "", "icon is required"},
		{"lowercase icon", "Pets", "heart", ColorLeisure, "", `icon "heart" is not valid` + iconMsg},
		{"unknown icon", "Pets", "DOG", ColorLeisure, "", `icon "DOG" is not valid` + iconMsg},
		{"empty color", "Pets", IconHeart, "", "", "color is required"},
		{"neutral color", "Pets", IconHeart, ColorNeutral, "", `color "NEUTRAL" is not valid` + colorMsg},
		{"unknown color", "Pets", IconHeart, "PINK", "", `color "PINK" is not valid` + colorMsg},
		{"lowercase color", "Pets", IconHeart, "leisure", "", `color "leisure" is not valid` + colorMsg},
		{"name before icon", "", "DOG", "PINK", "", "name is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateInput(tc.inName, tc.icon, tc.color)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if got != tc.wantName {
					t.Fatalf("name = %q, want %q", got, tc.wantName)
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
	want := personalCategory(1, 42, "Pets")
	var gotUserID int64
	var gotReq CreateCategoryRequest
	svc := NewService(&mockCategoryRepository{
		CreateFn: func(_ context.Context, userID int64, req *CreateCategoryRequest) (*Category, bool, error) {
			gotUserID, gotReq = userID, *req
			return want, true, nil
		},
	})

	got, created, err := svc.Create(ctxWithUser(42), validReq("  Pets "))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != want || !created {
		t.Fatalf("got (%v, %v), want (%v, true)", got, created, want)
	}
	if gotUserID != 42 || gotReq.Name != "Pets" || gotReq.Icon != IconHeart || gotReq.Color != ColorLeisure {
		t.Fatalf("repository received userID=%d req=%+v", gotUserID, gotReq)
	}
}

func TestService_Create_Reactivates(t *testing.T) {
	want := personalCategory(7, 42, "Pets")
	svc := NewService(&mockCategoryRepository{
		CreateFn: func(context.Context, int64, *CreateCategoryRequest) (*Category, bool, error) {
			return want, false, nil
		},
	})
	got, created, err := svc.Create(ctxWithUser(42), validReq("pets"))
	if err != nil || got != want || created {
		t.Fatalf("got (%v, %v, %v), want (%v, false, nil)", got, created, err, want)
	}
}

func TestService_Create_ActiveOwnName(t *testing.T) {
	cases := []struct {
		name string
		fn   func(context.Context, int64, *CreateCategoryRequest) (*Category, bool, error)
	}{
		{"no row", func(context.Context, int64, *CreateCategoryRequest) (*Category, bool, error) { return nil, false, nil }},
		{"conflict", func(context.Context, int64, *CreateCategoryRequest) (*Category, bool, error) {
			return nil, false, apperrors.ErrConflict
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(&mockCategoryRepository{CreateFn: tc.fn})
			_, _, err := svc.Create(ctxWithUser(42), validReq("Pets"))
			if !errors.Is(err, ErrNameConflict) {
				t.Fatalf("err = %v, want ErrNameConflict", err)
			}
		})
	}
}

func TestService_Create_DefaultName(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		ExistsGlobalByNameFn: func(context.Context, string) (bool, error) { return true, nil },
		CreateFn: func(context.Context, int64, *CreateCategoryRequest) (*Category, bool, error) {
			t.Fatal("repository Create must not be called")
			return nil, false, nil
		},
	})
	_, _, err := svc.Create(ctxWithUser(42), validReq("saude"))
	if !errors.Is(err, ErrDefaultNameConflict) {
		t.Fatalf("err = %v, want ErrDefaultNameConflict", err)
	}
}

func TestService_Create_NoUserID(t *testing.T) {
	svc := NewService(&mockCategoryRepository{})
	_, _, err := svc.Create(context.Background(), validReq("X"))
	if !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_Create_NilRequest(t *testing.T) {
	svc := NewService(&mockCategoryRepository{})
	_, _, err := svc.Create(ctxWithUser(1), nil)
	if !errors.Is(err, apperrors.ErrValidation) || err.Error() != "name is required" {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestService_Create_RepositoryError(t *testing.T) {
	repoErr := errors.New("db failure")
	svc := NewService(&mockCategoryRepository{
		CreateFn: func(context.Context, int64, *CreateCategoryRequest) (*Category, bool, error) {
			return nil, false, repoErr
		},
	})
	_, _, err := svc.Create(ctxWithUser(1), validReq("X"))
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
		*personalCategory(7, 42, "Pets"),
		*archivedCategory(9, 42, "Academia"),
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
	if len(got) != 3 {
		t.Errorf("expected 3 items, archived included, got %d", len(got))
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

func ownedLookup(c *Category) func(context.Context, int64) (*Category, error) {
	return func(context.Context, int64) (*Category, error) { return c, nil }
}

func TestService_Update_Success(t *testing.T) {
	updated := personalCategory(5, 42, "Animais")
	var gotUserID, gotID int64
	var gotReq UpdateCategoryRequest
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: ownedLookup(personalCategory(5, 42, "Pets")),
		UpdateFn: func(_ context.Context, userID, id int64, req *UpdateCategoryRequest) (*Category, error) {
			gotUserID, gotID, gotReq = userID, id, *req
			return updated, nil
		},
	})

	got, err := svc.Update(ctxWithUser(42), 5, validUpdate(" Animais "))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != updated {
		t.Fatalf("got %v, want %v", got, updated)
	}
	if gotUserID != 42 || gotID != 5 || gotReq != (UpdateCategoryRequest{Name: "Animais", Icon: IconHouse, Color: ColorHome}) {
		t.Fatalf("repository received userID=%d id=%d req=%+v", gotUserID, gotID, gotReq)
	}
}

func TestService_Update_Archived(t *testing.T) {
	called := false
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: ownedLookup(archivedCategory(9, 42, "Academia")),
		UpdateFn: func(context.Context, int64, int64, *UpdateCategoryRequest) (*Category, error) {
			called = true
			return archivedCategory(9, 42, "Ginástica"), nil
		},
	})
	if _, err := svc.Update(ctxWithUser(42), 9, validUpdate("Ginástica")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("an archived category must be editable")
	}
}

func TestService_Update_NoUserID(t *testing.T) {
	svc := NewService(&mockCategoryRepository{})
	_, err := svc.Update(context.Background(), 1, validUpdate("X"))
	if !errors.Is(err, apperrors.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized, got %v", err)
	}
}

func TestService_Update_NilRequest(t *testing.T) {
	svc := NewService(&mockCategoryRepository{})
	_, err := svc.Update(ctxWithUser(1), 1, nil)
	if !errors.Is(err, apperrors.ErrValidation) || err.Error() != "name is required" {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestService_Update_Ownership(t *testing.T) {
	tests := []struct {
		name     string
		existing *Category
		want     error
	}{
		{"missing", nil, apperrors.ErrNotFound},
		{"global", globalCategory(1, "Alimentação"), apperrors.ErrForbidden},
		{"other user", personalCategory(5, 99, "Alheia"), apperrors.ErrNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewService(&mockCategoryRepository{
				GetByIDFn: ownedLookup(tc.existing),
				UpdateFn: func(context.Context, int64, int64, *UpdateCategoryRequest) (*Category, error) {
					t.Fatal("repository Update must not be called")
					return nil, nil
				},
			})
			_, err := svc.Update(ctxWithUser(42), 5, validUpdate("Comida"))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestService_Update_LookupError(t *testing.T) {
	repoErr := errors.New("db failure")
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(context.Context, int64) (*Category, error) { return nil, repoErr },
	})
	_, err := svc.Update(ctxWithUser(42), 5, validUpdate("X"))
	if !errors.Is(err, repoErr) {
		t.Fatalf("expected wrapped repoErr, got %v", err)
	}
}

func TestService_Update_ValidationBeforeLookup(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: func(context.Context, int64) (*Category, error) {
			t.Fatal("GetByID must not be called before validation")
			return nil, nil
		},
	})
	_, err := svc.Update(ctxWithUser(42), 5, validUpdate(strings.Repeat("a", 41)))
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Fatalf("expected ErrValidation, got %v", err)
	}
}

func TestService_Update_DefaultName(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn:            ownedLookup(personalCategory(5, 42, "Minha")),
		ExistsGlobalByNameFn: func(context.Context, string) (bool, error) { return true, nil },
	})
	_, err := svc.Update(ctxWithUser(42), 5, validUpdate("ALIMENTACAO"))
	if !errors.Is(err, ErrDefaultNameConflict) {
		t.Fatalf("err = %v, want ErrDefaultNameConflict", err)
	}
}

func conflictRepo(other *Category, gotUserID *int64, gotName *string) *mockCategoryRepository {
	return &mockCategoryRepository{
		GetByIDFn: ownedLookup(personalCategory(5, 42, "Pets")),
		UpdateFn: func(context.Context, int64, int64, *UpdateCategoryRequest) (*Category, error) {
			return nil, apperrors.ErrConflict
		},
		FindOwnByNameFn: func(_ context.Context, userID int64, name string) (*Category, error) {
			*gotUserID, *gotName = userID, name
			return other, nil
		},
	}
}

func TestService_Update_ConflictWithActive(t *testing.T) {
	var gotUserID int64
	var gotName string
	svc := NewService(conflictRepo(personalCategory(6, 42, "Assinaturas"), &gotUserID, &gotName))
	_, err := svc.Update(ctxWithUser(42), 5, validUpdate("  assinaturas "))
	if !errors.Is(err, ErrNameConflict) || errors.Is(err, ErrArchivedNameConflict) {
		t.Fatalf("err = %v, want ErrNameConflict", err)
	}
	if gotUserID != 42 || gotName != "assinaturas" {
		t.Fatalf("FindOwnByName received userID=%d name=%q", gotUserID, gotName)
	}
}

func TestService_Update_ConflictWithArchived(t *testing.T) {
	var gotUserID int64
	var gotName string
	svc := NewService(conflictRepo(archivedCategory(9, 42, "Bar"), &gotUserID, &gotName))
	_, err := svc.Update(ctxWithUser(42), 5, validUpdate("bar"))
	if !errors.Is(err, ErrArchivedNameConflict) {
		t.Fatalf("err = %v, want ErrArchivedNameConflict", err)
	}
}

func TestService_Update_ConflictRowGone(t *testing.T) {
	var gotUserID int64
	var gotName string
	svc := NewService(conflictRepo(nil, &gotUserID, &gotName))
	_, err := svc.Update(ctxWithUser(42), 5, validUpdate("Bar"))
	if !errors.Is(err, ErrNameConflict) {
		t.Fatalf("err = %v, want ErrNameConflict", err)
	}
}

func TestService_Update_NotFoundOnWrite(t *testing.T) {
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: ownedLookup(personalCategory(5, 42, "Pets")),
		UpdateFn: func(context.Context, int64, int64, *UpdateCategoryRequest) (*Category, error) {
			return nil, nil
		},
	})
	_, err := svc.Update(ctxWithUser(42), 5, validUpdate("Animais"))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestService_Update_RepositoryError(t *testing.T) {
	repoErr := errors.New("db failure")
	svc := NewService(&mockCategoryRepository{
		GetByIDFn: ownedLookup(personalCategory(5, 42, "Pets")),
		UpdateFn: func(context.Context, int64, int64, *UpdateCategoryRequest) (*Category, error) {
			return nil, repoErr
		},
	})
	_, err := svc.Update(ctxWithUser(42), 5, validUpdate("Animais"))
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
		existing *Category
		found    bool
		want     error
		setCalls int
	}{
		{"own", ctxWithUser(42), personalCategory(5, 42, "Pets"), true, nil, 1},
		{"own archived", ctxWithUser(42), archivedCategory(5, 42, "Pets"), true, nil, 1},
		{"global", ctxWithUser(42), globalCategory(5, "Lazer"), true, apperrors.ErrForbidden, 0},
		{"other user", ctxWithUser(42), personalCategory(5, 99, "Alheia"), true, apperrors.ErrNotFound, 0},
		{"missing", ctxWithUser(42), nil, true, apperrors.ErrNotFound, 0},
		{"gone on write", ctxWithUser(42), personalCategory(5, 42, "Pets"), false, apperrors.ErrNotFound, 1},
		{"no user", context.Background(), personalCategory(5, 42, "Pets"), true, apperrors.ErrUnauthorized, 0},
	}
	for _, op := range ops {
		for _, tc := range cases {
			t.Run(op.name+"/"+tc.name, func(t *testing.T) {
				var calls int
				svc := NewService(&mockCategoryRepository{
					GetByIDFn: ownedLookup(tc.existing),
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
	svc := NewService(&mockCategoryRepository{
		GetByIDFn:     ownedLookup(personalCategory(5, 42, "Pets")),
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
		repo := &mockCategoryRepository{
			GetByIDFn: ownedLookup(personalCategory(id, userID, "Pets")),
			CreateFn: func(_ context.Context, u int64, _ *CreateCategoryRequest) (*Category, bool, error) {
				check("Create", u)
				return personalCategory(id, u, "Pets"), true, nil
			},
			UpdateFn: func(_ context.Context, u, _ int64, _ *UpdateCategoryRequest) (*Category, error) {
				check("Update", u)
				return personalCategory(id, u, "Pets"), nil
			},
			SetArchivedFn: func(_ context.Context, u, _ int64, _ bool) (bool, error) {
				check("SetArchived", u)
				return true, nil
			},
		}
		svc := NewService(repo)
		ctx := ctxWithUser(userID)

		if _, _, err := svc.Create(ctx, validReq("Pets")); err != nil {
			rt.Fatalf("Create: %v", err)
		}
		if _, err := svc.Update(ctx, id, validUpdate("Pets")); err != nil {
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
