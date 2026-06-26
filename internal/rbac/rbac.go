package rbac

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger/firefly-common/pkg/i18n"
)

// Permission represents a specific permission
type Permission string

// Role represents a user role
type Role string

// Resource represents a resource type
type Resource string

const (
	// Permissions
	PermissionRead   Permission = "read"
	PermissionWrite  Permission = "write"
	PermissionDelete Permission = "delete"
	PermissionAdmin  Permission = "admin"

	// Roles
	RoleAdmin     Role = "admin"
	RoleUser      Role = "user"
	RoleViewer    Role = "viewer"
	RoleOperator  Role = "operator"

	// Resources
	ResourceMessages    Resource = "messages"
	ResourceData       Resource = "data"
	ResourceContracts  Resource = "contracts"
	ResourceIdentities Resource = "identities"
	ResourceTokens     Resource = "tokens"
	ResourceEvents     Resource = "events"
	ResourceNamespaces Resource = "namespaces"

	// Maximum failed attempts before temporary lockout
	maxFailedAttempts = 5
	// Lockout duration
	lockoutDuration = 15 * time.Minute
)

// UserRole represents a user's role with additional metadata
type UserRole struct {
	Role            Role
	AssignedAt      time.Time
	AssignedBy      string
	LastAccess      time.Time
	FailedAttempts  int
	LockoutUntil    time.Time
	IsLocked        bool
}

// RBACManager handles role-based access control
type RBACManager struct {
	mu       sync.RWMutex
	roles    map[Role][]Permission
	userRole map[string]*UserRole
}

// NewRBACManager creates a new RBAC manager
func NewRBACManager() *RBACManager {
	rbac := &RBACManager{
		roles:    make(map[Role][]Permission),
		userRole: make(map[string]*UserRole),
	}

	// Initialize default roles and permissions
	rbac.roles[RoleAdmin] = []Permission{
		PermissionRead,
		PermissionWrite,
		PermissionDelete,
		PermissionAdmin,
	}

	rbac.roles[RoleUser] = []Permission{
		PermissionRead,
		PermissionWrite,
	}

	rbac.roles[RoleViewer] = []Permission{
		PermissionRead,
	}

	rbac.roles[RoleOperator] = []Permission{
		PermissionRead,
		PermissionWrite,
	}

	return rbac
}

// AssignRole assigns a role to a user
func (rbac *RBACManager) AssignRole(userID string, role Role, assignedBy string) error {
	rbac.mu.Lock()
	defer rbac.mu.Unlock()

	if _, exists := rbac.roles[role]; !exists {
		return i18n.NewError(context.Background(), i18n.MsgInvalidRole)
	}

	now := time.Now()
	rbac.userRole[userID] = &UserRole{
		Role:       role,
		AssignedAt: now,
		AssignedBy: assignedBy,
		LastAccess: now,
	}

	return nil
}

// GetRole returns the role assigned to a user
func (rbac *RBACManager) GetRole(userID string) (Role, error) {
	rbac.mu.RLock()
	defer rbac.mu.RUnlock()

	userRole, exists := rbac.userRole[userID]
	if !exists {
		return "", i18n.NewError(context.Background(), i18n.MsgUserNotFound)
	}

	return userRole.Role, nil
}

// CheckPermission checks if a user has permission to perform an action on a resource
func (rbac *RBACManager) CheckPermission(ctx context.Context, userID string, permission Permission, resource Resource) error {
	rbac.mu.Lock()
	defer rbac.mu.Unlock()

	userRole, exists := rbac.userRole[userID]
	if !exists {
		return i18n.NewError(ctx, i18n.MsgUserNotFound)
	}

	// Check if user is locked out
	if userRole.IsLocked {
		if time.Now().Before(userRole.LockoutUntil) {
			return i18n.NewError(ctx, i18n.MsgUserLocked, userRole.LockoutUntil.Sub(time.Now()))
		}
		// Reset lockout if duration has passed
		userRole.IsLocked = false
		userRole.FailedAttempts = 0
	}

	permissions, exists := rbac.roles[userRole.Role]
	if !exists {
		return i18n.NewError(ctx, i18n.MsgInvalidRole)
	}

	// Admin role has all permissions
	if userRole.Role == RoleAdmin {
		userRole.LastAccess = time.Now()
		return nil
	}

	// Check if user has the required permission
	for _, p := range permissions {
		if p == permission {
			userRole.LastAccess = time.Now()
			return nil
		}
	}

	// Increment failed attempts
	userRole.FailedAttempts++
	if userRole.FailedAttempts >= maxFailedAttempts {
		userRole.IsLocked = true
		userRole.LockoutUntil = time.Now().Add(lockoutDuration)
		return i18n.NewError(ctx, i18n.MsgUserLocked, lockoutDuration)
	}

	return i18n.NewError(ctx, i18n.MsgPermissionDenied)
}

// AddRole adds a new role with specified permissions
func (rbac *RBACManager) AddRole(role Role, permissions []Permission) error {
	rbac.mu.Lock()
	defer rbac.mu.Unlock()

	if _, exists := rbac.roles[role]; exists {
		return i18n.NewError(context.Background(), i18n.MsgRoleExists)
	}

	rbac.roles[role] = permissions
	return nil
}

// UpdateRole updates permissions for an existing role
func (rbac *RBACManager) UpdateRole(role Role, permissions []Permission) error {
	rbac.mu.Lock()
	defer rbac.mu.Unlock()

	if _, exists := rbac.roles[role]; !exists {
		return i18n.NewError(context.Background(), i18n.MsgRoleNotFound)
	}

	rbac.roles[role] = permissions
	return nil
}

// RemoveRole removes a role
func (rbac *RBACManager) RemoveRole(role Role) error {
	rbac.mu.Lock()
	defer rbac.mu.Unlock()

	if _, exists := rbac.roles[role]; !exists {
		return i18n.NewError(context.Background(), i18n.MsgRoleNotFound)
	}

	// Check if any users have this role
	for _, userRole := range rbac.userRole {
		if userRole.Role == role {
			return i18n.NewError(context.Background(), i18n.MsgRoleInUse)
		}
	}

	delete(rbac.roles, role)
	return nil
}

// GetUserPermissions returns all permissions for a user
func (rbac *RBACManager) GetUserPermissions(userID string) ([]Permission, error) {
	rbac.mu.RLock()
	defer rbac.mu.RUnlock()

	userRole, exists := rbac.userRole[userID]
	if !exists {
		return nil, i18n.NewError(context.Background(), i18n.MsgUserNotFound)
	}

	permissions, exists := rbac.roles[userRole.Role]
	if !exists {
		return nil, i18n.NewError(context.Background(), i18n.MsgInvalidRole)
	}

	return permissions, nil
}

// ResetFailedAttempts resets the failed attempts counter for a user
func (rbac *RBACManager) ResetFailedAttempts(userID string) error {
	rbac.mu.Lock()
	defer rbac.mu.Unlock()

	userRole, exists := rbac.userRole[userID]
	if !exists {
		return i18n.NewError(context.Background(), i18n.MsgUserNotFound)
	}

	userRole.FailedAttempts = 0
	userRole.IsLocked = false
	return nil
}

// GetUserRoleInfo returns detailed information about a user's role
func (rbac *RBACManager) GetUserRoleInfo(userID string) (*UserRole, error) {
	rbac.mu.RLock()
	defer rbac.mu.RUnlock()

	userRole, exists := rbac.userRole[userID]
	if !exists {
		return nil, i18n.NewError(context.Background(), i18n.MsgUserNotFound)
	}

	return userRole, nil
} 