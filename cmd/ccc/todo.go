package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/anutron/claude-command-center/internal/config"
	"github.com/anutron/claude-command-center/internal/db"
)

func runTodo(args []string) error {
	fs := flag.NewFlagSet("todo", flag.ContinueOnError)
	getID := fs.String("get", "", "Get todo by display_id (integer)")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *getID == "" {
		return fmt.Errorf("usage: ccc todo --get <display_id>")
	}

	displayID, err := strconv.Atoi(*getID)
	if err != nil {
		return fmt.Errorf("invalid display_id %q: must be an integer", *getID)
	}

	database, err := db.OpenDB(config.DBPath())
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	todo, err := db.DBLoadTodoByDisplayID(database, displayID)
	if err != nil {
		return fmt.Errorf("query todo: %w", err)
	}
	if todo == nil {
		return fmt.Errorf("no todo with display_id %d", displayID)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(todo)
}
