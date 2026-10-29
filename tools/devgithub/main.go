// Command devgithub runs a fake GitHub (device-flow sign-in and the Git
// Data API) for trying Remvyn sync locally:
//
//	go run ./tools/devgithub -addr 127.0.0.1:2280
//	REMVYN_GITHUB_WEB=http://127.0.0.1:2280 REMVYN_GITHUB_API=http://127.0.0.1:2280 \
//	  REMVYN_GITHUB_CLIENT_ID=dev REMVYN_GITHUB_APP_SLUG=remvyn-dev wails dev
//
// Sign-in is approved automatically on the second poll.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"remvyn/backend/testutil"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:2280", "listen address")
	repo := flag.String("repo", "octo/remvyn-vault", "private repositories to create, comma-separated")
	flag.Parse()
	gh := testutil.NewFakeGitHubOn(*addr)
	defer gh.Close()
	for _, r := range strings.Split(*repo, ",") {
		gh.AddRepo(strings.TrimSpace(r), true)
	}
	fmt.Printf("fake GitHub on %s (user %q, repo %s)\n", gh.URL, gh.Login, *repo)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
