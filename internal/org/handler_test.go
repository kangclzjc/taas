package org

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/taas-platform/taas/internal/auth"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------------------------------------------------------------------------
// Mock OrgRepository
// ---------------------------------------------------------------------------

type mockOrgRepo struct {
	orgs    map[uuid.UUID]*Organization
	members map[uuid.UUID][]*OrgMember // orgID -> members
}

func newMockOrgRepo() *mockOrgRepo {
	return &mockOrgRepo{
		orgs:    make(map[uuid.UUID]*Organization),
		members: make(map[uuid.UUID][]*OrgMember),
	}
}

func (m *mockOrgRepo) Create(_ context.Context, org *Organization) error {
	if org.ID == uuid.Nil {
		org.ID = uuid.New()
	}
	now := time.Now().UTC()
	org.CreatedAt = now
	org.UpdatedAt = now
	if org.SLATier == "" {
		org.SLATier = "standard"
	}
	m.orgs[org.ID] = org
	return nil
}

func (m *mockOrgRepo) GetByID(_ context.Context, id uuid.UUID) (*Organization, error) {
	o, ok := m.orgs[id]
	if !ok {
		return nil, nil
	}
	return o, nil
}

func (m *mockOrgRepo) GetBySlug(_ context.Context, slug string) (*Organization, error) {
	for _, o := range m.orgs {
		if o.Slug == slug {
			return o, nil
		}
	}
	return nil, nil
}

func (m *mockOrgRepo) Update(_ context.Context, org *Organization) error {
	if _, ok := m.orgs[org.ID]; !ok {
		return fmt.Errorf("organization not found")
	}
	org.UpdatedAt = time.Now().UTC()
	m.orgs[org.ID] = org
	return nil
}

func (m *mockOrgRepo) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := m.orgs[id]; !ok {
		return fmt.Errorf("organization not found")
	}
	delete(m.orgs, id)
	delete(m.members, id)
	return nil
}

func (m *mockOrgRepo) ListByUser(_ context.Context, userID uuid.UUID) ([]*Organization, error) {
	var result []*Organization
	for orgID, members := range m.members {
		for _, mem := range members {
			if mem.UserID == userID {
				if o, ok := m.orgs[orgID]; ok {
					result = append(result, o)
				}
				break
			}
		}
	}
	return result, nil
}

func (m *mockOrgRepo) ListMembers(_ context.Context, orgID uuid.UUID) ([]*OrgMember, error) {
	return m.members[orgID], nil
}

func (m *mockOrgRepo) AddMember(_ context.Context, member *OrgMember) error {
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now().UTC()
	}
	if member.Role == "" {
		member.Role = "member"
	}
	m.members[member.OrgID] = append(m.members[member.OrgID], member)
	return nil
}

