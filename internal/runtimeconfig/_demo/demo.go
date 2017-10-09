/*
This binary demonstrates watching over a Runtime Configurator variable using the runtimeconfig
package.  To cancel the Watcher.Watch call, enter 'x' and '<enter>' keys on the terminal.
*/
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"time"

	"github.com/golang/gddo/internal/runtimeconfig"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintf(os.Stderr,
			"Usage: %s <project-id> <config-name> <var-name>\n\n",
			path.Base(os.Args[0]))
		os.Exit(1)
	}

	projectID := os.Args[1]
	configName := os.Args[2]
	varName := os.Args[3]

	ctx := context.Background()
	client, err := runtimeconfig.NewClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w, err := client.NewWatcher(ctx, projectID, configName, varName,
		&runtimeconfig.WatchOptions{WaitTime: 10 * time.Second})
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		key := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(key)
			if err != nil {
				log.Printf("stdin error: %v\n", err)
			}
			if n == 1 && key[0] == 'x' {
				log.Println("quiting demo")
				cancel()
				time.Sleep(1 * time.Second)
				os.Exit(0)
			}
		}
	}()

	vrbl := w.Variable()
	log.Printf("watching variable %v\n", variableString(&vrbl))

	isWatching := true
	for isWatching {
		log.Println("waiting for update...")
		select {
		case <-ctx.Done():
			log.Println("done watching")
			isWatching = false
		default:
			err := w.Watch(ctx)
			vrbl = w.Variable()
			if err == nil {
				log.Printf("Updated: %s\n", variableString(&vrbl))
			} else {
				log.Println(err)
				if runtimeconfig.IsDeleted(err) {
					log.Printf("Deleted: %s\n", variableString(&vrbl))
				}
			}
		}
	}
}

func variableString(v *runtimeconfig.Variable) string {
	return fmt.Sprintf("<name: %q, value: %q, isDeleted: %t, updateTime: %v>",
		v.Name, string(v.Value), v.IsDeleted, v.UpdateTime)
}
