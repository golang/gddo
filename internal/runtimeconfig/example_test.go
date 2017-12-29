package runtimeconfig_test

import (
	"context"
	"log"

	"github.com/golang/gddo/internal/runtimeconfig"
)

func Example() {
	// Create a Client object.
	ctx := context.Background()
	client, err := runtimeconfig.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Create a Watcher object.
	w, err := client.NewWatcher(ctx, "project", "config-name", "food", nil)
	// Use retrieved Variable and apply to configurations accordingly.
	log.Printf("value: %s\n", string(w.Variable().Value))

	// Optionally, get a Context with cancel func to stop the Watch call.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Have a separate goroutine that waits for changes.
	go func() {
		for {
			select {
			case <-ctx.Done():
				// Cancelled or timed out.
				return
			default:
				if err := w.Watch(ctx); err != nil {
					// Log or handle other errors
					continue
				}
				// Use updated variable accordingly.
				log.Printf("value: %s\n", string(w.Variable().Value))
			}
		}
	}()
}
