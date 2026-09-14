package schema

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToSnakeCase_R5_6b_MandatoryTable(t *testing.T) {
	cases := []struct {
		field  string
		column string
	}{
		{"Email", "email"},
		{"CreatedAt", "created_at"},
		{"ID", "id"},
		{"UserID", "user_id"},
		{"APIKey", "api_key"},
		{"HTTPServer", "http_server"},
		{"OAuthToken", "o_auth_token"},
		{"Line1", "line1"},
		{"IPv4Address", "i_pv4_address"},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			require.Equal(t, tc.column, toSnakeCase(tc.field))
		})
	}
}

func TestToSnakeCase_SupplementaryCases(t *testing.T) {
	cases := []struct {
		name   string
		field  string
		column string
	}{
		{"empty string", "", ""},
		{"single lowercase word", "age", "age"},
		{"single uppercase letter", "X", "x"},
		{"trailing acronym", "UserAPI", "user_api"},
		{"all caps", "ID", "id"},
		{"leading digit run", "Item2Count", "item2_count"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.column, toSnakeCase(tc.field))
		})
	}
}
