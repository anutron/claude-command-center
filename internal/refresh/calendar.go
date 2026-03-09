package refresh

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// CalendarSource fetches today/tomorrow calendar events from Google Calendar.
type CalendarSource struct {
	CalendarIDs       []string
	AutoAcceptDomains []string
	enabled           bool
}

// NewCalendarSource creates a CalendarSource with the given config.
func NewCalendarSource(enabled bool, calendarIDs, autoAcceptDomains []string) *CalendarSource {
	return &CalendarSource{
		CalendarIDs:       calendarIDs,
		AutoAcceptDomains: autoAcceptDomains,
		enabled:           enabled,
	}
}

func (s *CalendarSource) Name() string    { return "calendar" }
func (s *CalendarSource) Enabled() bool   { return s.enabled }

func (s *CalendarSource) Fetch(ctx context.Context) (*SourceResult, error) {
	ts, err := loadCalendarAuth()
	if err != nil {
		return nil, fmt.Errorf("calendar auth: %w", err)
	}

	calendarIDs := s.CalendarIDs
	if len(calendarIDs) == 0 {
		calendarIDs = []string{"primary"}
	}

	data, err := fetchCalendarEvents(ctx, ts, calendarIDs)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	if len(s.AutoAcceptDomains) > 0 {
		autoAccept(ctx, ts, s.AutoAcceptDomains)
	}

	return &SourceResult{Calendar: data}, nil
}

// calendarTokenSource returns the calendar token source if available.
// Used by executePendingActions which needs calendar auth separately.
func calendarTokenSource() (oauth2.TokenSource, error) {
	return loadCalendarAuth()
}

func fetchCalendarEvents(ctx context.Context, ts oauth2.TokenSource, calendarIDs []string) (*db.CalendarData, error) {
	srv, err := calendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayEnd := todayStart.Add(24 * time.Hour)
	tomorrowEnd := todayEnd.Add(24 * time.Hour)

	var todayEvents, tomorrowEvents []db.CalendarEvent

	for _, calID := range calendarIDs {
		today, err := listEvents(ctx, srv, calID, todayStart, todayEnd)
		if err != nil {
			log.Printf("calendar %s today fetch: %v", calID, err)
			continue
		}
		todayEvents = append(todayEvents, today...)

		tomorrow, err := listEvents(ctx, srv, calID, todayEnd, tomorrowEnd)
		if err != nil {
			log.Printf("calendar %s tomorrow fetch: %v", calID, err)
			continue
		}
		tomorrowEvents = append(tomorrowEvents, tomorrow...)
	}

	return &db.CalendarData{
		Today:    todayEvents,
		Tomorrow: tomorrowEvents,
	}, nil
}

func listEvents(ctx context.Context, srv *calendar.Service, calendarID string, timeMin, timeMax time.Time) ([]db.CalendarEvent, error) {
	events, err := srv.Events.List(calendarID).
		TimeMin(timeMin.Format(time.RFC3339)).
		TimeMax(timeMax.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}

	var result []db.CalendarEvent
	for _, item := range events.Items {
		if item.EventType == "workingLocation" {
			continue
		}

		ev := db.CalendarEvent{
			Title:      item.Summary,
			CalendarID: calendarID,
		}

		if item.Start.DateTime == "" {
			ev.AllDay = true
			if t, err := time.Parse("2006-01-02", item.Start.Date); err == nil {
				ev.Start = t
			}
			if t, err := time.Parse("2006-01-02", item.End.Date); err == nil {
				ev.End = t
			}
		} else {
			if t, err := time.Parse(time.RFC3339, item.Start.DateTime); err == nil {
				ev.Start = t
			}
			if t, err := time.Parse(time.RFC3339, item.End.DateTime); err == nil {
				ev.End = t
			}
		}

		for _, attendee := range item.Attendees {
			if attendee.Self && attendee.ResponseStatus == "declined" {
				ev.Declined = true
				break
			}
		}

		result = append(result, ev)
	}

	return result, nil
}

// autoAccept accepts pending events from organizers matching the given domains.
func autoAccept(ctx context.Context, ts oauth2.TokenSource, domains []string) {
	srv, err := calendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowEnd := todayStart.Add(48 * time.Hour)

	events, err := srv.Events.List("primary").
		TimeMin(todayStart.Format(time.RFC3339)).
		TimeMax(tomorrowEnd.Format(time.RFC3339)).
		SingleEvents(true).
		Context(ctx).
		Do()
	if err != nil {
		return
	}

	for _, item := range events.Items {
		needsAction := false
		for _, attendee := range item.Attendees {
			if attendee.Self && attendee.ResponseStatus == "needsAction" {
				needsAction = true
				break
			}
		}
		if !needsAction {
			continue
		}

		if item.Organizer != nil && matchesDomain(item.Organizer.Email, domains) {
			for i, attendee := range item.Attendees {
				if attendee.Self {
					item.Attendees[i].ResponseStatus = "accepted"
					break
				}
			}
			_, err := srv.Events.Patch("primary", item.Id, &calendar.Event{
				Attendees: item.Attendees,
			}).SendUpdates("none").Context(ctx).Do()
			if err != nil {
				log.Printf("auto-accept failed for %q: %v", item.Summary, err)
			}
		}
	}
}

func matchesDomain(email string, domains []string) bool {
	for _, domain := range domains {
		if strings.HasSuffix(email, "@"+domain) {
			return true
		}
	}
	return false
}
