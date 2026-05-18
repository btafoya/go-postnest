package calendar

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/emersion/go-ical"
	"github.com/go-postnest/postnest/internal/models"
)

// EventToICS serializes a CalendarEvent into an iCalendar VEVENT document.
func EventToICS(e *models.CalendarEvent) ([]byte, error) {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropProductID, "-//go-postnest//calendar//EN")
	cal.Props.SetText(ical.PropVersion, "2.0")

	ev := ical.NewEvent()
	ev.Props.SetText(ical.PropUID, e.UID)
	ev.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	ev.Props.SetDateTime(ical.PropDateTimeStart, e.StartsAt)
	ev.Props.SetDateTime(ical.PropDateTimeEnd, e.EndsAt)
	if e.Summary != "" {
		ev.Props.SetText(ical.PropSummary, e.Summary)
	}
	if e.Description != "" {
		ev.Props.SetText(ical.PropDescription, e.Description)
	}
	if e.Location != "" {
		ev.Props.SetText(ical.PropLocation, e.Location)
	}
	if e.Status != "" {
		ev.Props.SetText(ical.PropStatus, e.Status)
	}
	if e.Organizer != "" {
		ev.Props.SetText(ical.PropOrganizer, e.Organizer)
	}
	if e.RRule != "" {
		ev.Props.SetText(ical.PropRecurrenceRule, e.RRule)
	}
	if e.Sequence > 0 {
		ev.Props.SetText(ical.PropSequence, fmt.Sprintf("%d", e.Sequence))
	}
	cal.Children = append(cal.Children, ev.Component)

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ICSToEvent parses an iCalendar document into a CalendarEvent.
// Recurrence and X-properties beyond the mapped columns are not preserved.
func ICSToEvent(data []byte) (*models.CalendarEvent, error) {
	dec := ical.NewDecoder(bytes.NewReader(data))
	cal, err := dec.Decode()
	if err != nil {
		return nil, err
	}
	for _, comp := range cal.Children {
		if comp.Name != ical.CompEvent {
			continue
		}
		ev := &ical.Event{Component: comp}
		e := &models.CalendarEvent{}
		e.UID = textProp(comp, ical.PropUID)
		e.Summary = textProp(comp, ical.PropSummary)
		e.Description = textProp(comp, ical.PropDescription)
		e.Location = textProp(comp, ical.PropLocation)
		e.Status = textProp(comp, ical.PropStatus)
		e.Organizer = textProp(comp, ical.PropOrganizer)
		e.RRule = textProp(comp, ical.PropRecurrenceRule)
		if start, err := ev.DateTimeStart(time.UTC); err == nil {
			e.StartsAt = start
		}
		if end, err := ev.DateTimeEnd(time.UTC); err == nil {
			e.EndsAt = end
		}
		e.Attendees = []string{}
		e.ETag = computeETag(e)
		return e, nil
	}
	return nil, fmt.Errorf("no VEVENT in calendar data")
}

func textProp(c *ical.Component, name string) string {
	if p := c.Props.Get(name); p != nil {
		return p.Value
	}
	return ""
}

func computeETag(e *models.CalendarEvent) string {
	h := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%s|%s|%d|%d|%s",
		e.UID, e.Summary, e.Description, e.Location,
		e.StartsAt.Unix(), e.EndsAt.Unix(), e.Status))
	return hex.EncodeToString(h[:16])
}
