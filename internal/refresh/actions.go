package refresh

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/anutron/claude-command-center/internal/db"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

func executePendingActions(ctx context.Context, ts oauth2.TokenSource, cc *db.CommandCenter) {
	if len(cc.PendingActions) == 0 {
		return
	}

	srv, err := calendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		log.Printf("cannot execute pending actions: calendar service error: %v", err)
		return
	}

	var remaining []db.PendingAction
	for _, action := range cc.PendingActions {
		if action.Type != "booking" {
			remaining = append(remaining, action)
			continue
		}

		title := "Blocked Time"
		for _, todo := range cc.Todos {
			if todo.ID == action.TodoID {
				title = todo.Title
				break
			}
		}

		slot, err := findFreeSlot(ctx, srv, action.DurationMinutes)
		if err != nil {
			log.Printf("could not find free slot for %q: %v", title, err)
			remaining = append(remaining, action)
			continue
		}

		event := &calendar.Event{
			Summary: title,
			Start: &calendar.EventDateTime{
				DateTime: slot.Format(time.RFC3339),
			},
			End: &calendar.EventDateTime{
				DateTime: slot.Add(time.Duration(action.DurationMinutes) * time.Minute).Format(time.RFC3339),
			},
		}

		_, err = srv.Events.Insert("primary", event).Context(ctx).Do()
		if err != nil {
			log.Printf("failed to create event for %q: %v", title, err)
			remaining = append(remaining, action)
			continue
		}

		log.Printf("booked %dm for %q at %s", action.DurationMinutes, title, slot.Format("15:04"))
	}

	cc.PendingActions = remaining
}

func findFreeSlot(ctx context.Context, srv *calendar.Service, durationMinutes int) (time.Time, error) {
	now := time.Now()
	start := now.Truncate(15 * time.Minute).Add(15 * time.Minute)
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 18, 0, 0, 0, now.Location())

	if start.After(endOfDay) {
		tomorrow := now.AddDate(0, 0, 1)
		start = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 9, 0, 0, 0, tomorrow.Location())
		endOfDay = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 18, 0, 0, 0, tomorrow.Location())
	}

	events, err := srv.Events.List("primary").
		TimeMin(start.Format(time.RFC3339)).
		TimeMax(endOfDay.Format(time.RFC3339)).
		SingleEvents(true).
		OrderBy("startTime").
		Context(ctx).
		Do()
	if err != nil {
		return time.Time{}, err
	}

	duration := time.Duration(durationMinutes) * time.Minute
	candidate := start

	for _, item := range events.Items {
		if item.Start.DateTime == "" {
			continue
		}
		eventStart, _ := time.Parse(time.RFC3339, item.Start.DateTime)

		if candidate.Add(duration).Before(eventStart) || candidate.Add(duration).Equal(eventStart) {
			return candidate, nil
		}

		eventEnd, _ := time.Parse(time.RFC3339, item.End.DateTime)
		if eventEnd.After(candidate) {
			candidate = eventEnd
		}
	}

	if candidate.Add(duration).Before(endOfDay) || candidate.Add(duration).Equal(endOfDay) {
		return candidate, nil
	}

	return time.Time{}, fmt.Errorf("no free slot of %d minutes found today", durationMinutes)
}
