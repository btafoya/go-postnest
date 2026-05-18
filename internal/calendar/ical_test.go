package calendar

import (
	"testing"
	"time"

	"github.com/go-postnest/postnest/internal/models"
)

func TestEventToICS_ICSToEvent_RoundTrip(t *testing.T) {
	start := time.Date(2026, 5, 17, 14, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	in := &models.CalendarEvent{
		UID:         "evt-123",
		Summary:     "Design Review",
		Description: "Quarterly planning",
		Location:    "Room 4",
		StartsAt:    start,
		EndsAt:      end,
		Status:      "CONFIRMED",
	}
	data, err := EventToICS(in)
	if err != nil {
		t.Fatalf("EventToICS: %v", err)
	}
	out, err := ICSToEvent(data)
	if err != nil {
		t.Fatalf("ICSToEvent: %v", err)
	}
	if out.UID != in.UID {
		t.Errorf("UID: got %q want %q", out.UID, in.UID)
	}
	if out.Summary != in.Summary {
		t.Errorf("Summary: got %q want %q", out.Summary, in.Summary)
	}
	if !out.StartsAt.Equal(start) {
		t.Errorf("StartsAt: got %v want %v", out.StartsAt, start)
	}
	if !out.EndsAt.Equal(end) {
		t.Errorf("EndsAt: got %v want %v", out.EndsAt, end)
	}
	if out.ETag == "" {
		t.Error("ETag should be computed")
	}
}

func TestICSToEvent_NoEvent(t *testing.T) {
	_, err := ICSToEvent([]byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n"))
	if err == nil {
		t.Fatal("expected error for calendar with no VEVENT")
	}
}

func TestComputeETag_StableAndSensitive(t *testing.T) {
	e := &models.CalendarEvent{UID: "x", Summary: "a", StartsAt: time.Unix(1, 0), EndsAt: time.Unix(2, 0)}
	first := computeETag(e)
	if first != computeETag(e) {
		t.Fatal("etag must be stable for identical input")
	}
	e.Summary = "b"
	if computeETag(e) == first {
		t.Fatal("etag must change when fields change")
	}
}
