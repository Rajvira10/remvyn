// Command updsmoke runs a real self-update against the published releases:
// it claims an older version, checks GitHub, downloads and verifies the
// latest release, swaps itself for it and lets the helper relaunch the new
// Remvyn. Build it into an empty folder and give it its own data folder, so
// the relaunched app doesn't touch your real vault:
//
//	go build -o /tmp/remvyn-e2e/Remvyn.exe ./tools/updsmoke
//	cd /tmp/remvyn-e2e && mkdir -p data && REMVYN_DATA_DIR="$PWD/data" ./Remvyn.exe -as 2.7.0
//
// Afterwards the binary in that folder is the latest release (compare its
// SHA-256 with the release asset) and the new version is running.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"remvyn/backend/updater"
)

func main() {
	as := flag.String("as", "0.0.1", "version to pretend to be")
	repo := flag.String("repo", updater.DefaultRepo, "GitHub repository with the releases")
	flag.Parse()
	if os.Getenv("REMVYN_DATA_DIR") == "" {
		fmt.Fprintln(os.Stderr, "updsmoke: set REMVYN_DATA_DIR to a scratch folder, so the relaunched Remvyn doesn't open your real vault")
		os.Exit(2)
	}

	u := updater.New(updater.Options{Current: *as, Repo: *repo, Emit: func(s updater.Status) {
		if s.State != updater.StateDownloading {
			fmt.Println("state:", s.State, s.Error, s.Manual)
		}
	}})
	s, err := u.Check(context.Background())
	if err != nil {
		fmt.Println("check:", err)
		os.Exit(1)
	}
	fmt.Printf("latest=%s available=%v manual=%q\n", s.Latest.Version, s.Available, s.Manual)
	if err := u.Install(context.Background()); err != nil {
		fmt.Println("install:", err)
		os.Exit(1)
	}
	fmt.Println("downloaded", u.Status().Downloaded, "bytes; exiting so the helper relaunches")
}
