package server

import (
	"net/http"

	"arca/internal/auth"
	"arca/internal/store/models"
)

type managedUser struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	Disabled  int    `json:"disabled"`
	CreatedAt int64  `json:"created_at"`
	// SSO marks an account linked to the configured identity provider.
	SSO bool `json:"sso"`
	// RoleManaged marks a linked account whose role the provider's admin groups
	// set at every sign-in, so it cannot be changed here.
	RoleManaged bool `json:"role_managed"`
}

func toManagedUser(user models.User) managedUser {
	disabled := 0
	if user.Disabled {
		disabled = 1
	}
	return managedUser{
		ID:        user.ID,
		Email:     user.Email,
		Name:      user.Name,
		Role:      user.Role,
		Disabled:  disabled,
		CreatedAt: user.CreatedAt,
	}
}

var roles = []string{models.RoleAdmin, models.RoleUser}

func (s *Server) routeListUsers(writer http.ResponseWriter, request *http.Request) error {
	if _, err := requireAdmin(request); err != nil {
		return err
	}

	users, err := s.store.ListUsers()
	if err != nil {
		return err
	}

	linked := map[string]bool{}
	if s.oidc != nil {
		ids, err := s.store.IdentityUserIDs(s.config.OIDC.Issuer)
		if err != nil {
			return err
		}
		for _, id := range ids {
			linked[id] = true
		}
	}

	responses := make([]managedUser, 0, len(users))
	for _, user := range users {
		response := toManagedUser(user)
		response.SSO = linked[user.ID]
		response.RoleManaged = response.SSO && len(s.config.OIDC.AdminGroups) > 0
		responses = append(responses, response)
	}

	writeJSON(writer, http.StatusOK, map[string]any{"users": responses})
	return nil
}

func (s *Server) routeCreateUser(writer http.ResponseWriter, request *http.Request) error {
	if _, err := requireAdmin(request); err != nil {
		return err
	}

	var body struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := s.decodeBody(request, &body); err != nil {
		return err
	}

	email, err := validateEmail(body.Email)
	if err != nil {
		return err
	}
	name, err := requireText(body.Name, "name", 120)
	if err != nil {
		return err
	}
	password, err := validatePassword(body.Password)
	if err != nil {
		return err
	}

	role := body.Role
	if role == "" {
		role = models.RoleUser
	}
	if role, err = requireOneOf(role, roles, "role"); err != nil {
		return err
	}

	taken, err := s.store.EmailTaken(email)
	if err != nil {
		return err
	}
	if taken {
		return fail(http.StatusConflict, "A user with that email already exists")
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	user := &models.User{Email: email, Name: name, PasswordHash: hash, Role: role}
	if err := s.store.CreateUser(user); err != nil {
		return err
	}

	writeJSON(writer, http.StatusCreated, toManagedUser(*user))
	return nil
}

func (s *Server) routeUpdateUser(writer http.ResponseWriter, request *http.Request) error {
	administrator, err := requireAdmin(request)
	if err != nil {
		return err
	}

	var body struct {
		Name     *string `json:"name"`
		Password *string `json:"password"`
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
	}
	if err := s.decodeBody(request, &body); err != nil {
		return err
	}

	id := routeVar(request, "id")
	changes := map[string]any{}

	if body.Name != nil {
		name, err := requireText(*body.Name, "name", 120)
		if err != nil {
			return err
		}
		changes["name"] = name
	}
	if body.Role != nil {
		role, err := requireOneOf(*body.Role, roles, "role")
		if err != nil {
			return err
		}
		// With admin groups configured the next sign-in through the provider
		// would undo the change, so it is refused up front.
		if len(s.config.OIDC.AdminGroups) > 0 {
			linked, err := s.store.HasIdentity(id, s.config.OIDC.Issuer)
			if err != nil {
				return err
			}
			if linked {
				return fail(http.StatusConflict, "This user's role is set by the identity provider's groups")
			}
		}
		changes["role"] = role
	}
	if body.Disabled != nil {
		changes["disabled"] = *body.Disabled
	}
	if body.Password != nil {
		password, err := validatePassword(*body.Password)
		if err != nil {
			return err
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			return err
		}
		changes["password_hash"] = hash
	}

	demoting := body.Role != nil && *body.Role == models.RoleUser
	disabling := body.Disabled != nil && *body.Disabled
	if id == administrator.ID && (demoting || disabling) {
		return fail(http.StatusBadRequest, "You cannot remove your own administrator access")
	}
	if len(changes) == 0 {
		return fail(http.StatusBadRequest, "No changes supplied")
	}

	updated, err := s.store.UpdateUser(id, changes)
	if err != nil {
		return err
	}
	if updated == 0 {
		return fail(http.StatusNotFound, "User not found")
	}

	if body.Password != nil || disabling {
		if err := s.store.DeleteUserSessions(id); err != nil {
			return err
		}
	}

	return noContent(writer)
}

func (s *Server) routeDeleteUser(writer http.ResponseWriter, request *http.Request) error {
	administrator, err := requireAdmin(request)
	if err != nil {
		return err
	}

	id := routeVar(request, "id")
	if id == administrator.ID {
		return fail(http.StatusBadRequest, "You cannot delete your own account")
	}

	deleted, err := s.store.DeleteUser(id)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return fail(http.StatusNotFound, "User not found")
	}

	return noContent(writer)
}
