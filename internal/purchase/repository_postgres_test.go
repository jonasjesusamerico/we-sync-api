package purchase

import (
	"reflect"
	"testing"
	"time"
)

func TestListWhere(t *testing.T) {
	date := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

	tests := []struct {
		name      string
		filter    ListFilter
		wantWhere string
		wantArgs  []any
	}{
		{
			name:      "user only",
			filter:    ListFilter{UserID: 7},
			wantWhere: "WHERE pe.user_id = $1",
			wantArgs:  []any{int64(7)},
		},
		{
			name:      "month and year become a date range",
			filter:    ListFilter{UserID: 7, Month: 2, Year: 2024},
			wantWhere: "WHERE pe.user_id = $1 AND pe.purchase_date >= $2 AND pe.purchase_date < $3",
			wantArgs:  []any{int64(7), date(2024, time.February, 1), date(2024, time.March, 1)},
		},
		{
			name:      "december rolls over to next year",
			filter:    ListFilter{UserID: 7, Month: 12, Year: 2025},
			wantWhere: "WHERE pe.user_id = $1 AND pe.purchase_date >= $2 AND pe.purchase_date < $3",
			wantArgs:  []any{int64(7), date(2025, time.December, 1), date(2026, time.January, 1)},
		},
		{
			name:      "year only",
			filter:    ListFilter{UserID: 7, Year: 2025},
			wantWhere: "WHERE pe.user_id = $1 AND pe.purchase_date >= $2 AND pe.purchase_date < $3",
			wantArgs:  []any{int64(7), date(2025, time.January, 1), date(2026, time.January, 1)},
		},
		{
			name:      "month only matches any year",
			filter:    ListFilter{UserID: 7, Month: 5},
			wantWhere: "WHERE pe.user_id = $1 AND EXTRACT(MONTH FROM pe.purchase_date) = $2",
			wantArgs:  []any{int64(7), 5},
		},
		{
			name: "all filters keep placeholder order",
			filter: ListFilter{
				UserID: 7, Month: 1, Year: 2026, Status: StatusPaid, CategoryID: 3, PaymentMethodID: 4,
			},
			wantWhere: "WHERE pe.user_id = $1 AND pe.purchase_date >= $2 AND pe.purchase_date < $3" +
				" AND pe.status = $4 AND pe.category_id = $5 AND pe.payment_method_id = $6",
			wantArgs: []any{int64(7), date(2026, time.January, 1), date(2026, time.February, 1), "PAID", int64(3), int64(4)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			where, args := listWhere(tt.filter)
			if where != tt.wantWhere {
				t.Errorf("where = %q, want %q", where, tt.wantWhere)
			}
			if !reflect.DeepEqual(args, tt.wantArgs) {
				t.Errorf("args = %#v, want %#v", args, tt.wantArgs)
			}
		})
	}
}
