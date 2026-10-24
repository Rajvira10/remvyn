// Command devgithub runs a fake GitHub (device-flow sign-in and the Git
// Data API) for trying Flux sync locally:
//
//	go run ./tools/devgithub -addr 127.0.0.1:2280
//	FLUX_GITHUB_WEB=http://127.0.0.1:2280 FLUX_GITHUB_API=http://127.0.0.1:2280 \
//	  FLUX_GITHUB_CLIENT_ID=dev FLUX_GITHUB_APP_SLUG=flux-dev wails dev
//
// Sign-in is approved automatically on the second poll.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"

	"flux/backend/testutil"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:2280", "listen address")
	repo := flag.String("repo", "octo/flux-vault", "private repository to create")
	flag.Parse()
	gh := testutil.NewFakeGitHubOn(*addr)
	defer gh.Close()
	gh.AddRepo(*repo, true)
	fmt.Printf("fake GitHub on %s (user %q, repo %s)\n", gh.URL, gh.Login, *repo)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