func (m *mockOrgRepo) RemoveMember(_ context.Context, orgID, userID uuid.UUID) error {
	members := m.members[orgID]
	for i, mem := range members {
		if mem.UserID == userID {
			m.members[orgID] = append(members[:i], members[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("member not found")
}

func (m *mockOrgRepo) UpdateMemberRole(_ context.Context, orgID, userID uuid.UUID, role string) error {
	for _, mem := range m.members[orgID] {
		if mem.UserID == userID {
			mem.Role = role
			return nil
		}
	}
	return fmt.Errorf("member not found")
}

// ---------------------------------------------------------------------------
// Mock UserRepository (for AddMember email lookup)
// ---------------------------------------------------------------------------

type mockUserRepo struct {
	users map[string]*auth.User
}

func newMockUserRepo() *mockUserRepo {
	return &mockUserRepo{users: make(map[string]*auth.User)}
}

func (m *mockUserRepo) CreateUser(_ context.Context, email, password, role string, orgID uuid.UUID) (*auth.User, error) {
	u := &auth.User{
		ID:    uuid.New(),
		OrgID: orgID,
		Email: email,
		Role:  role,
	}
	m.users[email] = u
	return u, nil
}

func (m *mockUserRepo) GetUserByEmail(_ context.Context, email string) (*auth.User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, nil
	}
	return u, nil
}

func (m *mockUserRepo) GetUserByID(_ context.Context, id uuid.UUID) (*auth.User, error) {
	for _, u := range m.users {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func setupRouter(orgRepo *mockOrgRepo, userRepo *mockUserRepo) *gin.Engine {
	logger := zap.NewNop()
	handler := NewHandler(orgRepo, userRepo, logger)

	r := gin.New()
	// Simulate authenticated user context
	r.Use(func(c *gin.Context) {
		c.Set("user_id", c.GetHeader("X-Test-UserID"))
		c.Next()
	})
	handler.RegisterRoutes(r.Group("/organizations"))
	return r
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCreateOrg(t *testing.T) {
	orgRepo := newMockOrgRepo()
	userRepo := newMockUserRepo()
	r := setupRouter(orgRepo, userRepo)

	userID := uuid.New().String()
	body := `{"slug":"my-org","display_name":"My Org"}`
	req := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-UserID", userID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp Organization
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Slug != "my-org" {
		t.Errorf("expected slug 'my-org', got '%s'", resp.Slug)
	}
	if resp.DisplayName != "My Org" {
		t.Errorf("expected display_name 'My Org', got '%s'", resp.DisplayName)
	}
	if resp.SLATier != "standard" {
		t.Errorf("expected sla_tier 'standard', got '%s'", resp.SLATier)
	}
}

func TestCreateOrgDuplicateSlug(t *testing.T) {
	orgRepo := newMockOrgRepo()
	userRepo := newMockUserRepo()
	r := setupRouter(orgRepo, userRepo)

	userID := uuid.New().String()
	body := `{"slug":"dup-org","display_name":"Org 1"}`
	req := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-UserID", userID)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("first create expected 201, got %d", w.Code)
	}

	// Try creating with the same slug
	body2 := `{"slug":"dup-org","display_name":"Org 2"}`
	req2 := httptest.NewRequest(http.MethodPost, "/organizations", bytes.NewBufferString(body2))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Test-UserID", userID)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusConflict {
		t.Fatalf("duplicate slug expected 409, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestGetOrg(t *testing.T) {
	orgRepo := newMockOrgRepo()
	userRepo := newMockUserRepo()
	r := setupRouter(orgRepo, userRepo)

	// Create an org first
	orgID := uuid.New()
	orgRepo.orgs[orgID] = &Organization{
		ID:          orgID,
		Slug:        "test-org",
		DisplayName: "Test Org",
		SLATier:     "standard",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID.String(), nil)
	req.Header.Set("X-Test-UserID", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp Organization
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Slug != "test-org" {
		t.Errorf("expected slug 'test-org', got '%s'", resp.Slug)
	}
}

func TestGetOrgNotFound(t *testing.T) {
	orgRepo := newMockOrgRepo()
	userRepo := newMockUserRepo()
	r := setupRouter(orgRepo, userRepo)

	req := httptest.NewRequest(http.MethodGet, "/organizations/"+uuid.New().String(), nil)
	req.Header.Set("X-Test-UserID", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpdateOrg(t *testing.T) {
	orgRepo := newMockOrgRepo()
	userRepo := newMockUserRepo()
	r := setupRouter(orgRepo, userRepo)

	orgID := uuid.New()
	orgRepo.orgs[orgID] = &Organization{
		ID:          orgID,
		Slug:        "old-slug",
		DisplayName: "Old Name",
		SLATier:     "standard",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	body := `{"display_name":"New Name","sla_tier":"premium"}`
	req := httptest.NewRequest(http.MethodPut, "/organizations/"+orgID.String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-UserID", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp Organization
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.DisplayName != "New Name" {
		t.Errorf("expected display_name 'New Name', got '%s'", resp.DisplayName)
	}
	if resp.SLATier != "premium" {
		t.Errorf("expected sla_tier 'premium', got '%s'", resp.SLATier)
	}
}

func TestDeleteOrg(t *testing.T) {
	orgRepo := newMockOrgRepo()
	userRepo := newMockUserRepo()
	r := setupRouter(orgRepo, userRepo)

	orgID := uuid.New()
	orgRepo.orgs[orgID] = &Organization{
		ID:          orgID,
		Slug:        "to-delete",
		DisplayName: "Delete Me",
		SLATier:     "standard",
	}

	req := httptest.NewRequest(http.MethodDelete, "/organizations/"+orgID.String(), nil)
	req.Header.Set("X-Test-UserID", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if _, ok := orgRepo.orgs[orgID]; ok {
		t.Error("expected org to be deleted from repo")
	}
}

func TestListOrgs(t *testing.T) {
	orgRepo := newMockOrgRepo()
	userRepo := newMockUserRepo()
	r := setupRouter(orgRepo, userRepo)

	userID := uuid.New()
	orgID := uuid.New()
	orgRepo.orgs[orgID] = &Organization{
		ID:          orgID,
		Slug:        "user-org",
		DisplayName: "User Org",
		SLATier:     "standard",
	}
	orgRepo.members[orgID] = []*OrgMember{
		{OrgID: orgID, UserID: userID, Role: "owner", JoinedAt: time.Now()},
	}

	req := httptest.NewRequest(http.MethodGet, "/organizations", nil)
	req.Header.Set("X-Test-UserID", userID.String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	var orgs []Organization
	if err := json.Unmarshal(resp["organizations"], &orgs); err != nil {
		t.Fatalf("failed to parse organizations: %v", err)
	}
	if len(orgs) != 1 {
		t.Errorf("expected 1 org, got %d", len(orgs))
	}
}

func TestAddAndListMembers(t *testing.T) {
	orgRepo := newMockOrgRepo()
	userRepo := newMockUserRepo()
	r := setupRouter(orgRepo, userRepo)

	// Create the org
	orgID := uuid.New()
	orgRepo.orgs[orgID] = &Organization{
		ID:          orgID,
		Slug:        "mem-org",
		DisplayName: "Member Org",
		SLATier:     "standard",
	}

	// Create a user to add
	testUser := &auth.User{
		ID:    uuid.New(),
		Email: "alice@example.com",
		Role:  "member",
	}
	userRepo.users[testUser.Email] = testUser

	// Add member
	body := `{"email":"alice@example.com","role":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID.String()+"/members", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-UserID", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("add member expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// List members
	req2 := httptest.NewRequest(http.MethodGet, "/organizations/"+orgID.String()+"/members", nil)
	req2.Header.Set("X-Test-UserID", uuid.New().String())
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("list members expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.Unmarshal(w2.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	var members []OrgMember
	if err := json.Unmarshal(resp["members"], &members); err != nil {
		t.Fatalf("failed to parse members: %v", err)
	}
	if len(members) != 1 {
		t.Errorf("expected 1 member, got %d", len(members))
	}
	if members[0].Role != "admin" {
		t.Errorf("expected role 'admin', got '%s'", members[0].Role)
	}
}

func TestRemoveMember(t *testing.T) {
	orgRepo := newMockOrgRepo()
	userRepo := newMockUserRepo()
	r := setupRouter(orgRepo, userRepo)

	orgID := uuid.New()
	orgRepo.orgs[orgID] = &Organization{
		ID:          orgID,
		Slug:        "rm-org",
		DisplayName: "Remove Org",
	}

	memberUserID := uuid.New()
	orgRepo.members[orgID] = []*OrgMember{
		{OrgID: orgID, UserID: memberUserID, Role: "member", JoinedAt: time.Now()},
	}

	req := httptest.NewRequest(http.MethodDelete, "/organizations/"+orgID.String()+"/members/"+memberUserID.String(), nil)
	req.Header.Set("X-Test-UserID", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("remove member expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if len(orgRepo.members[orgID]) != 0 {
		t.Error("expected member to be removed")
	}
}

func TestAddMemberUserNotFound(t *testing.T) {
	orgRepo := newMockOrgRepo()
	userRepo := newMockUserRepo()
	r := setupRouter(orgRepo, userRepo)

	orgID := uuid.New()
	orgRepo.orgs[orgID] = &Organization{
		ID:          orgID,
		Slug:        "no-user-org",
		DisplayName: "No User Org",
	}

	body := `{"email":"nonexistent@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/organizations/"+orgID.String()+"/members", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test-UserID", uuid.New().String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
