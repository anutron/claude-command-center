package refresh

import (
	"context"
	"log"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func fetchCalendarEvents(ctx context.Context, ts oauth2.TokenSource, calendarIDs []string) (*CalendarData, error) {
	srv, err := calendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, err
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	todayEnd := todayStart.Add(24 * time.Hour)
	tomorrowEnd := todayEnd.Add(24 * time.Hour)

	var todayEvents, tomorrowEvents []CalendarEvent

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

	return &CalendarData{
		Today:    todayEvents,
		Tomorrow: tomorrowEvents,
	}, nil
}

func listEvents(ctx context.Context, srv *calendar.Service, calendarID string, timeMin, timeMax time.Time) ([]CalendarEvent, error) {
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

	var result []CalendarEvent
	for _, item := range events.Items {
		if item.EventType == "workingLocation" {
			continue
		}

		ev := CalendarEvent{
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
