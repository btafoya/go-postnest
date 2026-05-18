package calendar

import (
	"time"

	"github.com/google/uuid"
	"github.com/go-postnest/postnest/internal/models"
)

// eventDTO is the JSON contract consumed by the React frontend.
type eventDTO struct {
	ID          uuid.UUID `json:"id"`
	UID         string    `json:"uid"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"all_day"`
	RRule       string    `json:"rrule"`
	Status      string    `json:"status"`
	Attendees   []string  `json:"attendees"`
}

func toEventDTO(e *models.CalendarEvent) eventDTO {
	at := e.Attendees
	if at == nil {
		at = []string{}
	}
	return eventDTO{
		ID:          e.ID,
		UID:         e.UID,
		Title:       e.Summary,
		Description: e.Description,
		Location:    e.Location,
		Start:       e.StartsAt,
		End:         e.EndsAt,
		AllDay:      e.AllDay,
		RRule:       e.RRule,
		Status:      e.Status,
		Attendees:   at,
	}
}

func toEventDTOs(evs []*models.CalendarEvent) []eventDTO {
	out := make([]eventDTO, 0, len(evs))
	for _, e := range evs {
		out = append(out, toEventDTO(e))
	}
	return out
}

// calendarDTO is the JSON contract for a calendar collection.
type calendarDTO struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Description string    `json:"description"`
}

func toCalendarDTO(c *models.Calendar) calendarDTO {
	return calendarDTO{ID: c.ID, Name: c.Name, Color: c.Color, Description: c.Description}
}
