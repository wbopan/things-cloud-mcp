package sync_test

import (
	"fmt"
	"log"
	"os"

	things "github.com/arthursoares/things-cloud-sdk"
	"github.com/arthursoares/things-cloud-sdk/sync"
)

func Example() {
	// Create Things Cloud client
	client := things.New(
		things.APIEndpoint,
		os.Getenv("THINGS_USERNAME"),
		os.Getenv("THINGS_PASSWORD"),
	)

	// Open sync database (creates if doesn't exist)
	syncer, err := sync.Open("~/.things-agent/state.db", client)
	if err != nil {
		log.Fatal(err)
	}
	defer syncer.Close()

	// Sync and get changes
	changes, err := syncer.Sync()
	if err != nil {
		log.Fatal(err)
	}

	// React to changes
	for _, c := range changes {
		switch e := c.(type) {
		case sync.TaskCreated:
			fmt.Printf("New task: %s\n", e.Task.Title)
		case sync.TaskCompleted:
			fmt.Printf("Completed: %s\n", e.Task.Title)
		case sync.TaskMovedToToday:
			fmt.Printf("Scheduled for today: %s (was in %s)\n", e.Task.Title, e.From)
		case sync.AreaCreated:
			fmt.Printf("New area: %s\n", e.Area.Title)
		}
	}

	// Query current state
	state := syncer.State()

	inbox, _ := state.TasksInInbox(sync.QueryOpts{})
	fmt.Printf("\nInbox has %d items\n", len(inbox))

	projects, _ := state.AllProjects(sync.QueryOpts{})
	fmt.Printf("You have %d active projects\n", len(projects))
}
