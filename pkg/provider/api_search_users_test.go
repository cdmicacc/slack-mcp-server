package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// searchUsersClient implements just enough of SlackAPI for SearchUsers tests,
// and records which lookup each query reached.
type searchUsersClient struct {
	SlackAPI // embed interface to satisfy all methods; only override what we need

	byEmail    *slack.User
	byEmailErr error

	emailQueries  []string
	idQueries     []string
	searchQueries []string
}

func (m *searchUsersClient) GetUsersInfo(users ...string) (*[]slack.User, error) {
	m.idQueries = append(m.idQueries, users...)
	return &[]slack.User{{ID: users[0]}}, nil
}

func (m *searchUsersClient) GetUserByEmailContext(ctx context.Context, email string) (*slack.User, error) {
	m.emailQueries = append(m.emailQueries, email)
	return m.byEmail, m.byEmailErr
}

func (m *searchUsersClient) UsersSearch(ctx context.Context, query string, count int) ([]slack.User, error) {
	m.searchQueries = append(m.searchQueries, query)
	return nil, nil
}

// TestUnitSearchUsersByEmail verifies that an email query is a direct
// users.lookupByEmail call, which needs no users cache and so also works
// under --no-cache.
func TestUnitSearchUsersByEmail(t *testing.T) {
	t.Run("an email query is looked up directly", func(t *testing.T) {
		client := &searchUsersClient{byEmail: &slack.User{ID: "U001", Name: "alice"}}
		ap := newTestApiProvider(client, &UsersCache{})

		users, err := ap.SearchUsers(context.Background(), "alice@example.com", 10)
		require.NoError(t, err)
		require.Len(t, users, 1)
		assert.Equal(t, "U001", users[0].ID)
		assert.Equal(t, []string{"alice@example.com"}, client.emailQueries)
		assert.Empty(t, client.searchQueries)
	})

	t.Run("users_not_found falls through to the search", func(t *testing.T) {
		// A name or display name can look like an email; the search still
		// finds it, so the result is never less than before the lookup existed.
		client := &searchUsersClient{byEmailErr: slack.SlackErrorResponse{Err: "users_not_found"}}
		ap := newTestApiProvider(client, &UsersCache{})

		_, err := ap.SearchUsers(context.Background(), "dev@home.lol", 10)
		require.NoError(t, err)
		assert.Equal(t, []string{"dev@home.lol"}, client.emailQueries)
		assert.Equal(t, []string{"dev@home.lol"}, client.searchQueries)
	})

	t.Run("a nil user with no error falls through to the search", func(t *testing.T) {
		client := &searchUsersClient{}
		ap := newTestApiProvider(client, &UsersCache{})

		_, err := ap.SearchUsers(context.Background(), "alice@example.com", 10)
		require.NoError(t, err)
		assert.Equal(t, []string{"alice@example.com"}, client.searchQueries)
	})

	t.Run("any other error is returned", func(t *testing.T) {
		client := &searchUsersClient{byEmailErr: slack.SlackErrorResponse{Err: "missing_scope"}}
		ap := newTestApiProvider(client, &UsersCache{})

		users, err := ap.SearchUsers(context.Background(), "alice@example.com", 10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing_scope")
		assert.Nil(t, users)
		assert.Empty(t, client.searchQueries)
	})

	t.Run("a transport error is returned", func(t *testing.T) {
		client := &searchUsersClient{byEmailErr: errors.New("connection reset")}
		ap := newTestApiProvider(client, &UsersCache{})

		_, err := ap.SearchUsers(context.Background(), "alice@example.com", 10)
		assert.Error(t, err)
	})

	t.Run("a query that is not a single complete email is searched, not looked up", func(t *testing.T) {
		for _, query := range []string{"alice", "alice@", "alice @example.com", "email alice@example.com", "@example.com"} {
			client := &searchUsersClient{}
			ap := newTestApiProvider(client, &UsersCache{})

			_, err := ap.SearchUsers(context.Background(), query, 10)
			require.NoError(t, err, query)
			assert.Empty(t, client.emailQueries, query)
			assert.Equal(t, []string{query}, client.searchQueries, query)
		}
	})

	t.Run("a user ID is still looked up by ID", func(t *testing.T) {
		client := &searchUsersClient{}
		ap := newTestApiProvider(client, &UsersCache{})

		users, err := ap.SearchUsers(context.Background(), "U07VCEPP4N5", 10)
		require.NoError(t, err)
		require.Len(t, users, 1)
		assert.Equal(t, []string{"U07VCEPP4N5"}, client.idQueries)
		assert.Empty(t, client.emailQueries)
	})
}
