package audit

import (
	"net/http"

	"github.com/essensys-hub/essensys-server-backend/internal/middleware"
	"github.com/essensys-hub/essensys-server-backend/internal/models"
)

const CharterVersion = "2026-07-034-v1"

// Authorizer enforces LAN RBAC for audit read APIs.
type Authorizer struct {
	charter *CharterRepository
}

func NewAuthorizer(charter *CharterRepository) *Authorizer {
	return &Authorizer{charter: charter}
}

func (a *Authorizer) CanReadAudit(role string) bool {
	switch role {
	case models.LanRoleAdmin, models.LanRoleUser:
		return true
	default:
		return false
	}
}

func (a *Authorizer) CanAdminAudit(role string) bool {
	return role == models.LanRoleAdmin
}

func (a *Authorizer) HasAcceptedCharter(userID int) (bool, error) {
	if a.charter == nil {
		return false, nil
	}
	return a.charter.HasAccepted(userID, CharterVersion)
}

func UserFromRequest(r *http.Request) (*models.LanUser, bool) {
	return middleware.GetLanUser(r)
}

// ActorIDFromRequest returns the LAN IAM user email when present, otherwise fallback (e.g. firmware client_id).
func ActorIDFromRequest(r *http.Request, fallback string) string {
	if u, ok := UserFromRequest(r); ok && u.Email != "" {
		return u.Email
	}
	return fallback
}
