package calendar

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/go-postnest/postnest/internal/models"
)

// ErrNotFound indicates the requested calendar resource does not exist.
var ErrNotFound = fmt.Errorf("not found")

// Store is the canonical interface for calendar persistence.
type Store interface {
	ListCalendars(ctx context.Context, domainID, userID uuid.UUID) ([]*models.Calendar, error)
	GetCalendar(ctx context.Context, domainID, userID, calID uuid.UUID) (*models.Calendar, error)
	CreateCalendar(ctx context.Context, cal *models.Calendar) error
	DeleteCalendar(ctx context.Context, domainID, userID, calID uuid.UUID) error

	ListEvents(ctx context.Context, calID uuid.UUID, from, to time.Time) ([]*models.CalendarEvent, error)
	GetEvent(ctx context.Context, calID uuid.UUID, uid string) (*models.CalendarEvent, error)
	PutEvent(ctx context.Context, ev *models.CalendarEvent) error
	DeleteEvent(ctx context.Context, calID uuid.UUID, uid string) error
	BumpCTag(ctx context.Context, calID uuid.UUID) error
}
