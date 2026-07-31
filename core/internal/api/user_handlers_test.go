package api

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/PrincepsVIIII/Espial/core/internal/auth"
)

type fakeUserAdministrator struct {
	users         auth.ManagedUserList
	roles         []auth.RoleView
	err           error
	created       auth.CreateManagedUser
	updated       auth.UpdateManagedUser
	passwordReset auth.ResetManagedPassword
}

func (fake *fakeUserAdministrator) ManagedUsers(context.Context, auth.ManagedUserFilter) (auth.ManagedUserList, error) {
	return fake.users, fake.err
}

func (fake *fakeUserAdministrator) ManagedRoles(context.Context) ([]auth.RoleView, error) {
	return fake.roles, fake.err
}

func (fake *fakeUserAdministrator) CreateManagedUser(_ context.Context, input auth.CreateManagedUser) (auth.ManagedUser, error) {
	fake.created = input
	if fake.err != nil {
		return auth.ManagedUser{}, fake.err
	}
	return fake.users.Items[0], nil
}

func (fake *fakeUserAdministrator) UpdateManagedUser(_ context.Context, input auth.UpdateManagedUser) (auth.ManagedUser, error) {
	fake.updated = input
	if fake.err != nil {
		return auth.ManagedUser{}, fake.err
	}
	return fake.users.Items[0], nil
}

func (fake *fakeUserAdministrator) ResetManagedUserPassword(_ context.Context, input auth.ResetManagedPassword) error {
	fake.passwordReset = input
	return fake.err
}

func userAdministrationHandler(authService AuthService, users UserAdministrator) http.Handler {
	publicURL, _ := url.Parse("https://espial.test")
	return New(Dependencies{
		Logger: discardLogger(), Ready: func(context.Context) error { return nil },
		Auth: authService, Users: users, PublicURL: publicURL, SecureCookies: true,
	})
}

func TestUserAdministrationIsDiscoverablePermissionGatedAndAudited(t *testing.T) {
	updatedAt := time.Date(2026, 7, 31, 12, 30, 45, 123456000, time.UTC)
	targetID := "70000000-0000-4000-8000-000000000002"
	administratorID := "70000000-0000-4000-8000-000000000001"
	users := &fakeUserAdministrator{
		users: auth.ManagedUserList{Items: []auth.ManagedUser{{
			ID: targetID, Username: "operator", DisplayName: "Operator", Enabled: true,
			Roles: []string{"operator"}, IdentityProvider: "local", UpdatedAt: updatedAt,
		}}},
		roles: []auth.RoleView{{Name: "operator", Permissions: []string{"resources:read"}}},
	}
	administrator := &fakeAuth{session: auth.Session{CSRFDigest: []byte("digest"), User: auth.User{
		ID: administratorID, Username: "admin", Permissions: []string{"users:manage"},
	}}}
	handler := userAdministrationHandler(administrator, users)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/users?limit=20", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"username":"operator"`) || strings.Contains(response.Body.String(), "password") {
		t.Fatalf("managed user list = %d %s", response.Code, response.Body.String())
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/roles", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"name":"operator"`) {
		t.Fatalf("managed roles = %d %s", response.Code, response.Body.String())
	}

	create := authenticatedRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(`{
		"username":"operator","display_name":"Operator","email":"operator@example.test",
		"role":"operator","password":"a long replacement password"
	}`))
	addMutationHeaders(create)
	create.Header.Set("X-Request-ID", "user-create-receipt")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, create)
	if response.Code != http.StatusCreated || response.Header().Get("X-Request-ID") != "user-create-receipt" || response.Header().Get("ETag") != `"2026-07-31T12:30:45.123456Z"` {
		t.Fatalf("create = %d headers=%v body=%s", response.Code, response.Header(), response.Body.String())
	}
	if users.created.Context.ActorUserID != administratorID || users.created.Context.CorrelationID != "user-create-receipt" || users.created.Password == "" {
		t.Fatalf("create audit context = %#v", users.created.Context)
	}

	update := authenticatedRequest(http.MethodPut, "/api/v1/users/"+targetID, bytes.NewBufferString(`{
		"display_name":"Renamed Operator","email":"","role":"viewer","enabled":true
	}`))
	addMutationHeaders(update)
	update.Header.Set("If-Match", `"2026-07-31T12:30:45.123456Z"`)
	update.Header.Set("X-Request-ID", "user-update-receipt")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, update)
	if response.Code != http.StatusOK || !users.updated.ExpectedUpdatedAt.Equal(updatedAt) || users.updated.Context.CorrelationID != "user-update-receipt" {
		t.Fatalf("update = %d input=%#v body=%s", response.Code, users.updated, response.Body.String())
	}

	reset := authenticatedRequest(http.MethodPost, "/api/v1/users/"+targetID+"/password", bytes.NewBufferString(`{
		"password":"a different long password"
	}`))
	addMutationHeaders(reset)
	reset.Header.Set("X-Request-ID", "password-receipt")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, reset)
	if response.Code != http.StatusNoContent || users.passwordReset.Context.CorrelationID != "password-receipt" || users.passwordReset.Context.ActorUserID != administratorID {
		t.Fatalf("password reset = %d input=%#v body=%s", response.Code, users.passwordReset, response.Body.String())
	}

	administrator.session.User.Permissions = []string{"audit:read"}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/api/v1/users", nil))
	if response.Code != http.StatusForbidden || !administrator.denied {
		t.Fatalf("permission boundary = %d audited=%v", response.Code, administrator.denied)
	}
}

func TestUserAdministrationMapsSafetyAndConcurrencyFailures(t *testing.T) {
	userID := "70000000-0000-4000-8000-000000000001"
	users := &fakeUserAdministrator{err: auth.ErrSelfLockout}
	administrator := &fakeAuth{session: auth.Session{CSRFDigest: []byte("digest"), User: auth.User{
		ID: userID, Permissions: []string{"users:manage"},
	}}}
	handler := userAdministrationHandler(administrator, users)
	body := `{"display_name":"Admin","email":"","role":"viewer","enabled":true}`

	for _, test := range []struct {
		name       string
		ifMatch    string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "missing precondition", wantStatus: http.StatusPreconditionRequired, wantCode: "precondition_required"},
		{name: "stale user", ifMatch: `"2026-07-31T12:00:00Z"`, err: auth.ErrUserChanged, wantStatus: http.StatusPreconditionFailed, wantCode: "precondition_failed"},
		{name: "self lockout", ifMatch: `"2026-07-31T12:00:00Z"`, err: auth.ErrSelfLockout, wantStatus: http.StatusConflict, wantCode: "self_lockout"},
		{name: "last administrator", ifMatch: `"2026-07-31T12:00:00Z"`, err: auth.ErrLastAdministrator, wantStatus: http.StatusConflict, wantCode: "last_administrator"},
	} {
		t.Run(test.name, func(t *testing.T) {
			users.err = test.err
			request := authenticatedRequest(http.MethodPut, "/api/v1/users/"+userID, bytes.NewBufferString(body))
			addMutationHeaders(request)
			if test.ifMatch != "" {
				request.Header.Set("If-Match", test.ifMatch)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus || !strings.Contains(response.Body.String(), test.wantCode) {
				t.Fatalf("response = %d %s", response.Code, response.Body.String())
			}
		})
	}
}
